# 1. Configuration
export SHELLLM_OUT="/tmp/shelllm.vlog"
export SHELLLM_SOCKET="/tmp/shelllm.socket"
touch "$SHELLLM_OUT"

# Save original terminal FDs (File Descriptors)
exec 3>&1
exec 4>&2

# 2. The Pre-Command Hook (Runs AFTER a command finishes)
_shelllm_precmd() {
    local exit_code=$?

    if [[ "$SHELLLM_CAPTURING" == "true" ]]; then
        # Restore terminal control to the original FDs
        exec 1>&3 2>&4
        unset SHELLLM_CAPTURING

        # Minimal pause for tee to flush the buffer
        sleep 0.02

        if [[ -s "$SHELLLM_OUT" ]]; then
            # Zsh history expansion for the last command executed
            local last_cmd=$history[$((HISTCMD-1))]
            local cmd_output=$(<"$SHELLLM_OUT")

            ( jq -n \
                --arg cmd "$last_cmd" \
                --arg out "$cmd_output" \
                --arg code "$exit_code" \
                '{Command: $cmd, Output: $out, ReturnCode: ($code|tonumber)}' | \
              nc -U -N -w 1 "$SHELLLM_SOCKET" >/dev/null 2>&1 ) &! 

            # Clear the log file for the next run
            true > "$SHELLLM_OUT"
        fi
    fi
}

# 3. The Pre-Execution Hook (Runs BEFORE a command starts)
_shelllm_preexec() {
    # $1 is the full command string in Zsh preexec
    local cmd="$1"
    
    # Define your ALLOWLIST
    local ALLOWED_TOOLS="^(docker|tail|file|dmesg|nc|lsof|ss|dig|ping|traceroute|ls|cat|echo|ip|git|jj|grep|awk|sed|curl|wget|df|du|ps|whoami|pwd|date|docker|kubectl)"

    # Use Zsh regex matching (~ =)
    if [[ "$cmd" =~ $ALLOWED_TOOLS ]]; then
        export SHELLLM_CAPTURING="true"
        # On macOS/Zsh, we use 'stdbuf' (if installed via brew) or simply tee
        # Note: If stdbuf isn't found, you can just use: exec > >(tee -a "$SHELLLM_OUT") 2>&1
        exec > >(stdbuf -oL tee -a "$SHELLLM_OUT") 2>&1
    fi
}

# 4. Register Hooks
# Zsh uses array-based hooks which are safer than overriding variables
autoload -Uz add-zsh-hook
add-zsh-hook precmd _shelllm_precmd
add-zsh-hook preexec _shelllm_preexec