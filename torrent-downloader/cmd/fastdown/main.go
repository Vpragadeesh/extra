package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/anacrolix/torrent"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
	"torrent-downloader/pkg/downloader"
	"torrent-downloader/internal/config"
)

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
			cfg := config.Default()
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
			dm := downloader.NewManager(client, cfg)
			return dm.Download(context.Background(), resource)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
