# File Transfer

Lightweight file-transfer web server written in Go using Fiber.

This small app serves a single-page web UI (from `static/index.html`), accepts file uploads, and exposes uploaded files and a small JSON API for integration.

## Quick facts

- Upload directory (default): `/home/pragadeesh/Videos/`
- Static files directory: `static/` (served at `/`)
- Files served at: `/files/<filename>`
- Upload endpoint: `POST /upload` (form field name: `file`)
- Files list API: `GET /api/files` (returns JSON array of filenames)
- Network info API: `GET /api/network-info` (returns JSON with client/server IP and port)
- Server port: `1234`

When the server starts it prints the local and LAN access URLs and generates a QR code (if a non-loopback IP is found).

## Prerequisites

- Go (1.24)
- Fiber (latest)

## Build and run

From the repository root:

```sh
# download deps (optional, `go run` will do this too)
go mod tidy
```
```
go get github.com/gofiber/fiber/v2@latest
```
# build
```
go build -o file-transfer main.go
```
# run
```
./file-transfer
```
# or run directly
```
go run main.go
```

After starting, open:

- http://localhost:1234/ (or the LAN URL printed in your terminal)

The app will also print a QR code linking to the LAN URL when one is available.

## Uploading files

You can upload from the web UI (if `static/index.html` provides one) or with curl:

```sh
curl -F "file=@/path/to/local.file" http://localhost:1234/upload
```

Uploaded files are saved into the configured upload directory and are publicly accessible at `/files/<filename>`.

## API examples

- List files:

```sh
curl http://localhost:1234/api/files
```

- Network info:

```sh
curl http://localhost:1234/api/network-info
```

## Configuration

The server configuration is set as constants in `main.go`:

- `uploaddir` — path to save uploaded files (default: `/home/pragadeesh/Videos/`)
- `staticdir` — path to the SPA static files (default: `./static` in the repo)
- `serverport` — port to listen on (default: `1234`)

To change these, edit the constants at the top of `main.go` and rebuild. Optionally, you can modify the code to read these values from environment variables.

## Permissions and security

- The app will attempt to create the upload directory if it doesn't exist. Make sure the user running the binary has permission to write to the configured `uploaddir`.
- The server listens on all interfaces (by listening on `:` + port). Be careful when running on a machine attached to an untrusted network — consider firewall rules or binding to `localhost` for local-only usage.

## Notes

- This repository includes a simple SPA in `static/` used as the web UI. You can replace it with your own frontend if desired.
- The implementation intentionally keeps things simple and minimal. If you need authentication, HTTPS, or size limits, add those before exposing the service publicly.

---

If you'd like, I can update `main.go` to accept configuration via environment variables and add a small Makefile or systemd unit file for easier running.
