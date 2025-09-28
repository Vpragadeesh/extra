#!/bin/bash

MUSIC_ROOT="."
LIKES_FILE="$MUSIC_ROOT/likes.txt"
DISLIKES_FILE="$MUSIC_ROOT/dislikes.txt"
mpv_socket="/tmp/mpv-musicplayer"

# Select default folder as 'Hariharan' if available, else prompt user
select_default_folder() {
    if [[ -d "$MUSIC_ROOT/Hariharan/" ]]; then
        FOLDER="Hariharan"
    else
        echo "Default folder 'Hariharan' not found. Please select a folder:"
        select_folder
    fi
}

# Interactive fuzzy folder selector using fzf
select_folder() {
    FOLDER=$(find "$MUSIC_ROOT" -mindepth 1 -maxdepth 1 -type d -printf "%f\n" | \
        fzf --prompt="🎵 Select Folder: " --height=40%)
    [[ -z "$FOLDER" ]] && exit 0
}

# Populate songs array with supported audio files within selected folder
get_songs() {
    shopt -s nullglob
    mapfile -t songs < <(find "$MUSIC_ROOT/$FOLDER" -maxdepth 1 -type f \
        \( -iname \*.mp3 -o -iname \*.wav -o -iname \*.flac -o -iname \*.aac -o -iname \*.ogg \) | sort)
    if [[ ${#songs[@]} -eq 0 ]]; then
        echo "No songs found in folder '$FOLDER'"
        exit 1
    fi
}

# Play the current song using mpv with IPC input enabled
play_song() {
    # Remove old socket file if exists
    [ -e "$mpv_socket" ] && rm "$mpv_socket"

    # Kill previous mpv process if still running and wait for it to terminate
    if [[ -n "$mpv_pid" ]] && kill -0 "$mpv_pid" 2>/dev/null; then
        kill "$mpv_pid" 2>/dev/null
        wait "$mpv_pid" 2>/dev/null
    fi

    # Start mpv in background
    # Prevent mpv from reading terminal input by redirecting stdin from /dev/null
    # Also silence mpv stdout/stderr so it doesn't print "No key binding found" messages
    # Prefer loading an mpv MPRIS plugin if present so playerctl can control mpv
    MPRIS_CANDIDATES=("$HOME/.config/mpv/scripts/mpris.so" "/usr/share/mpv/scripts/mpris.so" "/usr/local/share/mpv/scripts/mpris.so")
    MPRIS_ARG=""
    for p in "${MPRIS_CANDIDATES[@]}"; do
        if [[ -f "$p" ]]; then
            MPRIS_ARG=("--script" "$p")
            echo "Found MPRIS plugin at $p" > /tmp/music_player_debug.log
            break
        fi
    done
    if [[ -z "$MPRIS_ARG" ]]; then
        echo "MPRIS plugin not found." > /tmp/music_player_debug.log
    fi

    # Start mpv. Use --input-terminal=no as extra safeguard against mpv reading stdin.
    # Redirect stdin/stdout/stderr so it doesn't interfere with the UI.
    mpv --no-video --quiet --input-terminal=no --input-ipc-server="$mpv_socket" ${MPRIS_ARG[@]} "${songs[$index]}" </dev/null >/dev/null 2>&1 &
    mpv_pid=$!

    # Wait a bit to ensure mpv sets up IPC socket
    sleep 2
    needs_full_redraw=true
}

# Send JSON commands to mpv via socat and IPC socket
send_mpv_command() {
    echo "$1" | socat - "$mpv_socket" 2>/dev/null
}

# Get current playback position as percentage
get_playback_position() {
    if [[ -e "$mpv_socket" ]]; then
        local response=$(send_mpv_command '{ "command": ["get_property", "percent-pos"] }')
        if [[ "$response" =~ \"data\":([0-9]+\.?[0-9]*) ]]; then
            echo "${BASH_REMATCH[1]}"
        else
            echo "0"
        fi
    else
        echo "0"
    fi
}

# Get current time position and duration
get_time_info() {
    if [[ -e "$mpv_socket" ]]; then
        local pos_response=$(send_mpv_command '{ "command": ["get_property", "time-pos"] }')
        local dur_response=$(send_mpv_command '{ "command": ["get_property", "duration"] }')

        local current_time="0"
        local total_time="0"

        if [[ "$pos_response" =~ \"data\":([0-9]+\.?[0-9]*) ]]; then
            current_time="${BASH_REMATCH[1]}"
        fi

        if [[ "$dur_response" =~ \"data\":([0-9]+\.?[0-9]*) ]]; then
            total_time="${BASH_REMATCH[1]}"
        fi

        echo "$current_time $total_time"
    else
        echo "0 0"
    fi
}

# Format seconds to MM:SS
format_time() {
    local seconds=$1
    local mins=$((${seconds%.*}/60))
    local secs=$((${seconds%.*}%60))
    printf "%02d:%02d" $mins $secs
}

# Draw progress bar
# draw_progress_bar() {
#     local percent=$1
#     local width=50
#     local filled=$((percent * width / 100))
#     local empty=$((width - filled))
#
#     local bar=""
#     for ((i=0; i<filled; i++)); do
#         bar+="█"
#     done
#     for ((i=0; i<empty; i++)); do
#         bar+="░"
#     done
#
#     echo "$bar"
# }

draw_progress_bar() {
    local percent=$1
    local width=50
    local total_steps=$(( width * 8 ))
    local filled_steps=$(( (percent * total_steps + 50) / 100 ))
    local full_blocks=$(( filled_steps / 8 ))
    local partial_level=$(( filled_steps % 8 ))

    # Unicode blocks from empty to full (8 levels of partial blocks)
    local blocks=( "" "▏" "▎" "▍" "▌" "▋" "▊" "▉" "█" )

    local bar="["

    # Add full blocks
    local i
    for (( i=0; i<full_blocks; i++ )); do
        bar+="█"
    done

    # Add partial block if needed
    if (( partial_level > 0 )); then
        bar+="${blocks[partial_level]}"
    fi

    # Add empty space as░
    local empty_blocks=$(( width - full_blocks - ( partial_level > 0 ? 1 : 0 ) ))
    for (( i=0; i<empty_blocks; i++ )); do
        bar+="░"
    done

    bar+="] ${percent%.*}%"
    echo "$bar"
}


# Update only the progress bar at the last line
update_progress_display() {
    local position_percent=$1
    local current_pos=$2
    local total_duration=$3

    # Calculate total lines in interface
    local total_lines=$((2 + 1 + ${#songs[@]} + 1 + 4 + 1))  # header + divider + songs + divider + controls + progress line

    # Save cursor position
    echo -ne "\033[s"

    # Move to last line
    echo -ne "\033[${total_lines};1H"

    if [[ "$position_percent" != "0" ]] || [[ "$current_pos" != "0" ]]; then
        local progress_bar=$(draw_progress_bar ${position_percent%.*})
        local current_formatted=$(format_time $current_pos)
        local total_formatted=$(format_time $total_duration)

        # Clear the line and draw progress info
        echo -ne "\033[K\e[1;36m$progress_bar\e[0m \e[1;37m$current_formatted / $total_formatted (${position_percent%.*}%)\e[0m"
    else
        echo -ne "\033[K\e[1;33mLoading...\e[0m"
    fi

    # Restore cursor position
    echo -ne "\033[u"
}

# Draw the complete interface
draw_full_interface() {
    clear
    echo -e "\e[1;35m╔═══════════════════ NOW PLAYING ═══════════════════╗\e[0m"
    echo -e "\e[1;36m  $FOLDER/\e[1;33m${songs[$index]##*/}\e[0m"
    echo -e "\e[1;35m╠═══════════════════════════════════════════════════╣\e[0m"
    for i in "${!songs[@]}"; do
        if [[ $i -eq $index ]]; then
            printf " \e[1;32m▶ %s\e[0m\n" "${songs[$i]##*/}"
        else
            printf "  %s\n" "${songs[$i]##*/}"
        fi
    done
    echo -e "\e[1;35m╠═══════════════════════ CONTROLS ══════════════════════╣\e[0m"
    echo -e "  [\e[1;33ml\e[0m] like    [\e[1;31md\e[0m] dislike    [\e[1mn\e[0m] next"
    echo -e "  [\e[1mb\e[0m] skip −5s    [\e[1mf\e[0m] skip +5s    [\e[1mp/SPACE\e[0m] play/pause"
    echo -e "  [\e[1ms\e[0m] choose song    [\e[1mc\e[0m] change folder    [\e[1mq\e[0m] quit"
    echo -e "\e[1;35m╚═══════════════════════════════════════════════════╝\e[0m"
    echo  # Empty line for progress bar
}

# Interactive fuzzy song selector with fzf
choose_song() {
    selected_song=$(printf '%s\n' "${songs[@]}" | sed "s@.*/@@g" | \
        fzf --prompt="🎵 Select Song: " --height=40% --no-sort)
    for i in "${!songs[@]}"; do
        if [[ "${songs[$i]##*/}" == "$selected_song" ]]; then
            index=$i
            return
        fi
    done
}

# Clean up mpv process on script exit
cleanup() {
    if [[ -n "$mpv_pid" ]] && kill -0 "$mpv_pid" 2>/dev/null; then
        kill "$mpv_pid" 2>/dev/null
        wait "$mpv_pid" 2>/dev/null
    fi
    [ -e "$mpv_socket" ] && rm -f "$mpv_socket"
    # Reset cursor and clear screen
    echo -ne "\033[?25h"  # Show cursor
    clear
    echo "Goodbye!"
    exit 0
}

trap cleanup SIGINT SIGTERM

# --- Script Initialization ---

select_default_folder
get_songs

# Start with a random song
index=$(( RANDOM % ${#songs[@]} ))
play_song

# --- Main Loop with Progress Bar at Bottom ---

needs_full_redraw=true
last_progress_update=0

# Hide cursor for cleaner display
echo -ne "\033[?25l"

while true; do
    current_time=$(date +%s)

    # Full redraw when needed (song change, user input, etc.)
    if [[ "$needs_full_redraw" == true ]]; then
        draw_full_interface
        needs_full_redraw=false
        last_progress_update=0
    fi

    # Update progress bar every second at the bottom line
    if (( current_time - last_progress_update >= 1 )); then
        position_percent=$(get_playback_position)
        time_info=$(get_time_info)
        current_pos=$(echo $time_info | cut -d' ' -f1)
        total_duration=$(echo $time_info | cut -d' ' -f2)

        update_progress_display "$position_percent" "$current_pos" "$total_duration"
        last_progress_update=$current_time
    fi

    # Check if mpv process has ended (song finished)
    if [[ -n "$mpv_pid" ]] && ! kill -0 "$mpv_pid" 2>/dev/null; then
        # Song finished, advance to next song automatically
        ((index=(index+1)%${#songs[@]}))
        play_song
        continue
    fi

    # Read user input with timeout (non-blocking)
    if read -rsN1 -t 0.5 key; then
        case "$key" in
            l|L)
                echo "$FOLDER/${songs[$index]##*/}" >> "$LIKES_FILE"
                # Show temporary feedback above progress bar
                echo -ne "\033[s"
                local feedback_line=$(( 2 + 1 + ${#songs[@]} + 1 + 4 ))
                echo -ne "\033[${feedback_line};1H\033[K\e[1;32mLiked!\e[0m"
                echo -ne "\033[u"
                sleep 1
                # Clear feedback
                echo -ne "\033[s\033[${feedback_line};1H\033[K\033[u"
                ;;
            d|D)
                echo "$FOLDER/${songs[$index]##*/}" >> "$DISLIKES_FILE"
                # Show temporary feedback above progress bar
                echo -ne "\033[s"
                local feedback_line=$(( 2 + 1 + ${#songs[@]} + 1 + 4 ))
                echo -ne "\033[${feedback_line};1H\033[K\e[1;31mDisliked!\e[0m"
                echo -ne "\033[u"
                sleep 1
                # Clear feedback
                echo -ne "\033[s\033[${feedback_line};1H\033[K\033[u"
                ;;
            n|N)
                # Stop current playback and play next song
                if [[ -n "$mpv_pid" ]] && kill -0 "$mpv_pid" 2>/dev/null; then
                    kill "$mpv_pid" 2>/dev/null
                    wait "$mpv_pid" 2>/dev/null
                fi
                ((index=(index+1)%${#songs[@]}))
                play_song
                ;;
            s|S)
                if [[ -n "$mpv_pid" ]] && kill -0 "$mpv_pid" 2>/dev/null; then
                    kill "$mpv_pid" 2>/dev/null
                    wait "$mpv_pid" 2>/dev/null
                fi
                choose_song
                play_song
                ;;
            c|C)
                if [[ -n "$mpv_pid" ]] && kill -0 "$mpv_pid" 2>/dev/null; then
                    kill "$mpv_pid" 2>/dev/null
                    wait "$mpv_pid" 2>/dev/null
                fi
                select_folder
                get_songs
                index=0
                play_song
                ;;
            q|Q)
                cleanup
                ;;
            p|P|' ')
                # Toggle play/pause
                send_mpv_command '{ "command": ["cycle", "pause"] }' >/dev/null 2>&1
                ;;
            b|B)
                # Skip backward 5 seconds
                send_mpv_command '{ "command": ["seek", -5, "relative"] }' >/dev/null 2>&1
                ;;
            f|F)
                # Skip forward 5 seconds
                send_mpv_command '{ "command": ["seek", 5, "relative"] }' >/dev/null 2>&1
                ;;
            *)
                # Ignore unknown keys
                ;;
        esac
    fi
done
