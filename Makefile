# ShellLM Makefile

DAEMON_BINARY=shelllm-daemon
CLI_BINARY=slm

# OS and Architecture detection
OS=$(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(shell uname -m)

ifeq ($(ARCH),x86_64)
    GO_ARCH=amd64
    ONNX_ARCH=x64
else ifeq ($(ARCH),arm64)
    GO_ARCH=arm64
    ONNX_ARCH=arm64
else
    GO_ARCH=$(ARCH)
    ONNX_ARCH=$(ARCH)
endif

# Library paths
LIB_DIR=$(CURDIR)/lib/$(OS)_$(GO_ARCH)
INC_DIR=$(CURDIR)/include

# Default ONNX path
ONNX_VERSION=1.24.3
ifeq ($(OS),darwin)
    ONNX_OS=osx
else
    ONNX_OS=$(OS)
endif
ONNX_DIR=$(CURDIR)/onnxruntime-$(ONNX_OS)-$(ONNX_ARCH)-$(ONNX_VERSION)

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

LANCED_VERSION=v0.1.2
LANCED_URL=https://github.com/lancedb/lancedb-go/releases/download/$(LANCED_VERSION)/liblancedb_go.a
LANCED_H_URL=https://github.com/lancedb/lancedb-go/releases/download/$(LANCED_VERSION)/lancedb.h

.PHONY: all build build-daemon build-cli run clean help fetch-model fetch-onnx fetch-lancedb confirm install

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

## fetch-lancedb: Download LanceDB native library if missing
fetch-lancedb:
	@if [ ! -f $(LIB_DIR)/liblancedb_go.a ] || [ $$(stat -f%z $(LIB_DIR)/liblancedb_go.a 2>/dev/null || stat -c%s $(LIB_DIR)/liblancedb_go.a) -lt 100 ]; then \
		$(MAKE) confirm; \
		echo "Downloading LanceDB native library from $(LANCED_URL)..."; \
		mkdir -p $(LIB_DIR); \
		curl -L -o $(LIB_DIR)/liblancedb_go.a $(LANCED_URL); \
	fi
	@if [ ! -f $(INC_DIR)/lancedb.h ]; then \
		echo "Downloading lancedb.h..."; \
		mkdir -p $(INC_DIR); \
		curl -L -o $(INC_DIR)/lancedb.h $(LANCED_H_URL); \
	fi

## fetch-model: Download the ONNX model if it is missing
fetch-model:
	@if [ ! -f $(MODEL_PATH) ]; then \
		if [ ! -d $(ONNX_DIR) ]; then $(MAKE) confirm; fi; \
		echo "Downloading $(MODEL_PATH) from Hugging Face..."; \
		curl -L -o $(MODEL_PATH) $(MODEL_URL); \
	fi

## build: Compile both daemon and cli binaries
build: build-daemon build-cli

## build-daemon: Compile the shelllm-daemon binary
build-daemon: fetch-onnx fetch-lancedb fetch-model
	@echo "Building daemon for $(OS)_$(GO_ARCH)..."
	CGO_CFLAGS="-I$(INC_DIR) -I$(ONNX_DIR)/include" \
	CGO_LDFLAGS="-L$(LIB_DIR) -llancedb_go -L$(ONNX_DIR)/lib -Wl,-rpath,$(ONNX_DIR)/lib -framework Security -framework CoreFoundation" \
	go build -o $(DAEMON_BINARY) src/daemon/daemon.go

## dev: Run the daemon with debug logging enabled
dev: fetch-onnx fetch-lancedb fetch-model
	@echo "Building daemon with debug tags..."
	CGO_CFLAGS="-I$(INC_DIR) -I$(ONNX_DIR)/include" \
	CGO_LDFLAGS="-L$(LIB_DIR) -llancedb_go -L$(ONNX_DIR)/lib -Wl,-rpath,$(ONNX_DIR)/lib -framework Security -framework CoreFoundation" \
	go build -tags debug -o $(DAEMON_BINARY) src/daemon/daemon.go src/daemon/log_dev.go src/daemon/log_prod.go
	ONNXRUNTIME_LIB_PATH=$(ONNX_LIB) ./$(DAEMON_BINARY)

## build-cli: Compile the slm CLI binary
build-cli:
	@echo "Building CLI..."
	go build -o $(CLI_BINARY) src/cli/slm.go

## install: Install the slm binary to /usr/local/bin
install: build-cli
	@echo "Installing $(CLI_BINARY) to /usr/local/bin..."
	sudo cp $(CLI_BINARY) /usr/local/bin/$(CLI_BINARY)

## run: Build and run the daemon
run: build-daemon
	@echo "Running $(DAEMON_BINARY)..."
	ONNXRUNTIME_LIB_PATH=$(ONNX_LIB) \
	./$(DAEMON_BINARY)

## clean: Remove binaries and temporary runtime files
clean:
	@echo "Cleaning up..."
	go clean
	rm -f $(DAEMON_BINARY) $(CLI_BINARY)
	rm -f /tmp/shelllm.history.socket
	rm -f /tmp/shelllm.query.socket
	rm -f /tmp/shelllm.socket
	# Caution: removing database
	# rm -rf /tmp/testdb

help:
	@echo "Available targets:"
	@sed -n 's/^##//p' Makefile
