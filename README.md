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

2.  **Download Dependencies:**
    ShellLM requires the ONNX Runtime, the LanceDB native libraries, and the pre-trained all-MiniLM-L6-v2 model. The included `Makefile` automates these downloads for your platform.

    **Note:** The initial build will prompt for confirmation as it may download ~500MB of dependencies.

    ```bash
    make build
    ```

3.  **Shell Integration:**
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
