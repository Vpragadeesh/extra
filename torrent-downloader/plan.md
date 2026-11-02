# fastdown — Fix Plan: "Not Downloading" Bug

## Root Cause

The torrent client fails to start with:

```
failed to create torrent client: first listen: listen tcp4 :42069: bind: address already in use
```

`anacrolix/torrent` defaults to **port 42069** for BitTorrent peer connections (TCP + uTP).
Our code never sets `ListenPort`, so it always tries 42069 — if anything (a previous
instance, another torrent app, or the OS) already holds that port the **entire client
creation fails**, and because the TUI swallows the error, the user just sees
"Connecting to peers…" forever.

---

## Issues Found

| # | Issue | Severity |
|---|-------|----------|
| 1 | Hardcoded default port 42069 conflicts with prior instances | 🔴 Critical |
| 2 | `NewManager` error is returned but TUI's `addTorrent` ignores it | 🔴 Critical |
| 3 | No visible error shown in TUI when client fails to create | 🟠 High |
| 4 | No configurable `listen_port` in `config.yaml` | 🟡 Medium |
| 5 | Duplicate files still present in `~/Videos/` from old BoltDB bug | 🟡 Medium |

---

## Fix Plan

### Fix 1 — Use a random/OS-assigned listen port (port 0)

**File:** `manager/manager.go` → `buildClientConfig()`

```go
// Before (implicit default = 42069):
clientConfig := torrent.NewDefaultClientConfig()

// After (port 0 = OS picks a free port):
clientConfig := torrent.NewDefaultClientConfig()
clientConfig.ListenPort = 0   // let the OS assign a free port
```

**Why port 0?** The OS guarantees a free port. BitTorrent doesn't require a fixed port
— peers announce whatever port they listen on via tracker/DHT.

---

### Fix 2 — Make listen port configurable

**File:** `config/config.go`, `configs/config.yaml`

Add `ListenPort int` field to `Config`:
```yaml
listen_port: 0   # 0 = OS-assigned random port (recommended)
                 # Set to e.g. 42069 if you need a fixed port for firewall rules
```

Pass it through in `buildClientConfig`.

---

### Fix 3 — Surface errors in the TUI

**File:** `tui/app.go` → `addTorrent()` cmd

The current code:
```go
func (m Model) addTorrent(resource string) tea.Cmd {
    return func() tea.Msg {
        mgr.AddTorrent(ctx, resource)  // error silently discarded!
        return addedMsg{}
    }
}
```

Fix: return an `errorMsg` if `AddTorrent` fails so the TUI can display it inline.

---

### Fix 4 — Handle manager startup failure gracefully

When `NewManager` itself fails, the TUI is never launched and the error prints behind
the alternate screen. Fix: if `NewManager` fails, skip launching the TUI and print
a clear, visible error message.

---

## Implementation Order

1. ✅ **Fix listen port** → `buildClientConfig()` sets `ListenPort = cfg.ListenPort` (default 0)
2. ✅ **Add `listen_port` to config struct and yaml** → also added `--port/-p` CLI flag
3. ✅ **Fix `addTorrent` error handling** → `errorMsg` type shown inline in TUI list
4. ✅ **Graceful startup error** → printed to stderr in color before TUI launches, with tips

---

## Testing Checklist

- [x] Run two `./fastdown` instances simultaneously — both start without error (port 0 = OS picks separate ports)
- [ ] Add a magnet link → progress bar should move within 30s
- [ ] Kill and restart `./fastdown` → fast-resume should pick up where it left off
- [ ] Check `~/Videos/` — only one copy of each file after completion
- [ ] Detail view (`Tab`) shows live peer count > 0 while downloading

