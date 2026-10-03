# Fast Chunk Downloader

Fast Chunk Downloader is a Go-based torrent downloader built with `anacrolix/torrent`. The application uses `configs/config.yaml` for configuration defaults.

## Features

- Magnet link and `.torrent` file support
- YAML configuration with sensible defaults
- Live progress display with percentage, speed, and ETA
- Download rate limiting
- Simple CLI via `urfave/cli`

## Project Structure

```
torrent-downloader/
├── main.go              # Entry point — parses CLI flags, bootstraps app
├── config/
│   └── config.go        # Config struct, YAML loading, defaults, path expansion
├── manager/
│   ├── manager.go       # Download manager — orchestrates torrent client
│   └── limiter.go       # Bandwidth rate limiter
├── ui/
│   └── progress.go      # Progress bar, live stats display
├── configs/
│   └── config.yaml      # Runtime defaults
├── go.mod
├── go.sum
└── README.md
```

## Installation

1. Build the project:
   ```
   go build -o fastdown main.go
   ```

2. Run the downloader:
   ```
   ./fastdown <magnet-uri-or-torrent-file>
   ```

## Configuration

Edit `configs/config.yaml` to control the download rate limit, output directory, and other defaults. Missing fields fall back to the defaults defined in `config/config.go`.

## Usage

Download from a magnet link:
```
./fastdown "magnet:?xt=urn:btih:..."
```

Download from a local torrent file:
```
./fastdown ./archlinux-2026.01.01-x86_64.iso.torrent
```

Specify custom output directory and speed limit (in KB/s):
```
./fastdown --output ~/Downloads --speed 5000 ./archlinux-2026.01.01-x86_64.iso.torrent
```

## Contributing

Contributions are welcome. Please open an issue or submit a pull request for fixes or improvements.

## License

This project is licensed under the MIT License. See the LICENSE file for details.