package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"torrent-downloader/config"
	"torrent-downloader/manager"
	"torrent-downloader/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v2"
)

// configRedirect points fd 2 at the log file (see config.LogFile) and returns
// a writable file for the REAL terminal that was fd 2 at process start — used
// for startup errors that must be visible before/without the TUI.
func configRedirect(logPath string) *os.File {
	logPath = strings.TrimSpace(logPath)
	if logPath == "" {
		logPath = "fastdown.log"
	}
	saved := configOpenLogDup(logPath)
	if saved > 0 {
		// os.NewFile on the saved descriptor number so writes target the
		// original physical stderr even after the dup3 replacement.
		f := os.NewFile(uintptr(saved), "/dev/stderr-real")
		if f != nil {
			return f
		}
	}
	return os.Stderr
}

func main() {
	app := &cli.App{
		Name:      "fastdown",
		Usage:     "Ultra-fast torrent downloader (TUI)",
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
				Value:   300,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output directory",
			},
			&cli.Int64Flag{
				Name:    "speed",
				Aliases: []string{"s"},
				Usage:   "Max download speed in KB/s (0 = unlimited)",
				Value:   0,
			},
			&cli.Int64Flag{
				Name:    "upload-speed",
				Aliases: []string{"u"},
				Usage:   "Max upload speed in KB/s (0 = auto ~15% of download, or 1 MB/s if unlimited)",
				Value:   0,
			},
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Listen port for BitTorrent connections (0 = OS-assigned, avoids conflicts)",
				Value:   0,
			},
			&cli.StringFlag{
				Name:    "log",
				Aliases: []string{"L"},
				Usage:   "File for internal torrent-library logs (default from config; 'none' discards)",
			},
		},
		Action: func(c *cli.Context) error {
			// Optional: pre-load a magnet/torrent if provided on CLI
			resource := c.Args().Get(0)

			configPath := c.String("config")
			outputDir := c.String("output")
			maxConnections := c.Int("connections")
			maxSpeedKBps := c.Int64("speed")
			listenPort := c.Int("port")

			// Load config with defaults
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
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
			if c.IsSet("connections") && maxConnections > 0 {
				cfg.Connections = maxConnections
			}
			if c.IsSet("speed") {
				cfg.MaxSpeed = maxSpeedKBps * 1024
			}
			if c.IsSet("upload-speed") {
				cfg.MaxUploadSpeed = c.Int64("upload-speed") * 1024
			}
			if c.IsSet("port") {
				cfg.ListenPort = listenPort
			}

			// Redirect fd 2 (stderr) to the log file BEFORE creating the torrent
			// client. The library logs every blocked/failed tracker announce
			// straight to stderr; with the TUI on the alt screen those writes
			// paint over (mix with) the interface. Everything now lands in the
			// log file instead — including Go panics, which are the most useful
			// thing to capture. Errors printed via `realErr` still reach the
			// physical terminal.
			logPath := cfg.LogFile
			if c.IsSet("log") {
				logPath = c.String("log")
			}
			realErr := configRedirect(logPath)

			// Initialize manager — print errors directly so they are visible
			// BEFORE the alternate screen (TUI) takes over.
			dm, err := manager.NewManager(cfg)
			if err != nil {
				fmt.Fprintf(realErr, "\n\033[31mERROR:\033[0m %v\n", err)
				if cfg.ListenPort != 0 {
					fmt.Fprintf(realErr, "Tip: try a different --port, or use --port 0 to let the OS pick one.\n")
				} else {
					fmt.Fprintf(realErr, "Tip: another fastdown instance may still be running.\n")
				}
				return err
			}
			defer dm.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Launch the TUI
			model := tui.NewModel(dm, cfg, ctx, cancel, resource)
			p := tea.NewProgram(model, tea.WithAltScreen())

			_, err = p.Run()
			return err
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
