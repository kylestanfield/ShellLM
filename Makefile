# ShellLM Makefile

BINARY_NAME=shelllm_server
GO_FILES=src/main.go

# OS and Architecture detection
OS=$(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(shell uname -m)

ifeq ($(ARCH),x86_64)
    GO_ARCH=amd64
else ifeq ($(ARCH),arm64)
    GO_ARCH=arm64
else
    GO_ARCH=$(ARCH)
endif

# Library paths
LIB_DIR=$(CURDIR)/lib/$(OS)_$(GO_ARCH)
INC_DIR=$(CURDIR)/include

# Default ONNX path (adjust as needed for Mac)
ONNX_VERSION=1.24.3
ifeq ($(ARCH),x86_64)
    ONNX_ARCH=x64
else
    ONNX_ARCH=$(ARCH)
endif
ONNX_DIR=$(CURDIR)/onnxruntime-$(OS)-$(ONNX_ARCH)-$(ONNX_VERSION)

ifeq ($(OS),darwin)
    LIB_EXT=dylib
else
    LIB_EXT=so
endif

ONNX_LIB=$(ONNX_DIR)/lib/libonnxruntime.$(LIB_EXT)

.PHONY: all build run clean help

all: build

## build: Compile the shelllm_server binary
build:
	@echo "Building for $(OS)_$(GO_ARCH)..."
	CGO_CFLAGS="-I$(INC_DIR)" \
	CGO_LDFLAGS="-L$(LIB_DIR) -Wl,-rpath,$(LIB_DIR)" \
	go build -o $(BINARY_NAME) $(GO_FILES)

## run: Build and run the server
run: build
	@echo "Running $(BINARY_NAME)..."
	ONNXRUNTIME_LIB_PATH=$(ONNX_LIB) \
	LD_LIBRARY_PATH=$(LIB_DIR):$(LD_LIBRARY_PATH) \
	DYLD_LIBRARY_PATH=$(LIB_DIR):$(DYLD_LIBRARY_PATH) \
	./$(BINARY_NAME)

## clean: Remove binaries and temporary runtime files
clean:
	@echo "Cleaning up..."
	go clean
	rm -f $(BINARY_NAME)
	rm -f /tmp/shelllm.history.socket
	rm -f /tmp/shelllm.query.socket
	rm -f /tmp/shelllm.socket
	# Caution: removing database
	# rm -rf /tmp/testdb

help:
	@echo "Available targets:"
	@sed -n 's/^##//p' Makefile
