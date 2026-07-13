#!/bin/bash

MUSIC_ROOT="/home/pragadeesh/Music"
LIKES_FILE="$MUSIC_ROOT/likes.txt"
DISLIKES_FILE="$MUSIC_ROOT/dislikes.txt"
numeric_prefix=""

# Select default folder as 'Unni_Menon' if available, else prompt user
select_default_folder() {
    if [[ -d "$MUSIC_ROOT/Unni_Menon/" ]]; then
        FOLDER="S.P.B Golden Melodies (Tamil)"
    else
        echo "Default folder 'Unni_Menon' not found. Please select a folder:"
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
        \( -iname \*.mp3 -o -iname \*.wav -o -iname \*.flac -o -iname \*.aac -o -iname \*.ogg \) -printf '%T@ %p\n' | sort -n | sed 's/^[0-9.]* //')
    if [[ ${#songs[@]} -eq 0 ]]; then
        echo "No songs found in folder '$FOLDER'"
        exit 1
    fi
}

# Play the current song using mpc/mpd
play_song() {
    local song_path="${songs[$index]}"

    # Ensure mpd/mpc state is clean, then load the single song and play it
    mpc stop >/dev/null 2>&1
    mpc clear >/dev/null 2>&1

    # Convert absolute path to relative path from music_directory
    local relative_path="${song_path#$MUSIC_ROOT/}"

    # Add the song (relative to music directory) and play
    mpc add "$relative_path" >/dev/null 2>&1
    mpc play >/dev/null 2>&1

    # Small pause to allow mpd to start playback
    sleep 0.3
    needs_full_redraw=true
}

# Get current playback position as percentage (from mpc/mpd)
get_playback_position() {
    local status_line
    status_line=$(mpc status 2>/dev/null | sed -n '2p')
    if [[ -z "$status_line" ]]; then
        echo "0"
        return
    fi
    if [[ "$status_line" =~ \(([0-9]+)%\) ]]; then
        echo "${BASH_REMATCH[1]}"
    else
        echo "0"
    fi
}

# Get current time position and duration (seconds)
get_time_info() {
    local status_line
    status_line=$(mpc status 2>/dev/null | sed -n '2p')
    if [[ -z "$status_line" ]]; then
        echo "0 0"
        return
    fi
    if [[ "$status_line" =~ ([0-9]+:[0-5][0-9])/([0-9]+:[0-5][0-9]) ]]; then
        local cur="${BASH_REMATCH[1]}"
        local tot="${BASH_REMATCH[2]}"
        # Strip leading zeros to avoid octal interpretation (e.g. 09 -> 9)
        local cur_min=$((10#${cur%%:*}))
        local cur_sec=$((10#${cur##*:}))
        local tot_min=$((10#${tot%%:*}))
        local tot_sec=$((10#${tot##*:}))
        local cur_s=$(( cur_min * 60 + cur_sec ))
        local tot_s=$(( tot_min * 60 + tot_sec ))
        echo "$cur_s $tot_s"
    else
        echo "0 0"
    fi
}

# Get song technical info using ffprobe
get_song_info() {
    local song_path="${songs[$index]}"
    if [[ -f "$song_path" ]]; then
        # Get file size in human-readable format (MB)
        local size_bytes=$(ffprobe -v error -show_entries format=size -of default=noprint_wrappers=1:nokey=1 "$song_path")
        local size_mb=$(awk "BEGIN {printf \"%.2f MB\", $size_bytes / (1024*1024)}")

        # Get audio stream info using a single ffprobe call
        local stream_info=$(ffprobe -v error -select_streams a:0 -show_entries stream=sample_rate,bit_rate,bits_per_raw_sample -of default=noprint_wrappers=1:nokey=1 "$song_path")

        local sample_rate=$(echo "$stream_info" | sed -n '1p')
        local bit_rate=$(echo "$stream_info" | sed -n '2p')
        local bit_depth=$(echo "$stream_info" | sed -n '3p')

        # Format for display
        [[ -z "$sample_rate" || "$sample_rate" == "N/A" ]] && sample_rate="-" || sample_rate="$((sample_rate/1000)) kHz"
        [[ -z "$bit_rate" || "$bit_rate" == "N/A" ]] && bit_rate="-" || bit_rate="$((bit_rate/1000)) kbps"
        [[ -z "$bit_depth" || "$bit_depth" == "N/A" || "$bit_depth" == "0" ]] && bit_depth="-" || bit_depth+="-bit"

        echo "$size_mb|$sample_rate|$bit_rate|$bit_depth"
    else
        echo "- MB|- kHz|- kbps|- "
    fi
}

# Format seconds to MM:SS
format_time() {
    local seconds=${1:-0}
    # Strip leading zeros and non-numeric characters
    seconds=${seconds%%.*}
    seconds=${seconds##+([!0-9])}
    seconds=${seconds:-0}
    local mins=$((10#$seconds / 60))
    local secs=$((10#$seconds % 60))
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
    local total_lines=$((3 + 1 + 1 + ${#songs[@]} + 1 + 5 + 1 + 1))  # header + song info + divider + songs + divider + controls + progress line

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
    local song_info_str=$1
    local size=$(echo "$song_info_str" | cut -d'|' -f1)
    local sample_rate=$(echo "$song_info_str" | cut -d'|' -f2)
    local bit_rate=$(echo "$song_info_str" | cut -d'|' -f3)
    local bit_depth=$(echo "$song_info_str" | cut -d'|' -f4)

    clear
    echo -e "\e[1;35m╔════════════════════ PLAYLIST ═════════════════════╗\e[0m"
    local total=${#songs[@]}
    for i in "${!songs[@]}"; do
        # Relative numbering like neovim: 0 for current, distance for others
        local rel=$(( i - index ))
        [[ $rel -lt 0 ]] && rel=$(( -rel ))  # absolute value
        if [[ $i -eq $index ]]; then
            printf " %3d \e[1;32m▶ %s\e[0m\n" "$rel" "${songs[$i]##*/}"
        else
            printf " %3d   %s\n" "$rel" "${songs[$i]##*/}"
        fi
    done
    echo -e "\e[1;35m╠═══════════════════════ CONTROLS ══════════════════════╣\e[0m"
    echo -e "  [\e[1;33mf\e[0m] like    [\e[1;31md\e[0m] dislike    [\e[1;31mD\e[0m] delete    [\e[1;32mj\e[0m] next    [\e[1;32mk\e[0m] previous    [\e[1;35mm\e[0m] move"
    echo -e "  [\e[1mh\e[0m] skip −5s    [\e[1ml\e[0m] skip +5s    [\e[1mp/SPACE\e[0m] play/pause"
    echo -e "  [\e[1ms\e[0m] choose song    [\e[1mc\e[0m] change folder    [\e[1;q\e[0m] quit"
    # Show numeric prefix if any (vim-style count)
    if [[ -n "$numeric_prefix" ]]; then
        echo -e "  \e[1;33mPrefix:\e[0m $numeric_prefix"
    fi
    echo -e "\e[1;35m╠═══════════════════ NOW PLAYING ═══════════════════╣\e[0m"
    echo -e "  $FOLDER/\e[1;33m${songs[$index]##*/}\e[0m"
    echo -e "  \e[1;34mSize:\e[0m $size   \e[1;34mSample Rate:\e[0m $sample_rate   \e[1;34mBitrate:\e[0m $bit_rate   \e[1;34mBit Depth:\e[0m $bit_depth"
    echo -e "\e[1;35m╚═════════════════════════════════════════════════════╝\e[0m"
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

# Move the current song to another folder (selected via fzf)
move_song() {
    local src_path="${songs[$index]}"
    local base_name="${src_path##*/}"

    # Let user pick destination folder (name only)
    local dest=$(find "$MUSIC_ROOT" -mindepth 1 -maxdepth 1 -type d -printf "%f\n" | \
        fzf --prompt="🎯 Move to folder: " --height=40% --no-sort)
    [[ -z "$dest" ]] && return 1

    # If same folder, nothing to do
    if [[ "$dest" == "$FOLDER" ]]; then
        return 2
    fi

    # Stop current playback to avoid issues while moving file
    mpc stop >/dev/null 2>&1

    mkdir -p "$MUSIC_ROOT/$dest"
    local dest_path="$MUSIC_ROOT/$dest/$base_name"

    # Avoid overwriting existing file: if exists, append suffix
    if [[ -e "$dest_path" ]]; then
        local name="${base_name%.*}"
        local ext="${base_name##*.}"
        local i=1
        while [[ -e "$MUSIC_ROOT/$dest/$name-$i.$ext" ]]; do
            ((i++))
        done
        dest_path="$MUSIC_ROOT/$dest/$name-$i.$ext"
    fi

    if ! mv -- "$src_path" "$dest_path" 2>/dev/null; then
        return 3
    fi

    # Remember destination for feedback
    MOVE_DEST="$dest"

    # Refresh song list for the current folder (do not change FOLDER)
    local old_index=$index
    get_songs

    # If the moved song was removed from the list, keep index pointing to the next song
    if (( old_index >= ${#songs[@]} )); then
        index=0
    else
        index=$old_index
    fi

    # Start playback of the current index (next song in same folder)
    play_song
    return 0
}

cleanup() {
    mpc stop >/dev/null 2>&1
    mpc clear >/dev/null 2>&1
    # Reset cursor and clear screen
    echo -ne "\033[?25h"  # Show cursor
    clear
    echo "Goodbye!"
    exit 0
}

trap cleanup SIGINT SIGTERM

# --- Script Initialization ---

# Sync mpd database so all files on disk are available
mpc update --quiet 2>/dev/null
sleep 1

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
        song_info=$(get_song_info)
        draw_full_interface "$song_info"
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

    # Check mpd/mpc playback state; if playback stopped, advance to next song
    status_line=$(mpc status 2>/dev/null | sed -n '2p')
    if [[ -z "$status_line" ]] || (! echo "$status_line" | grep -q '\[playing\]' && ! echo "$status_line" | grep -q '\[paused\]'); then
        ((index=(index+1)%${#songs[@]}))
        play_song
        continue
    fi

    # Read user input with timeout (non-blocking)
    if read -rsN1 -t 0.1 key;
    then
        # Collect numeric prefix (vim-style counts)
        if [[ "$key" =~ [0-9] ]]; then
            # avoid leading zeros making it empty; allow 0 as part of multi-digit
            numeric_prefix+="$key"
            needs_full_redraw=true
            continue
        fi

        case "$key" in
            f|F|+)
                echo "$FOLDER/${songs[$index]##*/}" >> "$LIKES_FILE"
                # Show temporary feedback above progress bar
                echo -ne "\033[s"
                feedback_line=$((3 + 1 + 1 + ${#songs[@]} + 1 + 5))
                echo -ne "\033[${feedback_line};1H\033[K\e[1;32mLiked!\\e[0m"
                echo -ne "\033[u"
                sleep 0.3
                # Clear feedback
                echo -ne "\033[s\033[${feedback_line};1H\033[K\033[u"
                ;;
            d|-)
                echo "$FOLDER/${songs[$index]##*/}" >> "$DISLIKES_FILE"
                # Show temporary feedback above progress bar
                echo -ne "\033[s"
                feedback_line=$((3 + 1 + 1 + ${#songs[@]} + 1 + 5))
                echo -ne "\033[${feedback_line};1H\033[K\e[1;32mDisliked!\\e[0m"
                echo -ne "\033[u"
                sleep 0.3
                # Clear feedback
                echo -ne "\033[s\033[${feedback_line};1H\033[K\033[u"
                ;;
            D)
                # Confirm deletion using prompt line
                prompt_line=$((3 + 1 + 1 + ${#songs[@]} + 1 + 5))
                echo -ne "\033[s"
                echo -ne "\033[${prompt_line};1H\033[K"
                echo -ne "\033[?25h"
                read -p "Delete ${songs[$index]##*/}? (y/N): " -n1 -r delete_choice
                echo -ne "\033[?25l"
                echo -ne "\033[u"

                # Clear the prompt line after reading input
                echo -ne "\033[s"
                echo -ne "\033[${prompt_line};1H\033[K"
                echo -ne "\033[u"

                if [[ "$delete_choice" =~ ^[Yy]$ ]]; then
                    # Stop current playback if running
                    mpc stop >/dev/null 2>&1

                    # Delete file
                    if rm -f -- "${songs[$index]}" 2>/dev/null; then
                        # Refresh song list and play next
                        old_index=$index
                        get_songs
                        if (( ${#songs[@]} == 0 )); then
                            # No songs left
                            echo -ne "\033[s"
                            echo -ne "\033[${prompt_line};1H\033[K\e[1;32mDeleted. No songs left. Exiting...\e[0m"
                            echo -ne "\033[u"
                            sleep 1
                            cleanup
                        else
                            if (( old_index >= ${#songs[@]} )); then
                                index=0
                            else
                                index=$old_index
                            fi
                            play_song
                            echo -ne "\033[s"
                            echo -ne "\033[${prompt_line};1H\033[K\e[1;32mDeleted!\e[0m"
                            echo -ne "\033[u"
                            sleep 0.3
                            echo -ne "\033[s\033[${prompt_line};1H\033[K\033[u"
                        fi
                    else
                        echo -ne "\033[s"
                        echo -ne "\033[${prompt_line};1H\033[K\e[1;31mDelete failed\e[0m"
                        echo -ne "\033[u"
                        sleep 0.3
                        echo -ne "\033[s\033[${prompt_line};1H\033[K\033[u"
                    fi
                else
                    # Cancelled
                    echo -ne "\033[s"
                    echo -ne "\033[${prompt_line};1H\033[K\e[1;33mDelete cancelled\e[0m"
                    echo -ne "\033[u"
                    sleep 0.3
                    echo -ne "\033[s\033[${prompt_line};1H\033[K\033[u"
                fi
                ;;
            m|M)
                # Move current song to another folder
                move_song
                ret=$?
                echo -ne "\033[s"
                feedback_line=$((3 + 1 + 1 + ${#songs[@]} + 1 + 5))
                if [[ $ret -eq 0 ]]; then
                    echo -ne "\033[${feedback_line};1H\033[K\e[1;32mMoved to $FOLDER!\e[0m"
                elif [[ $ret -eq 1 ]]; then
                    echo -ne "\033[${feedback_line};1H\033[K\e[1;33mMove cancelled\e[0m"
                elif [[ $ret -eq 2 ]]; then
                    echo -ne "\033[${feedback_line};1H\033[K\e[1;33mAlready in that folder\e[0m"
                else
                    echo -ne "\033[${feedback_line};1H\033[K\e[1;31mMove failed\e[0m"
                fi
                echo -ne "\033[u"
                sleep 0.3
                # Clear feedback
                echo -ne "\033[s\033[${feedback_line};1H\033[K\033[u"
                ;;
            j|J)
                # Move down by numeric_prefix (default 1)
                cnt=1
                if [[ -n "$numeric_prefix" ]]; then
                    cnt=$numeric_prefix
                fi
                numeric_prefix=""
                mpc stop >/dev/null 2>&1
                ((index=(index+cnt)%${#songs[@]}))
                play_song
                ;;
            k|K)
                # Move up by numeric_prefix (default 1)
                cnt=1
                if [[ -n "$numeric_prefix" ]]; then
                    cnt=$numeric_prefix
                fi
                numeric_prefix=""
                mpc stop >/dev/null 2>&1
                ((index=(index-cnt+${#songs[@]})%${#songs[@]}))
                play_song
                ;;
            s|S)
                mpc stop >/dev/null 2>&1
                choose_song
                play_song
                ;;
            c|C)
                mpc stop >/dev/null 2>&1
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
                mpc toggle >/dev/null 2>&1
                ;;
            h|H)
                # Skip backward 5 seconds
                mpc seek -5 >/dev/null 2>&1
                ;;
            l|L)
                # Skip forward 5 seconds
                mpc seek +5 >/dev/null 2>&1 || mpc seek 5 >/dev/null 2>&1
                ;;
            /)
                # Use the feedback line for prompting
                prompt_line=$((3 + 1 + 1 + ${#songs[@]} + 1 + 5))
                echo -ne "\033[s" # save cursor
                echo -ne "\033[${prompt_line};1H\033[K" # move to line and clear it
                echo -ne "\033[?25h" # show cursor
                read -p "Seek to (mm:ss): " timestamp_input
                echo -ne "\033[?25l" # hide cursor
                echo -ne "\033[u" # restore cursor

                # Clear the prompt line after reading input
                echo -ne "\033[s"
                echo -ne "\033[${prompt_line};1H\033[K"
                echo -ne "\033[u"

                if [[ "$timestamp_input" =~ ^[0-9]+:[0-5][0-9]$ ]]; then
                    minutes=$(echo "$timestamp_input" | cut -d: -f1)
                    seconds=$(echo "$timestamp_input" | cut -d: -f2)
                    total_seconds=$((minutes * 60 + seconds))
                    mpc seek $total_seconds >/dev/null 2>&1
                elif [[ -n "$timestamp_input" ]]; then # show error only if user typed something
                    # Show error message for a moment
                    echo -ne "\033[s"
                    echo -ne "\033[${prompt_line};1H\033[K"
                    echo -ne "\e[1;31mInvalid format! (mm:ss)\e[0m"
                    echo -ne "\033[u"
                    sleep 1.5
                    # Clear the error message
                    echo -ne "\033[s"
                    echo -ne "\033[${prompt_line};1H\033[K"
                    echo -ne "\033[u"
                fi
                ;;
            *)
                # Ignore unknown keys
                ;;
        esac
    fi
done
