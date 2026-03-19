# ShellLM

ShellLM is a local-first command history augmentor. It provides semantic search capabilities for shell sessions by recording commands and their outputs into a vector database for context-aware retrieval.

## Overview

Rather than relying on keyword-based history searches, ShellLM implements a local **Retrieval-Augmented Generation (RAG)** pipeline. It creates a vector embedding based on your shell commands and stdout using **all-MiniLM-L6-v2** and stores the results in **LanceDB**, a high-performance columnar vector database.

## Architecture

- **Capture:** Bash code to add to your .bashrc (`bash_rc_script.sh`) that transmits commands and output to the server over Unix sockets.
- **Server:** A Go-based orchestrator (`shelllm_server`) responsible for generating embeddings, writing to the DB, and handling queries.
- **Storage:** LanceDB for efficient, local vector storage and retrieval.
- **Inference:** Local execution of the ONNX runtime for sentence embeddings.

## Getting Started

### Prerequisites

- **Go 1.26+**
- **ONNX Runtime:** Shared libraries appropriate for your host platform (e.g., Linux x64 or Darwin arm64).
- **LanceDB C++ Headers:** Required for the CGO bindings.

### Installation

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/your-username/shelllm.git
    cd shelllm
    ```

2.  **Download ONNX Runtime (v1.24.3):**
    The server requires the ONNX Runtime shared library. Download the appropriate package from the [official releases](https://github.com/microsoft/onnxruntime/releases/tag/v1.24.3):
    - **Linux x64:** `onnxruntime-linux-x64-1.24.3.tgz`
    - **macOS ARM64:** `onnxruntime-osx-arm64-1.24.3.tgz`

    Extract the archive and ensure the resulting folder is in the project root (e.g., `onnxruntime-linux-x64-1.24.3/`).

3.  **Configure LanceDB Native Libraries:**
    ShellLM uses CGO bindings for LanceDB. You need the `liblancedb_go` shared library and headers:
    - **Headers:** Ensure `include/lancedb.h` is present. This is typically bundled with the `lancedb-go` source or can be retrieved from the [LanceDB repository](https://github.com/lancedb/lancedb).
    - **Shared Library:** Place the platform-specific `liblancedb_go.so` (Linux) or `liblancedb_go.dylib` (macOS) into the appropriate subdirectory:
        - Linux: `lib/linux_amd64/`
        - macOS (M5): `lib/darwin_arm64/`

4.  **Build the Server:**
    The project uses a `Makefile` to manage the CGO configuration and RPATH:
    ```bash
    make build
    ```

5.  **Shell Integration:**
    Source the provided integration script in your `.bashrc`:
    ```bash
    echo "source $(pwd)/bash_rc_script.sh" >> ~/.bashrc
    ```


### Usage

- **Start the background server:**
  ```bash
  make run
  ```
- **Querying:**
  Currently, querying is handled via the internal query socket. CLI-based query tools are under development.

## Maintenance

To remove binaries and temporary socket files:

```bash
make clean
```

## License

MIT
