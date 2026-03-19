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

# Download URLs
MODEL_PATH=src/internal/all_minilm/model.onnx
MODEL_URL=https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx

ifeq ($(OS),linux)
	ONNX_URL=https://github.com/microsoft/onnxruntime/releases/download/v1.24.3/onnxruntime-linux-x64-1.24.3.tgz
else
	ONNX_URL=https://github.com/microsoft/onnxruntime/releases/download/v1.24.3/onnxruntime-osx-arm64-1.24.3.tgz
endif

.PHONY: all build run clean help fetch-model fetch-onnx confirm

all: build

confirm:
	@echo -n "This may download several hundred MBs of dependencies. Continue? [y/N] " && read ans && [ $${ans:-N} = y ]

## fetch-onnx: Download and extract ONNX Runtime if missing
fetch-onnx:
	@if [ ! -d $(ONNX_DIR) ]; then \
		$(MAKE) confirm; \
		echo "Downloading ONNX Runtime from $(ONNX_URL)..."; \
		curl -L -o onnxruntime.tgz $(ONNX_URL); \
		tar -xzf onnxruntime.tgz; \
		rm onnxruntime.tgz; \
	fi

## fetch-model: Download the ONNX model if it is missing
fetch-model:
	@if [ ! -f $(MODEL_PATH) ]; then \
		if [ ! -d $(ONNX_DIR) ]; then $(MAKE) confirm; fi; \
		echo "Downloading $(MODEL_PATH) from Hugging Face..."; \
		curl -L -o $(MODEL_PATH) $(MODEL_URL); \
	fi

## build: Compile the shelllm_server binary
build: fetch-onnx fetch-model
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
