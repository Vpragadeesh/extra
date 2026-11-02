# FastDown - Torrent Downloader

A lightweight, ultra-fast, Terminal User Interface (TUI) based torrent downloader built in Go. It uses `anacrolix/torrent` as the core download engine and `charmbracelet/bubbletea` for a rich, responsive console interface.

## Features

- **TUI Dashboard:** Live progress bar, speed metrics, peer tracking, and pieces health.
- **Ultra-Fast NVMe-Optimized:** Tuned buffer settings (`MaxUnverifiedBytes`) to flush downloaded data directly to disk instead of accumulating in RAM.
- **Instant Resume:** BoltDB-backed persistence records verified pieces, eliminating the need to re-hash the entire file on restart.
- **Multi-Threaded & Connection Heavy:** Allows up to 500 connections (default 300) with 8 piece hashers for maximum throughput.
- **Bandwidth Control:** Change download speed limits dynamically with hotkeys.

## Interface

The TUI displays:
- Download progress (Size, GB and percentage)
- Real-time download/upload speed (↓/↑)
- Current speed limit configuration
- Active and total peers, plus connected seeders
- Piece verification stats (Good vs Bad/Wasted chunks)

### Keybindings
| Key | Action |
|-----|--------|
| `q` / `ctrl+c` | Quit gracefully (saves resume state) |
| `p` | Pause / Resume |
| `+` / `=` | Increase speed limit by 1 MB/s |
| `-` | Decrease speed limit by 1 MB/s |
| `0` | Remove speed limit (unlimited) |

## Installation

You need Go 1.24+ installed.

```bash
git clone <repo>
cd torrent-downloader
go build -o fastdown ./main.go
```

## Usage

```bash
# Basic usage (magnet link)
./fastdown "magnet:?xt=urn:btih:..."

# Custom output directory
./fastdown --output ~/Downloads "magnet:?xt=urn:btih:..."

# Limit speed to 5000 KB/s
./fastdown --speed 5000 "magnet:?xt=urn:btih:..."

# Maximize connections
./fastdown --connections 1000 "magnet:?xt=urn:btih:..."
```

## Configuration

You can also use a YAML config file (default: `configs/config.yaml`):

```yaml
output_dir: "~/Videos"
connections: 300
timeout_seconds: 30
tui_refresh_ms: 250
theme: "dark"
```

Pass the config file with `-c` or `--config`:
```bash
./fastdown -c custom_config.yaml "magnet:..."
```