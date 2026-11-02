package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Connections != 100 {
		t.Errorf("Expected 100 connections, got %d", cfg.Connections)
	}
	if cfg.MaxSpeed != 0 {
		t.Errorf("Expected MaxSpeed 0, got %d", cfg.MaxSpeed)
	}
	if cfg.MaxUploadSpeed != 0 {
		t.Errorf("Expected MaxUploadSpeed 0 (auto), got %d", cfg.MaxUploadSpeed)
	}
	if cfg.Timeout != 120 {
		t.Errorf("Expected Timeout 120, got %d", cfg.Timeout)
	}
}

func TestExpandPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OutputDir = "~/testdir"

	err := cfg.ExpandPath()
	if err != nil {
		t.Fatalf("ExpandPath failed: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "testdir")

	if cfg.OutputDir != expected {
		t.Errorf("Expected %s, got %s", expected, cfg.OutputDir)
	}
}

// A non-zero default listen port makes a second instance fail with
// "address already in use", so the default must stay 0 (OS-assigned).
func TestDefaultListenPortIsZero(t *testing.T) {
	if got := DefaultConfig().ListenPort; got != 0 {
		t.Errorf("ListenPort = %d, want 0 (OS-assigned)", got)
	}
}
