#!/bin/bash
killall fastdown 2>/dev/null
sleep 1
./fastdown --output /tmp/ubuntu_test_final "magnet:?xt=urn:btih:3b245504cf5f11bbdbe1201cea6a6bf45aee1bc0&dn=ubuntu-24.04-desktop-amd64.iso&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=udp%3A%2F%2Fopen.tracker.cl%3A1337%2Fannounce" > /tmp/dl_out.log 2>&1 &
PID=$!
sleep 25
kill -INT $PID
cat /tmp/dl_out.log
