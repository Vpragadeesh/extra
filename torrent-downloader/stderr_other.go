//go:build !(linux || freebsd || darwin)

package main

// configOpenLogDup is a no-op on unsupported platforms: fd 2 stays wherever it
// was, so library logs may still reach the terminal there.
func configOpenLogDup(string) int { return 0 }
