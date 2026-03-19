package allminilm

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	ort "github.com/yalue/onnxruntime_go"
)

// These pull the files from the SAME folder as this .go file
//go:embed tokenizer.json
var embeddedTokenizer []byte

//go:embed model.onnx
var onnxModel []byte

type Model struct {
	tk          tokenizer.Tokenizer
	session     *ort.DynamicAdvancedSession
	runtimePath string
}

type ModelOption = func(*Model)

func WithRuntimePath(path string) ModelOption {
	return func(m *Model) {
		m.runtimePath = path
	}
}

func NewModel(opts ...ModelOption) (*Model, error) {
	model := new(Model)

	for _, opt := range opts {
		opt(model)
	}

	tk, err := pretrained.FromReader(bytes.NewBuffer(embeddedTokenizer))
	if err != nil {
		return nil, fmt.Errorf("failed to load tokenizer: %w", err)
	}

	if model.runtimePath != "" {
		ort.SetSharedLibraryPath(model.runtimePath)
	} else {
		path, ok := os.LookupEnv("ONNXRUNTIME_LIB_PATH")
		if ok {
			ort.SetSharedLibraryPath(path)
		}
	}

	err = ort.InitializeEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize onnx runtime: %w", err)
	}

	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}

	session, err := ort.NewDynamicAdvancedSessionWithONNXData(onnxModel, inputNames, outputNames, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &Model{
		tk:      *tk,
		session: session,
	}, nil
}

func (m *Model) Close() error {
	if m.session != nil {
		m.session.Destroy()
	}
	return ort.DestroyEnvironment()
}

func (m *Model) Compute(sentence string, addSpecialTokens bool) ([]float32, error) {
	results, err := m.ComputeBatch([]string{sentence}, addSpecialTokens)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

func (m *Model) ComputeBatch(sentences []string, addSpecialTokens bool) ([][]float32, error) {
	if len(sentences) == 0 {
		return nil, nil
	}

	inputBatch := []tokenizer.EncodeInput{}
	for _, s := range sentences {
		// FIXED: NewInputSequence instead of NewRawInputSequence
		inputBatch = append(inputBatch, tokenizer.NewSingleEncodeInput(tokenizer.NewInputSequence(s)))
	}

	if len(inputBatch) == 0 {
		return nil, nil
	}
	encodings, err := m.tk.EncodeBatch(inputBatch, addSpecialTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to tokenize: %w", err)
	}
	return m.ComputeBatchFromEncodings(encodings)
}

func (m *Model) ComputeBatchFromEncodings(encodings []tokenizer.Encoding) ([][]float32, error) {
	batchSize := len(encodings)
	seqLength := len(encodings[0].Ids)
	hiddenSize := 384

	inputShape := ort.NewShape(int64(batchSize), int64(seqLength))

	inputIdsData := make([]int64, batchSize*seqLength)
	attentionMaskData := make([]int64, batchSize*seqLength)
	tokenTypeIdsData := make([]int64, batchSize*seqLength)

	for b := 0; b < batchSize; b++ {
		for i, id := range encodings[b].Ids {
			inputIdsData[b*seqLength+i] = int64(id)
		}
		for i, mask := range encodings[b].AttentionMask {
			attentionMaskData[b*seqLength+i] = int64(mask)
		}
		for i, typeId := range encodings[b].TypeIds {
			tokenTypeIdsData[b*seqLength+i] = int64(typeId)
		}
	}

	inputIdsTensor, _ := ort.NewTensor(inputShape, inputIdsData)
	defer inputIdsTensor.Destroy()
	attentionMaskTensor, _ := ort.NewTensor(inputShape, attentionMaskData)
	defer attentionMaskTensor.Destroy()
	tokenTypeIdsTensor, _ := ort.NewTensor(inputShape, tokenTypeIdsData)
	defer tokenTypeIdsTensor.Destroy()

	sentenceOutputShape := ort.NewShape(int64(batchSize), int64(seqLength), int64(hiddenSize))
    sentenceOutputTensor, _ := ort.NewEmptyTensor[float32](sentenceOutputShape)
    defer sentenceOutputTensor.Destroy()

    err := m.session.Run([]ort.Value{inputIdsTensor, attentionMaskTensor, tokenTypeIdsTensor}, []ort.Value{sentenceOutputTensor})
    if err != nil {
        return nil, err
    }

    flatOutput := sentenceOutputTensor.GetData()
    results := make([][]float32, batchSize)
    
    // UPDATE: Extraction logic for CLS Pooling
    for i := 0; i < batchSize; i++ {
        results[i] = make([]float32, hiddenSize)
        // We calculate the start of the CLS token (index 0 of the sequence) for each batch item
        // Each batch item has (seqLength * hiddenSize) elements
        start := i * seqLength * hiddenSize
        copy(results[i], flatOutput[start : start+hiddenSize])
    }

    return results, nil
}