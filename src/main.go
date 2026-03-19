package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"
	"github.com/lancedb/lancedb-go/pkg/contracts"
	"github.com/lancedb/lancedb-go/pkg/lancedb"
	allminilm "kylestanfield.com/shelllm/src/internal/all_minilm"
)

const historySocket string = "/tmp/shelllm.history.socket"
const bufferSize int = 4096

const querySocket string = "/tmp/shelllm.query.socket"

const dbPath string = "/tmp/testdb"
const tableName string = "CommandHistory"

// all mini llm has 384 dimension embeddings
const EmbeddingDimensions int = 384

func openOrCreateDatabase(ctx context.Context) (contracts.ITable, *arrow.Schema, error) {
	conn, err := lancedb.Connect(ctx, dbPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to db: %v\n", err)
		return nil, nil, err
	}

	fields := []arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "command", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "output", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "returncode", Type: arrow.PrimitiveTypes.Int32, Nullable: false},
		{Name: "vector", Type: arrow.FixedSizeListOf(int32(EmbeddingDimensions),
			arrow.PrimitiveTypes.Float32), Nullable: false},
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
			fmt.Println("Existing table found.")
			return table, arrowSchema, nil
		}
	}
	// otherwise create the table
	fmt.Println("Table not found. Creating new table")
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

func GenerateEmbedding(ctx context.Context, result CommandResult, model *allminilm.Model) ([]float32, error) {
	return model.Compute(result.Command+" "+result.Output, false)
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
	fmt.Fprintf(os.Stdout, "Decoded command info: %+v\n", res)
	if err != nil {
		if err == io.EOF {
			// Connection closed
			return err
		}
		fmt.Fprintf(os.Stderr, "Failed to read from connection: %v\n", err)
		return err
	}
	fmt.Printf("Read command over connection: %+v\n", res)
	// Call the vector embedding library to get the vector for
	// command result
	cmdEmbedding, err := GenerateEmbedding(ctx, res, model)
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
	fmt.Println("Saved vector to the database!")
	return nil
}

func handleConnection(ctx context.Context,
	connection net.Conn,
	model *allminilm.Model,
	db contracts.ITable,
	schema *arrow.Schema) {

	fmt.Println("Handling connection")
	defer connection.Close()

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	decoder := json.NewDecoder(connection)

	for {
		if timeoutCtx.Err() != nil {
			fmt.Println("Connection handler stopping: timeout")
			return
		}
		err := ReadAndHandleCommand(timeoutCtx, decoder, model, db, schema)
		if err != nil {
			return
		}
	}
}

func handleQuery(ctx context.Context, conn net.Conn, model *allminilm.Model, table contracts.ITable) {
	// Read user query
	buf := make([]byte, 2048)
	bytesRead, err := conn.Read(buf)

}

func main() {
	fmt.Println("Entering main function")
	ctx := context.Background()
	// 0. setup all-MiniLM-L6-v2 model for sentence embeddings
	model, err := allminilm.NewModel()
	if err != nil {
		panic(err)
	}
	fmt.Println("Successfully loaded embedding model")
	defer model.Close()
	// 1. connect to the vector DB
	// create it if it doesn't exist
	table, schema, err := openOrCreateDatabase(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
	}

	// 2. create Unix domain sockets for IPC
	// one socket to receive command history from user's shell,
	// and another socket for handling queries
	_ = os.Remove(historySocket)
	_ = os.Remove(querySocket)

	//
	historyListener, err := net.Listen("unix", historySocket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Unix socket: %v\n", err)
	}
	// 3. listen for command history
	go func() {
		fmt.Printf("Listening for history on socket %s\n", historySocket)
		for {
			conn, err := historyListener.Accept()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to accept command history connection: %v\n", err)
				continue
			}
			go handleConnection(ctx, conn, model, table, schema)
		}
	}()

	// 4. listen for user queries
	queryListener, _ := net.Listen("unix", querySocket)
	fmt.Println("Listening for queris on socket %s", querySocket)

	for {
		conn, err := queryListener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to accept query connection: %v\n", err)
			continue
		}
		// TODO implement handlequery to use RAG to get context from command history
		// and pass that and the query to an LLM
		go handleQuery(ctx, conn, model, table)
	}
}
