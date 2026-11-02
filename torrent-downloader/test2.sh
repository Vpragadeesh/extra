#!/bin/bash
./fastdown ~/e/torrent-downloader > /tmp/fastdown_output4.log 2>&1 &
PID=$!
sleep 2
kill -INT $PID
cat /tmp/fastdown_output4.log
