package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config holds application settings
type Config struct {
	OutputDir      string `yaml:"output_dir"`
	MaxSpeed       int64  `yaml:"max_speed"`        // bytes per second (0 = unlimited)
	MaxUploadSpeed int64  `yaml:"max_upload_speed"` // bytes per second (0 = auto: ~15% of download, or 1 MB/s if unlimited)
	Connections    int    `yaml:"connections"`
	ChunkSize      int64  `yaml:"chunk_size"`
	Timeout        int    `yaml:"timeout_seconds"`
	SaveResume     bool   `yaml:"save_resume"`
	ResumeFile     string `yaml:"resume_file"`
	// ListenPort is the TCP/UDP port for BitTorrent peer connections.
	// 0 (default) means the OS picks a free port — safe to run multiple instances.
	ListenPort int `yaml:"listen_port"`
	// LogFile receives ALL internal library output (tracker announce failures,
	// DHT chatter, peer errors, Go stack traces). It must never reach the
	// terminal — that would paint over the TUI. "none" discards everything.
	LogFile string `yaml:"log_file"`

	// Multi-network support
	NetworkPreferences map[string]NetworkPreference `yaml:"network_preferences"`
	GlobalSpeedLimit   int64                        `yaml:"global_speed_limit"` // 0 = unlimited
	SlowMode           bool                         `yaml:"slow_mode"`
	SlowModeSpeed      int64                        `yaml:"slow_mode_speed"` // bytes/sec when slow mode enabled
}

// ProxyConfig holds per-interface proxy settings.
type ProxyConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Type     string `yaml:"type"` // http, https, socks4, socks5
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// NetworkPreference holds per-network-interface settings.
type NetworkPreference struct {
	CustomName      string       `yaml:"custom_name"`
	ColorID         string       `yaml:"color_id"`
	Proxy           *ProxyConfig `yaml:"proxy"`
	Off             bool         `yaml:"off"`
	SpeedLimit      int64        `yaml:"speed_limit"`       // bytes/sec, 0 = unlimited
	DataLimit       int64        `yaml:"data_limit"`        // bytes, 0 = unlimited
	DataLimitPeriod string       `yaml:"data_limit_period"` // "day" | "week" | "month"
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		OutputDir:      "~/Videos/",
		MaxSpeed:       0, // unlimited
		MaxUploadSpeed: 0, // auto: ~15% of download, or 1 MB/s if unlimited
		Connections:    100,
		ChunkSize:      16 * 1024,
		Timeout:        120,
		SaveResume:     true,
		ResumeFile:     ".resume",
		// 0 = OS assigns a free port. Safe with multiple running instances;
		// peers learn the port via tracker/DHT announce either way.
		ListenPort: 0,
		// Internal logs must not mix with the TUI on the terminal.
		LogFile: "fastdown.log",

		// Multi-network defaults
		NetworkPreferences: make(map[string]NetworkPreference),
		GlobalSpeedLimit:   0,
		SlowMode:           false,
		SlowModeSpeed:      2 << 20, // 2 MiB/s
	}
}

// LoadConfig loads the configuration from a file or returns the default
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}
	// It's okay if the config file doesn't exist, just use defaults
	return cfg, nil
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
