#!/bin/bash
# Bash completion for video_player.py

_video_player_complete() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Available options
    opts="--list -l --play -p --fzf -f --auto-play --directory -d --player --no-recursive -nr --show-hidden --version -v --no-color --quiet -q --help -h"

    case "${prev}" in
        --directory|-d)
            # Complete with directories
            COMPREPLY=( $(compgen -d -- ${cur}) )
            return 0
            ;;
        --player)
            # Complete with common video players
            COMPREPLY=( $(compgen -W "vlc mpv mplayer smplayer" -- ${cur}) )
            return 0
            ;;
        --play|-p|--auto-play)
            # Complete with video files in current directory
            local video_files=$(find . -maxdepth 1 -type f \( -iname "*.mp4" -o -iname "*.avi" -o -iname "*.mkv" -o -iname "*.mov" -o -iname "*.wmv" -o -iname "*.flv" -o -iname "*.webm" -o -iname "*.m4v" \) 2>/dev/null | sed 's|^\./||')
            COMPREPLY=( $(compgen -W "${video_files}" -- ${cur}) )
            return 0
            ;;
    esac

    # Complete with options
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
}

# Register completion function
complete -F _video_player_complete video_player.py
complete -F _video_player_complete ./video_player.py