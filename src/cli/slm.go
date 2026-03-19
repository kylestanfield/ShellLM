package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

const querySocket string = "/tmp/shelllm.query.socket"

func main() {
	query := strings.Join(os.Args[1:], " ")
	conn, err := net.Dial("unix", querySocket)
	if err != nil {
		log.Fatal("Failed to connect to ShellLM server on socket " + querySocket)
	}
	bytesRead, err := conn.Write([]byte(query))
	if err != nil || bytesRead <= 0 {
		log.Fatalf("Failed to write query to ShellLM server %v", err)
	}
	responseBuffer := make([]byte, 2048)
	bytesRead, err = conn.Read(responseBuffer)
	if err != nil || bytesRead <= 0 {
		log.Fatalf("Failed to read response from ShellLM server %v", err)
	}
	fmt.Println(string(responseBuffer))
}
