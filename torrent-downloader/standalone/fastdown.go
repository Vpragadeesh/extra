package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

// Config holds application settings
type Config struct {
	OutputDir   string `yaml:"output_dir"`
	MaxSpeed    int64  `yaml:"max_speed"` // bytes per second
	Connections int    `yaml:"connections"`
	ChunkSize   int64  `yaml:"chunk_size"`
	Timeout     int    `yaml:"timeout_seconds"`
	SaveResume  bool   `yaml:"save_resume"`
	ResumeFile  string `yaml:"resume_file"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		OutputDir:   "/home/pragadeesh/Videos/",
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

// Limiter provides bandwidth rate limiting
type Limiter struct {
	maxBytesPerSec int64
	lastTime       time.Time
	bytesSent      int64
}

// NewLimiter creates a new bandwidth limiter
func NewLimiter(maxBytesPerSec int64) *Limiter {
	return &Limiter{
		maxBytesPerSec: maxBytesPerSec,
		lastTime:       time.Now(),
	}
}

// Wait blocks if necessary to enforce the rate limit
func (l *Limiter) Wait(bytes int64) {
	if l.maxBytesPerSec <= 0 {
		return // No limit
	}

	l.bytesSent += bytes
	now := time.Now()
	elapsed := now.Sub(l.lastTime).Seconds()

	if elapsed >= 1.0 {
		l.bytesSent = 0
		l.lastTime = now
		return
	}

	allowedBytes := int64(float64(l.maxBytesPerSec) * elapsed)
	if l.bytesSent > allowedBytes {
		sleepTime := time.Duration(float64(time.Second) * float64(l.bytesSent-allowedBytes) / float64(l.maxBytesPerSec))
		time.Sleep(sleepTime)
	}
}

// Worker handles chunk downloads
type Worker struct {
	id     int
	file   *os.File
	offset int64
	size   int64
	done   chan error
}

// NewWorker creates a new worker
func NewWorker(id int, file *os.File, offset, size int64) *Worker {
	return &Worker{
		id:     id,
		file:   file,
		offset: offset,
		size:   size,
		done:   make(chan error, 1),
	}
}

// Download downloads a chunk
func (w *Worker) Download(reader io.Reader) error {
	if _, err := w.file.Seek(w.offset, 0); err != nil {
		return err
	}

	_, err := io.CopyN(w.file, reader, w.size)
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

// Manager handles torrent downloads
type Manager struct {
	client  *torrent.Client
	config  *Config
	limiter *Limiter
	workers []*Worker
}

// NewManager creates a new download manager
func NewManager(client *torrent.Client, cfg *Config) *Manager {
	limiter := NewLimiter(cfg.MaxSpeed)
	return &Manager{
		client:  client,
		config:  cfg,
		limiter: limiter,
		workers: make([]*Worker, cfg.Connections),
	}
}

// Download starts a torrent download
func (m *Manager) Download(ctx context.Context, resource string) error {
	var t *torrent.Torrent

	// Parse magnet or torrent file
	if len(resource) > 7 && resource[:7] == "magnet:" {
		spec, err := torrent.TorrentSpecFromMagnetUri(resource)
		if err != nil {
			return fmt.Errorf("invalid magnet URI: %w", err)
		}
		var err2 error
		t, _, err2 = m.client.AddTorrentSpec(spec)
		if err2 != nil {
			return fmt.Errorf("failed to add torrent: %w", err2)
		}
	} else {
		// For torrent files, read and get the metadata
		data, err := os.ReadFile(resource)
		if err != nil {
			return fmt.Errorf("failed to read torrent file: %w", err)
		}
		mi, err := metainfo.Load(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("failed to parse torrent file: %w", err)
		}
		// Create a minimal spec from the metadata
		spec := &torrent.TorrentSpec{
			Trackers: mi.AnnounceList,
		}
		var err2 error
		t, _, err2 = m.client.AddTorrentSpec(spec)
		if err2 != nil {
			return fmt.Errorf("failed to add torrent: %w", err2)
		}
	}

	// Create output directory
	if err := os.MkdirAll(m.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Wait for metadata if needed
	fmt.Println("[*] Waiting for torrent metadata...")
	select {
	case <-t.GotInfo():
		// Metadata received
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		// Give it a bit longer
		fmt.Println("[*] Still waiting for metadata (this can take a while with magnet links)...")
		<-t.GotInfo()
	}

	// Set up progress bar
	info := t.Info()
	if info == nil {
		return fmt.Errorf("failed to get torrent info")
	}

	fmt.Printf("[+] Starting download: %s\n", info.Name)
	fmt.Printf("[+] Total size: %.2f MB\n", float64(info.TotalLength())/1024/1024)
	fmt.Printf("[+] Output directory: %s\n", m.config.OutputDir)

	bar := progressbar.NewOptions64(
		info.TotalLength(),
		progressbar.OptionSetDescription("[cyan][downloading][reset]"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(50),
		progressbar.OptionThrottle(250*time.Millisecond),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	// Download all files concurrently with a wait group
	var wg sync.WaitGroup
	errChan := make(chan error, len(t.Files()))

	for _, file := range t.Files() {
		wg.Add(1)
		go func(f *torrent.File) {
			defer wg.Done()

			outputPath := filepath.Join(m.config.OutputDir, f.Path())
			if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
				errChan <- fmt.Errorf("failed to create directory: %w", err)
				return
			}

			out, err := os.Create(outputPath)
			if err != nil {
				errChan <- fmt.Errorf("failed to create file: %w", err)
				return
			}
			defer out.Close()

			reader := f.NewReader()
			defer reader.Close()

			_, err = io.CopyN(io.MultiWriter(out, bar), reader, f.Length())
			if err != nil && err != io.EOF {
				errChan <- fmt.Errorf("download failed for %s: %w", f.Path(), err)
				return
			}
		}(file)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			bar.Close()
			return err
		}
	}

	bar.Finish()
	fmt.Printf("\n[done] Download complete: %s\n", info.Name)
	return nil
}

func main() {
	app := &cli.App{
		Name:      "fastdown",
		Usage:     "Fast torrent downloader with speed control",
		ArgsUsage: "[magnet URI or torrent file path]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to config file",
				Value:   "configs/config.yaml",
			},
			&cli.IntFlag{
				Name:    "connections",
				Aliases: []string{"C"},
				Usage:   "Max concurrent connections",
				Value:   100,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output directory",
			},
		},
		Action: func(c *cli.Context) error {
			// Get the resource argument (magnet URI or torrent file)
			resource := c.Args().Get(0)
			if resource == "" {
				cli.ShowAppHelp(c)
				return fmt.Errorf("magnet URI or torrent file required")
			}

			configPath := c.String("config")
			outputDir := c.String("output")
			maxConnections := c.Int("connections")

			// Load config with defaults
			cfg := DefaultConfig()
			if data, err := os.ReadFile(configPath); err == nil {
				yaml.Unmarshal(data, cfg)
			}

			// Expand tilde paths
			if err := cfg.ExpandPath(); err != nil {
				return fmt.Errorf("failed to expand config paths: %w", err)
			}

			// Override with flags
			if outputDir != "" {
				cfg.OutputDir = outputDir
			}
			if cfg.OutputDir == "" {
				cfg.OutputDir = "."
			}
			if maxConnections > 0 {
				cfg.Connections = maxConnections
			}

			// Create torrent client
			client, err := torrent.NewClient(nil)
			if err != nil {
				return fmt.Errorf("failed to create torrent client: %w", err)
			}
			defer client.Close()

			// Download torrent
			dm := NewManager(client, cfg)
			return dm.Download(context.Background(), resource)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
