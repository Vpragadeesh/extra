#!/bin/bash
LOG_FILE=/tmp/fastdown_debug3.log ./fastdown ~/e/torrent-downloader > /tmp/fastdown_output3.log 2>&1 &
PID=$!
sleep 2
kill -INT $PID
cat /tmp/fastdown_output3.log
