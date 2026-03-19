package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"
	"github.com/lancedb/lancedb-go/pkg/contracts"
	"github.com/lancedb/lancedb-go/pkg/lancedb"
	allminilm "kylestanfield.com/shelllm/src/internal/all_minilm"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

// IPC Consts
const historySocket string = "/tmp/shelllm.history.socket"
const bufferSize int = 4096
const querySocket string = "/tmp/shelllm.query.socket"

// VectorDB Consts
const dbPath string = "/tmp/testdb"
const tableName string = "CommandHistory"
const vectorColumnName string = "vector"
const kMostSimilar int = 5

// Gemini consts
const systemPrompt string = "You are answering user queries from a command line tool that tracks bash command history and output for added contest. Make sure your output is formatted nicely for displaying in a Linux or MacOS terminal. The command context is:\n"

// all mini llm has 384 dimension embeddings
const EmbeddingDimensions int = 384

func openOrCreateDatabase(ctx context.Context) (contracts.ITable, *arrow.Schema, error) {
	conn, err := lancedb.Connect(ctx, dbPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to db: %v\n", err)
		return nil, nil, err
	}

	vectorMetadata := arrow.NewMetadata(
		[]string{"lancedb:metric"},
		[]string{"cosine"},
	)

	fields := []arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "command", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "output", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "returncode", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{
			Name:     vectorColumnName,
			Type:     arrow.FixedSizeListOf(int32(EmbeddingDimensions), arrow.PrimitiveTypes.Float32),
			Nullable: false,
			Metadata: vectorMetadata,
		},
	}
	arrowSchema := arrow.NewSchema(fields, nil)

	tables, err := conn.TableNames(ctx)
	// check if table already exists
	for _, v := range tables {
		if v == tableName {
			table, err := conn.OpenTable(ctx, tableName)
			if err != nil {
				return nil, nil, err
			}
			debugLog("Existing table found.")
			return table, arrowSchema, nil
		}
	}
	// otherwise create the table
	debugLog("Table not found. Creating new table")
	schema, err := lancedb.NewSchema(arrowSchema)
	if err != nil {
		return nil, nil, err
	}
	table, err := conn.CreateTable(ctx, tableName, schema)
	if err != nil {
		return nil, nil, err
	}
	return table, arrowSchema, nil
}

type CommandResult struct {
	Command    string
	Output     string
	ReturnCode int
}

func (c CommandResult) String() string {
	return c.Command + " " + c.Output
}

func SaveCommandResult(ctx context.Context,
	res CommandResult,
	embedding []float32,
	table contracts.ITable,
	schema *arrow.Schema) error {
	pool := memory.NewGoAllocator()
	builder := array.NewRecordBuilder(pool, schema)
	defer builder.Release()

	// 3. Fill standard fields
	// columnar db has no auto-increment
	// use timestamp as an id
	id := int32(time.Now().UnixNano() % 2147483647)
	builder.Field(0).(*array.Int32Builder).Append(id) // Assuming ID 1 or auto-increment logic
	builder.Field(1).(*array.StringBuilder).Append(res.Command)
	builder.Field(2).(*array.StringBuilder).Append(res.Output)
	builder.Field(3).(*array.Int32Builder).Append(int32(res.ReturnCode))

	// 4. Fill the Vector (FixedSizeList)
	// We get the ListBuilder, then get its internal ValueBuilder (Float32)
	listBuilder := builder.Field(4).(*array.FixedSizeListBuilder)
	valueBuilder := listBuilder.ValueBuilder().(*array.Float32Builder)

	listBuilder.Append(true)                  // Start a new list entry
	valueBuilder.AppendValues(embedding, nil) // Append all floats at once

	// 5. Generate the Record
	record := builder.NewRecord()
	defer record.Release()
	return table.Add(ctx, record, nil)
}

func GenerateEmbedding(ctx context.Context, sentence string, model *allminilm.Model) ([]float32, error) {
	return model.Compute(sentence, false)
}

func ReadAndHandleCommand(ctx context.Context,
	decoder *json.Decoder,
	model *allminilm.Model,
	db contracts.ITable,
	schema *arrow.Schema) error {

	var res CommandResult
	err := decoder.Decode(&res)
	if res.Command == "" && res.Output == "" {
		return nil
	}
	debugLog("Decoded command info: %+v", res)
	if err != nil {
		if err == io.EOF {
			// Connection closed
			return err
		}
		fmt.Fprintf(os.Stderr, "Failed to read from connection: %v\n", err)
		return err
	}
	debugLog("Read command over connection: %+v", res)
	// Call the vector embedding library to get the vector for
	// command result
	cmdEmbedding, err := GenerateEmbedding(ctx, res.String(), model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate embedding for %+v\n%v\n", res, err)
		return err
	}

	// write vector to the vectorDB
	err = SaveCommandResult(ctx, res, cmdEmbedding, db, schema)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save record to table for %+v\n%v\n", res, err)
		return err
	}
	debugLog("Saved vector to the database!")
	return nil
}

func handleConnection(ctx context.Context,
	connection net.Conn,
	model *allminilm.Model,
	db contracts.ITable,
	schema *arrow.Schema) {

	debugLog("Handling connection")
	defer connection.Close()

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	decoder := json.NewDecoder(connection)

	for {
		if timeoutCtx.Err() != nil {
			debugLog("Connection handler stopping: timeout")
			return
		}
		err := ReadAndHandleCommand(timeoutCtx, decoder, model, db, schema)
		if err != nil {
			return
		}
	}
}

func parseResultToCommand(resultMap []map[string]interface{}) []CommandResult {
	slice := make([]CommandResult, 0, len(resultMap))
	for _, row := range resultMap {
		rc := 0
		switch v := row["returncode"].(type) {
		case float64:
			rc = int(v)
		case int32:
			rc = int(v)
		case int:
			rc = v
		}
		res := CommandResult{
			Command:    row["command"].(string),
			Output:     row["output"].(string),
			ReturnCode: rc,
		}
		slice = append(slice, res)
	}
	return slice
}

func createContext(commands []CommandResult) string {
	var contextBuilder strings.Builder

	for i, row := range commands {
		var b strings.Builder
		b.WriteString("Command ")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(":\n User Command:")
		b.WriteString(row.Command)
		b.WriteString("\n Command Output:")
		b.WriteString(row.Output)
		b.WriteString("\n rc: ")
		b.WriteString(strconv.Itoa(row.ReturnCode))
		b.WriteString("\n")
		contextBuilder.WriteString(b.String())
	}
	return contextBuilder.String()
}

func handleQuery(ctx context.Context,
	conn net.Conn, model *allminilm.Model,
	table contracts.ITable,
	gemini *genai.Client) {

	// Read user query
	buf := make([]byte, 2048)
	bytesRead, err := conn.Read(buf)
	if err != nil || bytesRead <= 0 {
		fmt.Fprintf(os.Stderr, "Failed to read from unix socket: err=%v , n=%d", err, bytesRead)
		return
	}
	userQuery := string(buf)
	debugLog("Received user query %s", userQuery)

	// generate embedding for query
	embedding, err := GenerateEmbedding(ctx, userQuery, model)

	resultMap, err := table.VectorSearch(ctx, vectorColumnName, embedding, kMostSimilar)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to search the vectorDB: %v", err)
		return
	}
	cmdSlice := parseResultToCommand(resultMap)
	debugLog("Retrieved context from database:")
	for i, cmd := range cmdSlice {
		debugLog("  [%d] Command: %s | RC: %d | Output (truncated): %.100s...", i, cmd.Command, cmd.ReturnCode, cmd.Output)
	}
	geminiResponse, err := gemini.Models.GenerateContent(
		ctx,
		"gemini-flash-latest",
		genai.Text(systemPrompt+createContext(cmdSlice)+" The user query is: "+userQuery),
		nil,
	)
	if err != nil {
		debugLog("Failed to query Gemini gemini-3.1-flash %v", err)
		return
	}
	if len(geminiResponse.Candidates) > 0 && len(geminiResponse.Candidates[0].Content.Parts) > 0 {
		conn.Write([]byte(geminiResponse.Candidates[0].Content.Parts[0].Text))
		return
	}
	debugLog("Malformed response from Gemini")
}

func main() {
	debugLog("Entering main function")
	ctx := context.Background()
	// 0. setup all-MiniLM-L6-v2 model for sentence embeddings
	model, err := allminilm.NewModel()
	if err != nil {
		log.Fatal(err)
	}
	debugLog("Successfully loaded embedding model")
	defer model.Close()
	// 1. connect to the vector DB
	// create it if it doesn't exist
	table, schema, err := openOrCreateDatabase(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Setup Gemini connection
	err = godotenv.Load()

	if err != nil {
		log.Fatal("Error loading .env file")
	}
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 3. create Unix domain sockets for IPC
	// one socket to receive command history from user's shell,
	// and another socket for handling queries
	_ = os.Remove(historySocket)
	_ = os.Remove(querySocket)

	//
	historyListener, err := net.Listen("unix", historySocket)
	if err != nil {
		log.Fatal(err)
	}
	// 4. listen for command history
	go func() {
		debugLog("Listening for history on socket %s", historySocket)
		for {
			conn, err := historyListener.Accept()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to accept command history connection: %v\n", err)
				continue
			}
			go handleConnection(ctx, conn, model, table, schema)
		}
	}()

	// 5. listen for user queries
	queryListener, _ := net.Listen("unix", querySocket)
	debugLog("Listening for queries on socket %s", querySocket)
	for {
		conn, err := queryListener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to accept query connection: %v\n", err)
			continue
		}
		go handleQuery(ctx, conn, model, table, geminiClient)
	}
}
