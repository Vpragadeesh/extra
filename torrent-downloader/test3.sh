#!/bin/bash
./fastdown --output /tmp/test_dl3 "magnet:?xt=urn:btih:3b245504cf5f11bbdbe1201cea6a6bf45aee1bc0&dn=ubuntu-24.04-desktop-amd64.iso&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=udp%3A%2F%2Fopen.tracker.cl%3A1337%2Fannounce" > /tmp/ubuntu_output3.log 2>&1 &
PID=$!
sleep 20
kill -INT $PID
cat /tmp/ubuntu_output3.log
