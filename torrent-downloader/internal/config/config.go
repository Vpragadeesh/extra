package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Config holds application settings
type Config struct {
	OutputDir    string `yaml:"output_dir"`
	MaxSpeed     int64  `yaml:"max_speed"` // bytes per second
	Connections  int    `yaml:"connections"`
	ChunkSize    int64  `yaml:"chunk_size"`
	Timeout      int    `yaml:"timeout_seconds"`
	SaveResume   bool   `yaml:"save_resume"`
	ResumeFile   string `yaml:"resume_file"`
}

// Default returns the default configuration
func Default() *Config {
	return &Config{
		OutputDir:   "downloads",
		MaxSpeed:    0, // unlimited
		Connections: 100,
		ChunkSize:   524288, // 512KB
		Timeout:     30,
		SaveResume:  true,
		ResumeFile:  ".resume",
	}
}

// ExpandPath expands ~ to the home directory
func (c *Config) ExpandPath() error {
	if c.OutputDir != "" && strings.HasPrefix(c.OutputDir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		c.OutputDir = filepath.Join(home, c.OutputDir[1:])
	}
	return nil
}

