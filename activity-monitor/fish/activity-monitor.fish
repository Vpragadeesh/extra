# Best-effort fish command metadata collector. Logs only the command's first token,
# not arguments or the full command line, to reduce accidental secret collection.
# Source this file from ~/.config/fish/config.fish.
#
# Configuration:
#   ACTIVITY_MONITOR_DISABLE=1  - Disable tracking entirely
#   ACTIVITY_MONITOR_TOKEN=xxx  - API token for authentication (get from dashboard)

# Opt-out: set ACTIVITY_MONITOR_DISABLE=1 in your environment to disable tracking
if test -n "$ACTIVITY_MONITOR_DISABLE"
    return
end

function __activity_monitor_preexec --on-event fish_preexec
    set -l api 'http://127.0.0.1:8765/api/events'
    set -l raw $argv[1]

    # Extract only the first token (executable name) from the command line.
    # Aliases, functions, expansions, and pipelines can make this approximate.
    # Use literal separators: this fish build's `string split -r` does not
    # reliably match whitespace classes.
    set -l normalized (string replace -a (printf '\t') ' ' -- $raw)
    set -l first (string split -m 1 ' ' -- (string trim -- $normalized))[1]
    # Drop trailing line separators so compound commands record the real
    # executable token ("begin" not "begin;"). Interior separators in
    # pipelines remain approximate by design.
    set -l command_name (string trim -- (string replace -r '[;&|]+$' '' -- $first))
    if test -z "$command_name"
        return
    end

    # Avoid tracking absurd values and accidental secret-shaped input.
    if test (string length -- $command_name) -ge 256
        return
    end

    set -l curl_bin (command -s curl)
    if test -z "$curl_bin"
        return
    end

    # Prefer python3 for correct JSON encoding (the backend is Python-based).
    set -l python_bin (command -s python3)
    if test -z "$python_bin"
        set python_bin (command -s python)
    end
    if test -z "$python_bin"
        return
    end

    set -l payload (
        $python_bin -c 'import json,sys; print(json.dumps({"source":"fish","event_type":"command","command":sys.argv[1],"cwd":sys.argv[2]}))' \
            "$command_name" "$PWD" 2>/dev/null
    )
    if test -z "$payload"
        return
    end

    set -l api_token $ACTIVITY_MONITOR_TOKEN
    set -l curl_cmd $curl_bin --silent --max-time 0.5 --connect-timeout 0.2 \
        -H 'Content-Type: application/json'
    if test -n "$api_token"
        set curl_cmd $curl_cmd -H "X-API-Token: $api_token"
    end
    set curl_cmd $curl_cmd -d "$payload" "$api"

    # Best-effort HTTP post; never block or break interactive shell use.
    # `disown` keeps the short-lived curl out of the job table so the shell
    # can exit without a "There are still jobs active" warning.
    $curl_cmd >/dev/null 2>&1 &
    disown
end
