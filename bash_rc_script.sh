# --- ShellLM Frictionless Integration (v4 - Allowlist Edition) ---

# 1. Configuration
export SHELLLM_OUT="/tmp/shelllm.vlog"
export SHELLLM_SOCKET="/tmp/shelllm.socket"
touch "$SHELLLM_OUT"

# Save original terminal FDs
exec 3>&1
exec 4>&2

# 2. The Post-Execution Function
_shelllm_postexec() {
    local exit_code=$?

    # Only run logic if we actually started a capture
    if [[ "$SHELLLM_CAPTURING" == "true" ]]; then
        # Restore terminal control
        exec 1>&3 2>&4
        unset SHELLLM_CAPTURING

        # Allow tee to flush
        sleep 0.02

        if [[ -s "$SHELLLM_OUT" ]]; then
            local last_cmd=$(history 1 | sed 's/^[ ]*[0-9]*[ ]*//')
            local cmd_output=$(<"$SHELLLM_OUT")

            ( jq -n \
                --arg cmd "$last_cmd" \
                --arg out "$cmd_output" \
                --arg code "$exit_code" \
                '{Command: $cmd, Output: $out, ReturnCode: ($code|tonumber)}' | \
              nc -U -N -w 1 "$SHELLLM_SOCKET" >/dev/null 2>&1 ) & disown

            true > "$SHELLLM_OUT"
        fi
    fi
}

# 3. The Pre-Execution Function (Allowlist Logic)
_shelllm_preexec() {
    [[ -z "$BASH_COMMAND" ]] && return

    # Define your ALLOWLIST of safe commands here
    # Add any tools you want to track (e.g., docker, kubectl, python)
    local ALLOWED_TOOLS="^(ping|traceroute|ls|cat|echo|ip|git|grep|awk|sed|curl|wget|df|du|ps|whoami|pwd|date|docker|kubectl)"

    # Check if the command starts with an allowed tool
    if [[ "$BASH_COMMAND" =~ $ALLOWED_TOOLS ]]; then
        export SHELLLM_CAPTURING="true"
        # Line-buffered capture
        exec > >(stdbuf -oL tee -a "$SHELLLM_OUT") 2>&1
    fi
}

# 4. Global Shell Settings
set +m
shopt -s extdebug

# 5. Hooks
PROMPT_COMMAND="_shelllm_postexec"
trap '_shelllm_preexec' DEBUG
