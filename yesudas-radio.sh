#!/usr/bin/env bash

STREAM_URL="https://stream-169.zeno.fm/uvdbygm6a48uv?zt=..."
LOG_FILE="$HOME/yesudas-radio-log.txt"

echo "🎵 Now playing: KJ Yesudas Radio"
echo "📝 Logging songs to: $LOG_FILE"

mpv "$STREAM_URL"
while IFS= read -r line; do
    title=$(echo "$line" | sed -n 's/.*icy-title: //p')
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $title" >> "$LOG_FILE"
    echo "🎶 $title"
done
