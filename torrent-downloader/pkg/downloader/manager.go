package downloader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/schollz/progressbar/v3"
	"torrent-downloader/internal/config"
	"torrent-downloader/internal/netlimit"
)

// Manager handles torrent downloads
type Manager struct {
	client    *torrent.Client
	config    *config.Config
	limiter   *netlimit.Limiter
	workers   []*Worker
}

// NewManager creates a new download manager
func NewManager(client *torrent.Client, cfg *config.Config) *Manager {
	limiter := netlimit.NewLimiter(cfg.MaxSpeed)
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
	<-t.GotInfo()


	// Set up progress bar
	info := t.Info()
	if info == nil {
		return fmt.Errorf("failed to get torrent info")
	}

	bar := progressbar.NewOptions64(
		info.TotalLength(),
		progressbar.OptionSetDescription(info.Name),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(20),
		progressbar.OptionThrottle(100*time.Millisecond),
	)

	// Download files
	for _, file := range t.Files() {
		outputPath := filepath.Join(m.config.OutputDir, file.Path())
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		// Read and write with progress
		reader := file.NewReader()
		_, err = io.CopyN(io.MultiWriter(f, bar), reader, file.Length())
		f.Close()
		reader.Close()

		if err != nil && err != io.EOF {
			return fmt.Errorf("download failed: %w", err)
		}
	}

	bar.Finish()
	fmt.Printf("Download complete: %s\n", info.Name)
	return nil
}
