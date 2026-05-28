package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
	"golang.org/x/time/rate"
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
		Connections: 300,
		ChunkSize:   16 * 1024,
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
	client      *torrent.Client
	config      *Config
	limiter     *Limiter
	workers     []*Worker
	activeTorr  *torrent.Torrent
	downloadDir string
	torrentHash string
}

// NewManager creates a new download manager
func NewManager(client *torrent.Client, cfg *Config) *Manager {
	limiter := NewLimiter(cfg.MaxSpeed)
	return &Manager{
		client:      client,
		config:      cfg,
		limiter:     limiter,
		workers:     make([]*Worker, cfg.Connections),
		downloadDir: cfg.OutputDir,
	}
}

// Pause pauses the active download
func (m *Manager) Pause() error {
	if m.activeTorr == nil {
		return fmt.Errorf("no active download")
	}
	m.activeTorr.Drop()
	fmt.Println("[*] Download paused (torrent dropped from client)")
	return nil
}

// Resume resumes a paused download - requires re-adding
func (m *Manager) Resume() error {
	return fmt.Errorf("use --resume flag to reload saved torrent")
}

// Status returns current download status
func (m *Manager) Status() (string, int64, int64, error) {
	if m.activeTorr == nil {
		return "", 0, 0, fmt.Errorf("no active download")
	}
	info := m.activeTorr.Info()
	if info == nil {
		return "waiting for metadata", 0, 0, nil
	}
	completed := m.activeTorr.BytesCompleted()
	total := info.TotalLength()
	return info.Name, completed, total, nil
}

// SaveResumeData saves torrent info hash for resume
func SaveResumeData(hash string) error {
	return os.WriteFile(".fastdown_resume", []byte(hash), 0644)
}

// LoadResumeData loads saved torrent info hash
func LoadResumeData() (string, error) {
	data, err := os.ReadFile(".fastdown_resume")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ClearResumeData removes saved resume data
func ClearResumeData() error {
	os.Remove(".fastdown_resume")
	return nil
}

// SetActiveTorrent sets the active torrent for pause/resume
func (m *Manager) SetActiveTorrent(t *torrent.Torrent) {
	m.activeTorr = t
	if t.Info() != nil {
		m.torrentHash = hex.EncodeToString(m.activeTorr.InfoHash()[:])
	}
}

// StartDownload begins a new download
func (m *Manager) StartDownload(ctx context.Context, resource string, pauseImmediately bool) error {
	var t *torrent.Torrent

	if len(resource) > 7 && resource[:7] == "magnet:" {
		spec, err := torrent.TorrentSpecFromMagnetUri(resource)
		if err != nil {
			return fmt.Errorf("invalid magnet URI: %w", err)
		}
		if m.config.ChunkSize > 0 {
			spec.ChunkSize = pp.Integer(m.config.ChunkSize)
		}
		var err2 error
		t, _, err2 = m.client.AddTorrentSpec(spec)
		if err2 != nil {
			return fmt.Errorf("failed to add torrent: %w", err2)
		}
	} else {
		data, err := os.ReadFile(resource)
		if err != nil {
			return fmt.Errorf("failed to read torrent file: %w", err)
		}
		mi, err := metainfo.Load(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("failed to parse torrent file: %w", err)
		}
		torrentHandle, err := m.client.AddTorrent(mi)
		if err != nil {
			return fmt.Errorf("failed to add torrent: %w", err)
		}
		t = torrentHandle
	}

	if err := os.MkdirAll(m.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fmt.Println("[*] Waiting for torrent metadata...")
	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		fmt.Println("[*] Still waiting for metadata (this can take a while with magnet links)...")
		<-t.GotInfo()
	}

	info := t.Info()
	if info == nil {
		return fmt.Errorf("failed to get torrent info")
	}

	m.SetActiveTorrent(t)

	fmt.Printf("[+] Starting download: %s\n", info.Name)
	fmt.Printf("[+] Total size: %.2f MB\n", float64(info.TotalLength())/1024/1024)

	if pauseImmediately {
		t.Drop()
		m.torrentHash = hex.EncodeToString(t.InfoHash()[:])
		SaveResumeData(m.torrentHash)
		return nil
	}

	return m.MonitorDownload(t, info)
}

// MonitorDownload watches and updates progress
func (m *Manager) MonitorDownload(t *torrent.Torrent, info *metainfo.Info) error {
	if info == nil {
		info = t.Info()
		if info == nil {
			return fmt.Errorf("no torrent info")
		}
	}

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

	t.DownloadAll()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.Complete().On():
			_ = bar.Set64(info.TotalLength())
			bar.Finish()
			ClearResumeData()
			fmt.Printf("\n[done] Download complete: %s\n", info.Name)
			return nil
		case <-sigChan:
			t.Drop()
			m.torrentHash = hex.EncodeToString(t.InfoHash()[:])
			SaveResumeData(m.torrentHash)
			bar.Describe("[yellow][paused][reset]")
			fmt.Println("\n[*] Download paused. Run with --resume to continue.")
			select {}
		case <-ticker.C:
			stats := t.Stats()
			bar.Describe(fmt.Sprintf("[cyan][downloading][reset] peers:%d/%d", stats.ActivePeers, stats.TotalPeers))
			_ = bar.Set64(t.BytesCompleted())
		}
	}
}

func NewTorrentClientConfig(cfg *Config) *torrent.ClientConfig {
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = cfg.OutputDir
	clientConfig.EstablishedConnsPerTorrent = cfg.Connections
	clientConfig.HalfOpenConnsPerTorrent = max(cfg.Connections/2, 50)
	clientConfig.TotalHalfOpenConns = max(cfg.Connections, 100)
	clientConfig.TorrentPeersHighWater = max(cfg.Connections*8, 1000)
	clientConfig.TorrentPeersLowWater = max(cfg.Connections, 100)
	clientConfig.MaxUnverifiedBytes = 512 << 20
	clientConfig.PieceHashersPerTorrent = 4
	clientConfig.DialRateLimiter = rate.NewLimiter(rate.Limit(max(cfg.Connections*2, 200)), max(cfg.Connections*2, 200))
	if cfg.MaxSpeed > 0 {
		clientConfig.DownloadRateLimiter = rate.NewLimiter(rate.Limit(cfg.MaxSpeed), max(int(cfg.MaxSpeed), 64<<10))
	}
	return clientConfig
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
				Value:   DefaultConfig().Connections,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output directory",
			},
			&cli.BoolFlag{
				Name:  "pause",
				Usage: "Pause download immediately after starting",
			},
			&cli.BoolFlag{
				Name:  "resume",
				Usage: "Resume from saved state",
			},
		},
		Action: func(c *cli.Context) error {
			resume := c.Bool("resume")
			pause := c.Bool("pause")

			// Load config
			configPath := c.String("config")
			outputDir := c.String("output")
			maxConnections := c.Int("connections")

			cfg := DefaultConfig()
			if data, err := os.ReadFile(configPath); err == nil {
				yaml.Unmarshal(data, cfg)
			}

			if err := cfg.ExpandPath(); err != nil {
				return fmt.Errorf("failed to expand config paths: %w", err)
			}

			if outputDir != "" {
				cfg.OutputDir = outputDir
			}
			if cfg.OutputDir == "" {
				cfg.OutputDir = "."
			}
			if c.IsSet("connections") && maxConnections > 0 {
				cfg.Connections = maxConnections
			}

			// Create torrent client
			client, err := torrent.NewClient(NewTorrentClientConfig(cfg))
			if err != nil {
				return fmt.Errorf("failed to create torrent client: %w", err)
			}
			defer client.Close()

			dm := NewManager(client, cfg)

			// Resume mode
			if resume {
				hash, err := LoadResumeData()
				if err != nil {
					return fmt.Errorf("no saved download to resume. run without --resume first")
				}
				var infoHash metainfo.Hash
				decoded, err := hex.DecodeString(hash)
				if err != nil || len(decoded) != 20 {
					return fmt.Errorf("invalid stored hash: %s", hash)
				}
				copy(infoHash[:], decoded)
				t, ok := client.Torrent(infoHash)
				if !ok {
					torrentFile := cfg.OutputDir + "/" + hash + ".torrent"
					if data, err := os.ReadFile(torrentFile); err == nil {
						mi, err := metainfo.Load(bytes.NewReader(data))
						if err != nil {
							return fmt.Errorf("failed to parse torrent file: %w", err)
						}
						var err2 error
						t, _, err2 = client.AddTorrent(mi)
						if err2 != nil {
							return fmt.Errorf("failed to add torrent: %w", err2)
						}
					} else {
						return fmt.Errorf("torrent file not found: %s", torrentFile)
					}
				}
				dm.SetActiveTorrent(t)
				t.DownloadAll()
				return dm.MonitorDownload(t, nil)
			}

			// Normal download mode
			resource := c.Args().Get(0)
			if resource == "" {
				cli.ShowAppHelp(c)
				return fmt.Errorf("magnet URI or torrent file required")
			}

			err = dm.StartDownload(context.Background(), resource, pause)
			if err != nil {
				return err
			}

			if pause {
				dm.activeTorr.Pause()
				ClearResumeData()
				SaveResumeData(dm.torrentHash)
				fmt.Println("[*] Download paused. Run with --resume to continue.")
				select {}
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
