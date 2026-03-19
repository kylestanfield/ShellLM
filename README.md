# ShellLM

ShellLM is a local-first command history augmentor. It provides semantic search capabilities for shell sessions by recording commands and their outputs into a vector database for context-aware retrieval.

## Overview

Rather than relying on keyword-based history searches, ShellLM implements a local **Retrieval-Augmented Generation (RAG)** pipeline. It creates a vector embedding based on your shell commands and stdout using **all-MiniLM-L6-v2** and stores the results in **LanceDB**, a high-performance columnar vector database.

## Architecture

- **Capture:** Shell hooks (`bash_rc_script.sh` and `zsh_rc.sh`) transmit commands and output to the daemon over Unix sockets.
- **Daemon:** A Go-based orchestrator (`shelllm-daemon`) responsible for generating embeddings, writing to the DB, and handling queries.
- **CLI:** A lightweight frontend (`slm`) for querying your history using natural language.
- **Storage:** LanceDB for efficient, local vector storage and retrieval.
- **Inference:** Local execution of the ONNX runtime for sentence embeddings.

## Getting Started

### Prerequisites

- **Go 1.26+**
- **ONNX Runtime & LanceDB:** The `Makefile` will automatically download the correct shared libraries for your platform (Linux/macOS).

### Installation

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/kylestanfield/shelllm.git
    cd shelllm
    ```

2.  **Build and Install:**
    This will download necessary models and libraries (~500MB), compile the binaries, and install the `slm` command to `/usr/local/bin`.

    ```bash
    make build
    sudo make install
    ```

3.  **Shell Integration:**
    Add the hook to your shell configuration file.

    **For Bash (`~/.bashrc`):**
    ```bash
    source /path/to/shelllm/bash_rc_script.sh
    ```

    **For Zsh (`~/.zshrc`):**
    ```zsh
    source /path/to/shelllm/zsh_rc.sh
    ```

### Usage

1.  **Start the Daemon:**
    The daemon must be running to capture history and answer queries.
    ```bash
    make run
    ```
    *Note: In production mode, the daemon runs silently.*

2.  **Querying:**
    Use the `slm` command to ask questions about your shell history.
    ```bash
    slm "how do I untar a file?"
    slm why did my last docker build fail?
    ```

### Development

To run the daemon with verbose logging enabled:

```bash
make dev
```

## Maintenance

To remove binaries and temporary socket files (but keep the downloaded libraries and model):

```bash
make clean
```

## License

MIT
