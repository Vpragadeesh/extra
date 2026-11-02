#!/bin/bash
./fastdown --output /tmp/ubuntu_test "magnet:?xt=urn:btih:3b245504cf5f11bbdbe1201cea6a6bf45aee1bc0&dn=ubuntu-24.04-desktop-amd64.iso" > /tmp/ubuntu_output.log 2>&1 &
PID=$!
sleep 20
kill -INT $PID
cat /tmp/ubuntu_output.log
