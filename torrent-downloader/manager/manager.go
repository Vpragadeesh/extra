package manager

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"torrent-downloader/config"
	"torrent-downloader/network"

	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"
)

// PeerNetworkInfo tracks which network a peer is connected through.
type PeerNetworkInfo struct {
	PeerAddr  string    // "ip:port"
	NetworkID string    // network interface ID
	Connected time.Time // when connection was established
}

// TorrentHandle wraps a torrent with its lifecycle controls.
type TorrentHandle struct {
	ID             int
	Torrent        *torrent.Torrent
	Cancel         context.CancelFunc
	Paused         bool
	PeerNetworks   map[string]string // peerAddr -> networkID
	peerNetworksMu sync.RWMutex
}

// Manager handles torrent downloads
type Manager struct {
	client          *torrent.Client
	config          *config.Config
	networkMonitor  *network.Monitor
	networkLimits   *network.Limits
	networkDialer   *network.Dialer
	rateLimiter     *rate.Limiter // deprecated: kept for GetSpeedLimit compat
	storage         storage.ClientImplCloser
	pieceCompletion storage.PieceCompletion
	Events          chan ManagerEvent

	mu       sync.Mutex
	torrents map[int]*TorrentHandle
	nextID   int
}

func buildClientConfig(cfg *config.Config, st storage.ClientImplCloser) *torrent.ClientConfig {
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = cfg.OutputDir
	clientConfig.DefaultStorage = st
	// ListenPort 0 = OS picks a free port; avoids "address already in use" when
	// running multiple instances or after an unclean shutdown.
	clientConfig.ListenPort = cfg.ListenPort

	// Connection limits — 100 is plenty for any real swarm; 300 just adds churn.
	clientConfig.EstablishedConnsPerTorrent = cfg.Connections
	clientConfig.HalfOpenConnsPerTorrent = max(cfg.Connections/2, 25)
	clientConfig.TotalHalfOpenConns = max(cfg.Connections, 50)
	clientConfig.TorrentPeersHighWater = max(cfg.Connections*4, 500)
	clientConfig.TorrentPeersLowWater = max(cfg.Connections, 50)

	// Dial rate: 600/sec was a firehose that filled sockets with dead peers.
	// 20/sec sustained with a small burst lets good connections mature.
	clientConfig.DialRateLimiter = rate.NewLimiter(rate.Limit(20), 10)

	// Upload rate limiting. Unlimited upload chokes the download on shared links
	// because BitTorrent TCP connections are full-duplex and share the same
	// congestion window and bottleneck. Default: auto-capped at ~15% of the
	// download limit (or 1 MB/s if download is unlimited).
	ul := cfg.MaxUploadSpeed
	if ul <= 0 {
		if cfg.MaxSpeed > 0 {
			ul = cfg.MaxSpeed * 15 / 100 // ~15% of download
		} else {
			ul = 1 << 20 // 1 MiB/s default cap
		}
	}
	clientConfig.UploadRateLimiter = rate.NewLimiter(rate.Limit(ul), max(int(ul), 64<<10))
	// Disable "aggressive upload" — the library's mode that keeps uploading to
	// choked peers hoping they'll unchoke. It burns bandwidth with no reciprocal
	// download on asymmetric links.
	clientConfig.DisableAggressiveUpload = true

	// Buffer size for verified-but-not-yet-written piece data. This is a
	// THROTTLE, not a cache: while this budget is exhausted the download
	// pipeline stops pulling from peers until the hasher catches up. The library
	// default is 64 MiB; we were previously setting 16 MiB, which coupled
	// throughput to hashing speed and was a pure pessimization. Keep it well
	// above the library default so a burst of data never stalls the wire.
	clientConfig.MaxUnverifiedBytes = 256 << 20 // 256 MiB

	// Per-connection in-flight data budget. Larger values let each peer keep
	// more requests outstanding, which matters for high-latency or
	// high-bandwidth links; the default is 1 MiB.
	clientConfig.MaxAllocPeerRequestDataPerConn = 4 << 20 // 4 MiB

	// Hashers per torrent. Scales with the buffer above: hashing is what
	// releases buffered data, so it should not become the bottleneck.
	clientConfig.PieceHashersPerTorrent = 8
	return clientConfig
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ewmaAvgRate is the smoothing factor for the verified-progress rate used by
// the ETA. 0.3 means 30% of the newest sample per tick, which tracks genuine
// changes within a few seconds while suppressing the per-tick jitter caused by
// piece-completion gating.
const ewmaAvgRate = 0.3

// unlimitedBurst is the token-bucket burst used when no speed limit is set.
//
// It MUST be greater than zero. The torrent client gates ALL new peer
// connections on DownloadRateLimiter.Tokens() > 0 (see newConnsAllowed in
// anacrolix/torrent). A limiter built with burst 0 can never accumulate
// tokens, so with speed "unlimited" the client stops dialing peers entirely:
// DHT/tracker keep queueing addresses (PendingPeers grows) but no connection
// is ever made, so no peers -> no metadata -> the torrent hangs forever on
// "Downloading metadata...".
//
// The data path is unaffected: rate.Inf reservations always succeed, so the
// burst size is irrelevant to throughput when the limit is infinite.
const unlimitedBurst = 1 << 20 // 1 MiB

// newDownloadLimiter builds the download rate limiter. A limit of zero (or
// less) means unlimited.
func newDownloadLimiter(bytesPerSec int64) *rate.Limiter {
	if bytesPerSec <= 0 {
		return rate.NewLimiter(rate.Inf, unlimitedBurst)
	}
	return rate.NewLimiter(rate.Limit(bytesPerSec), max(int(bytesPerSec), 64<<10))
}

// NewManager creates a new download manager.
// Files are written directly to their final names (no .part temp files) so
// there is never a duplicate copy on disk. A BoltDB is used only to track
// piece-completion for fast-resume across restarts.
func NewManager(cfg *config.Config) (*Manager, error) {
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Initialize network components
	usagePath := filepath.Join(cfg.OutputDir, ".fastdown_network_usage.json")
	networkLimits := network.NewLimits(func(networkID string) {
		// Network hit its data cap - could emit event or pause downloads on this network
	}, usagePath)
	networkLimits.Configure(cfg.GlobalSpeedLimit, cfg.NetworkPreferences)

	networkMonitor := network.NewMonitor(func(interfaces []network.NetworkInterfaceInfo) {
		// Network changes could trigger reconfiguration; for now just log
	})
	// OS-level interface counters feed data-cap usage (all traffic incl. inbound).
	networkMonitor.SetTrafficCallback(func(deltas map[string]network.TrafficDelta) {
		for id, d := range deltas {
			networkLimits.AddTraffic(id, d.Down, d.Up)
		}
	})
	networkMonitor.Start()

	// Create dialer for interface-bound connections
	networkDialer := network.NewDialer(networkMonitor, cfg.NetworkPreferences, networkLimits)

	// Bolt DB for piece-completion fast-resume (NOT for storing piece bytes).
	resumeDir := filepath.Join(cfg.OutputDir, ".fastdown_resume")
	pc, err := storage.NewBoltPieceCompletion(resumeDir)
	if err != nil {
		networkMonitor.Stop()
		_ = networkLimits.Close()
		return nil, fmt.Errorf("failed to open resume DB: %w", err)
	}

	// File storage that writes directly to the final filename.
	// UsePartFiles=false means no ".part" temp files → no duplicate copies.
	st := storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir:   cfg.OutputDir,
		PieceCompletion: pc,
		UsePartFiles:    g.Some(false),
	})

	clientConfig := buildClientConfig(cfg, st)

	dlLimiter := newDownloadLimiter(cfg.MaxSpeed)
	clientConfig.DownloadRateLimiter = dlLimiter

	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		if clientConfig.ListenPort != 0 && strings.Contains(err.Error(), "address already in use") {
			clientConfig.ListenPort = 0
			client, err = torrent.NewClient(clientConfig)
		}
		if err != nil {
			_ = st.Close()
			_ = pc.Close()
			networkMonitor.Stop()
			_ = networkLimits.Close()
			return nil, fmt.Errorf("failed to create torrent client: %w", err)
		}
	}

	// Add custom dialer for multi-network support (binds sockets to interfaces)
	if networkDialer != nil {
		client.AddDialer(torrent.NetworkDialer{
			Network: "tcp",
			Dialer:  &networkDialerAdapter{dialer: networkDialer},
		})
	}

	return &Manager{
		client:          client,
		config:          cfg,
		networkMonitor:  networkMonitor,
		networkLimits:   networkLimits,
		networkDialer:   networkDialer,
		rateLimiter:     dlLimiter,
		storage:         st,
		pieceCompletion: pc,
		Events:          make(chan ManagerEvent, 50),
		torrents:        make(map[int]*TorrentHandle),
	}, nil
}

// Close shuts down the torrent client and flushes the resume database.
func (m *Manager) Close() {
	m.mu.Lock()
	for _, th := range m.torrents {
		if th.Cancel != nil {
			th.Cancel()
		}
	}
	m.mu.Unlock()

	if m.client != nil {
		m.client.Close()
	}
	if m.storage != nil {
		_ = m.storage.Close()
	}
	if m.pieceCompletion != nil {
		_ = m.pieceCompletion.Close()
	}
	if m.networkMonitor != nil {
		m.networkMonitor.Stop()
	}
	if m.networkLimits != nil {
		_ = m.networkLimits.Close()
	}
}

// SetSpeedLimit dynamically adjusts the download speed limit.
func (m *Manager) SetSpeedLimit(limitBytesPerSec int64) {
	m.config.MaxSpeed = limitBytesPerSec

	// Update network limits (global limit)
	if m.networkLimits != nil {
		m.networkLimits.Configure(limitBytesPerSec, m.config.NetworkPreferences)
	}

	// Keep old rateLimiter for GetSpeedLimit compat
	if m.rateLimiter != nil {
		if limitBytesPerSec > 0 {
			m.rateLimiter.SetLimit(rate.Limit(limitBytesPerSec))
			m.rateLimiter.SetBurst(max(int(limitBytesPerSec), 64<<10))
		} else {
			m.rateLimiter.SetLimit(rate.Inf)
			// Non-zero burst is required, else the client stops dialing peers
			// entirely (see newDownloadLimiter).
			m.rateLimiter.SetBurst(unlimitedBurst)
		}
	}
}

// GetSpeedLimit returns the current speed limit in bytes/sec.
func (m *Manager) GetSpeedLimit() int64 {
	return m.config.MaxSpeed
}

// AddTorrent adds a magnet or .torrent file, starts downloading, and
// begins sending progress events on the Events channel.
func (m *Manager) AddTorrent(parentCtx context.Context, resource string) (int, error) {
	m.mu.Lock()
	id := m.nextID
	m.nextID++
	m.mu.Unlock()

	m.Events <- ManagerEvent{
		Type:      EventMetadataWaiting,
		TorrentID: id,
		Message:   "Connecting...",
	}

	var t *torrent.Torrent

	if len(resource) > 7 && resource[:7] == "magnet:" {
		spec, err := torrent.TorrentSpecFromMagnetUri(resource)
		if err != nil {
			m.Events <- ManagerEvent{Type: EventError, TorrentID: id, Err: fmt.Errorf("invalid magnet: %w", err)}
			return id, err
		}
		if m.config.ChunkSize > 0 {
			spec.ChunkSize = pp.Integer(m.config.ChunkSize)
		}
		var err2 error
		t, _, err2 = m.client.AddTorrentSpec(spec)
		if err2 != nil {
			m.Events <- ManagerEvent{Type: EventError, TorrentID: id, Err: fmt.Errorf("failed to add: %w", err2)}
			return id, err2
		}
	} else {
		data, err := os.ReadFile(resource)
		if err != nil {
			m.Events <- ManagerEvent{Type: EventError, TorrentID: id, Err: fmt.Errorf("read file: %w", err)}
			return id, err
		}
		mi, err := metainfo.Load(bytes.NewReader(data))
		if err != nil {
			m.Events <- ManagerEvent{Type: EventError, TorrentID: id, Err: fmt.Errorf("parse torrent: %w", err)}
			return id, err
		}
		torrentHandle, err := m.client.AddTorrent(mi)
		if err != nil {
			m.Events <- ManagerEvent{Type: EventError, TorrentID: id, Err: fmt.Errorf("failed to add: %w", err)}
			return id, err
		}
		t = torrentHandle
	}

	ctx, cancel := context.WithCancel(parentCtx)
	handle := &TorrentHandle{
		ID:           id,
		Torrent:      t,
		Cancel:       cancel,
		PeerNetworks: make(map[string]string),
	}

	m.mu.Lock()
	m.torrents[id] = handle
	m.mu.Unlock()

	go m.runProgressLoop(ctx, handle)
	return id, nil
}

// RegisterPeerNetwork records which network a peer is connected through.
func (th *TorrentHandle) RegisterPeerNetwork(peerAddr, networkID string) {
	th.peerNetworksMu.Lock()
	defer th.peerNetworksMu.Unlock()
	th.PeerNetworks[peerAddr] = networkID
}

// UnregisterPeerNetwork removes a peer's network mapping.
func (th *TorrentHandle) UnregisterPeerNetwork(peerAddr string) {
	th.peerNetworksMu.Lock()
	defer th.peerNetworksMu.Unlock()
	delete(th.PeerNetworks, peerAddr)
}

// GetPeerNetwork returns the network ID for a peer address.
func (th *TorrentHandle) GetPeerNetwork(peerAddr string) (string, bool) {
	th.peerNetworksMu.RLock()
	defer th.peerNetworksMu.RUnlock()
	netID, ok := th.PeerNetworks[peerAddr]
	return netID, ok
}

// GetAllPeerNetworks returns a copy of all peer->network mappings.
func (th *TorrentHandle) GetAllPeerNetworks() map[string]string {
	th.peerNetworksMu.RLock()
	defer th.peerNetworksMu.RUnlock()
	result := make(map[string]string, len(th.PeerNetworks))
	for k, v := range th.PeerNetworks {
		result[k] = v
	}
	return result
}

// ToggleNetwork enables or disables a network interface.
func (m *Manager) ToggleNetwork(networkID string) {
	if m.networkLimits == nil {
		return
	}
	m.networkLimits.ToggleNetwork(networkID)
}

// PauseTorrent pauses a specific download.
func (m *Manager) PauseTorrent(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if th, ok := m.torrents[id]; ok && !th.Paused {
		th.Torrent.DisallowDataDownload()
		th.Paused = true
	}
}

// ResumeTorrent resumes a paused download.
func (m *Manager) ResumeTorrent(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if th, ok := m.torrents[id]; ok && th.Paused {
		th.Torrent.AllowDataDownload()
		th.Torrent.DownloadAll()
		th.Paused = false
	}
}

// RemoveTorrent stops and removes a download.
func (m *Manager) RemoveTorrent(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if th, ok := m.torrents[id]; ok {
		if th.Cancel != nil {
			th.Cancel()
		}
		th.Torrent.Drop()
		delete(m.torrents, id)
	}
}

func (m *Manager) runProgressLoop(ctx context.Context, handle *TorrentHandle) {
	t := handle.Torrent
	id := handle.ID

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	lastTime := time.Now()
	var lastBytes int64 = 0
	var lastUpBytes int64 = 0
	var lastWireDown int64 = 0
	// ewma holds the smoothed verified-progress rate (bytes/sec) used for ETA.
	var ewma float64 = 0

	var infoReady bool
	var totalLength int64
	gotInfoChan := t.GotInfo()

	// Metadata can legitimately take a while on a swarmy torrent, but it should
	// never block forever. Without this deadline a torrent with no reachable
	// peers (dead swarm, all trackers down, blocked port) leaves the TUI stuck
	// on "Downloading metadata..." indefinitely.
	var metadataTimeout <-chan time.Time
	if timeout := m.config.Timeout; timeout > 0 {
		timer := time.NewTimer(time.Duration(timeout) * time.Second)
		defer timer.Stop()
		metadataTimeout = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-metadataTimeout:
			// Only reachable while metadata is still outstanding. Report the
			// failure but keep the loop alive: metadata can still arrive later
			// (slow swarm, tracker recovery) and the TUI will pick it up via
			// EventMetadataReceived, which restores the "downloading" state.
			metadataTimeout = nil // one-shot; never fire again
			st := t.Stats()
			m.Events <- ManagerEvent{
				Type:      EventError,
				TorrentID: id,
				Err: fmt.Errorf("timed out after %ds waiting for metadata (%d peers discovered, %d connected)",
					m.config.Timeout, st.TotalPeers, st.ActivePeers),
			}
		case <-gotInfoChan:
			infoReady = true
			gotInfoChan = nil     // disable this case
			metadataTimeout = nil // metadata arrived; cancel the deadline

			info := t.Info()
			if info == nil {
				m.Events <- ManagerEvent{Type: EventError, TorrentID: id, Err: fmt.Errorf("failed to get info")}
				return
			}
			totalLength = info.TotalLength()

			completed := t.BytesCompleted()
			m.Events <- ManagerEvent{
				Type:      EventMetadataReceived,
				TorrentID: id,
				Snapshot: ProgressSnapshot{
					Name:           info.Name,
					TotalBytes:     totalLength,
					CompletedBytes: completed,
					TotalPieces:    t.NumPieces(),
				},
			}

			t.DownloadAll()

			lastBytes = completed
			lastTime = time.Now()
			initSt := t.Stats()
			lastUpBytes = initSt.BytesWrittenData.Int64()

		case <-t.Complete().On():
			if infoReady {
				stats := t.Stats()
				info := t.Info()
				m.Events <- ManagerEvent{
					Type:      EventComplete,
					TorrentID: id,
					Snapshot: ProgressSnapshot{
						Name:              info.Name,
						TotalBytes:        totalLength,
						CompletedBytes:    totalLength,
						ActivePeers:       stats.ActivePeers,
						TotalPeers:        stats.TotalPeers,
						PendingPeers:      stats.PendingPeers,
						HalfOpenPeers:     stats.HalfOpenPeers,
						ConnectedSeeders:  stats.ConnectedSeeders,
						PiecesComplete:    stats.PiecesComplete,
						TotalPieces:       t.NumPieces(),
						PiecesDirtiedGood: stats.PiecesDirtiedGood.Int64(),
						PiecesDirtiedBad:  stats.PiecesDirtiedBad.Int64(),
						ChunksWasted:      stats.ChunksReadWasted.Int64(),
					},
				}
			}
			return

		case now := <-ticker.C:
			st := t.Stats()

			if !infoReady {
				// We are still waiting for metadata, but we can report peer connections!
				m.Events <- ManagerEvent{
					Type:      EventProgress,
					TorrentID: id,
					Snapshot: ProgressSnapshot{
						Name:             "Connecting...",
						ActivePeers:      st.ActivePeers,
						TotalPeers:       st.TotalPeers,
						PendingPeers:     st.PendingPeers,
						HalfOpenPeers:    st.HalfOpenPeers,
						ConnectedSeeders: st.ConnectedSeeders,
					},
				}
				continue
			}

			// Normal progress updates after metadata is received.
			//
			// Speed is measured from wire-byte counters, NOT from
			// t.BytesCompleted(). BytesCompleted only advances when a whole
			// piece passes verification, so on a large torrent (multi-MB pieces)
			// a BytesCompleted-derived speed reads 0 for long stretches and then
			// jumps, which looks like a stall and makes ETA swing wildly.
			// BytesReadUsefulData counts payload bytes actually received, which is
			// what the user perceives as speed.
			currentBytes := t.BytesCompleted()
			currentUpBytes := st.BytesWrittenData.Int64()
			currentWireDown := st.BytesReadUsefulData.Int64()
			elapsed := now.Sub(lastTime).Seconds()

			var dlSpeed, dlSpeedWire, ulSpeed float64
			if elapsed > 0 {
				dlSpeedWire = float64(currentWireDown-lastWireDown) / elapsed
				ulSpeed = float64(currentUpBytes-lastUpBytes) / elapsed
				// Verified-progress rate, smoothed. Raw per-tick deltas are lumpy
				// because they are gated by piece completion, so feed an EWMA to
				// keep the ETA stable.
				verifiedRate := float64(currentBytes-lastBytes) / elapsed
				if verifiedRate > 0 {
					dlSpeed = verifiedRate
					if ewma == 0 {
						ewma = verifiedRate
					} else {
						ewma = ewmaAvgRate*ewma + (1-ewmaAvgRate)*verifiedRate
					}
				}
			}
			// Report the wire rate when it is meaningful; it reacts immediately,
			// whereas the verified rate lags by up to one piece.
			if dlSpeedWire > 0 {
				dlSpeed = dlSpeedWire
			}

			eta := time.Duration(0)
			// Base ETA on verified progress, not wire bytes: wire bytes can include
			// data that will be re-downloaded after a bad piece.
			if ewma > 0 {
				bytesLeft := float64(totalLength - currentBytes)
				if bytesLeft < 0 {
					bytesLeft = 0
				}
				eta = time.Duration(bytesLeft/ewma) * time.Second
			}

			m.Events <- ManagerEvent{
				Type:      EventProgress,
				TorrentID: id,
				Snapshot: ProgressSnapshot{
					Name:              t.Info().Name,
					TotalBytes:        totalLength,
					CompletedBytes:    currentBytes,
					DownloadSpeed:     dlSpeed,
					UploadSpeed:       ulSpeed,
					ActivePeers:       st.ActivePeers,
					TotalPeers:        st.TotalPeers,
					PendingPeers:      st.PendingPeers,
					HalfOpenPeers:     st.HalfOpenPeers,
					ConnectedSeeders:  st.ConnectedSeeders,
					PiecesComplete:    st.PiecesComplete,
					TotalPieces:       t.NumPieces(),
					PiecesDirtiedGood: st.PiecesDirtiedGood.Int64(),
					PiecesDirtiedBad:  st.PiecesDirtiedBad.Int64(),
					ChunksWasted:      st.ChunksReadWasted.Int64(),
					ETA:               eta,
					Networks:          m.collectNetworkStats(t, handle),
				},
			}

			lastTime = now
			lastBytes = currentBytes
			lastUpBytes = currentUpBytes
			lastWireDown = currentWireDown
		}
	}
}

// collectNetworkStats gathers per-network statistics for the TUI.
// It uses the torrent's connected peers and the dialer's peer->network mapping
// to attribute peers to networks and estimate per-network speeds.
func (m *Manager) collectNetworkStats(t *torrent.Torrent, handle *TorrentHandle) []network.NetworkProgress {
	if m.networkMonitor == nil || m.networkLimits == nil || m.networkDialer == nil {
		return nil
	}

	// Get peer->network mapping from dialer (outbound connections we initiated)
	peerNetworks := m.networkDialer.GetPeerNetworks()

	// Get connected peers from torrent
	peers := t.Stats().ActivePeers
	if peers == 0 {
		return m.collectNetworkStatsBasic()
	}

	// Count peers per network
	peersPerNetwork := make(map[string]int)
	peerAddrs := handle.GetAllPeerNetworks()

	// Merge dialer's mapping with handle's mapping
	for peerAddr, netID := range peerNetworks {
		peerAddrs[peerAddr] = netID
	}

	for _, netID := range peerAddrs {
		peersPerNetwork[netID]++
	}

	var stats []network.NetworkProgress
	interfaces := m.networkMonitor.Current()
	usedByPeriod := m.networkLimits.UsedByPeriod()
	rates := m.networkMonitor.TrafficRates()

	for _, iface := range interfaces {
		pref := network.GetNetworkPreference(m.config, iface.ID)

		// Get current period's data usage
		dataUsed := usedByPeriod[iface.ID]

		peerCount := peersPerNetwork[iface.ID]
		rate := rates[iface.ID]

		stats = append(stats, network.NetworkProgress{
			InterfaceID:     iface.ID,
			Label:           iface.DisplayName,
			Kind:            string(iface.Kind),
			DownloadSpeed:   float64(rate.Down),
			UploadSpeed:     float64(rate.Up),
			ActivePeers:     peerCount,
			BytesDownloaded: 0,
			BytesUploaded:   0,
			Status:          "on",
			DataUsed:        dataUsed,
			DataLimit:       pref.DataLimit,
			DataLimitPeriod: pref.DataLimitPeriod,
		})
	}

	return stats
}

// collectNetworkStatsBasic returns basic network stats without peer attribution.
func (m *Manager) collectNetworkStatsBasic() []network.NetworkProgress {
	if m.networkMonitor == nil || m.networkLimits == nil {
		return nil
	}

	var stats []network.NetworkProgress
	interfaces := m.networkMonitor.Current()
	usedByPeriod := m.networkLimits.UsedByPeriod()
	rates := m.networkMonitor.TrafficRates()

	for _, iface := range interfaces {
		pref := network.GetNetworkPreference(m.config, iface.ID)
		dataUsed := usedByPeriod[iface.ID]
		rate := rates[iface.ID]

		stats = append(stats, network.NetworkProgress{
			InterfaceID:     iface.ID,
			Label:           iface.DisplayName,
			Kind:            string(iface.Kind),
			DownloadSpeed:   float64(rate.Down),
			UploadSpeed:     float64(rate.Up),
			ActivePeers:     0,
			BytesDownloaded: 0,
			BytesUploaded:   0,
			Status:          "on",
			DataUsed:        dataUsed,
			DataLimit:       pref.DataLimit,
			DataLimitPeriod: pref.DataLimitPeriod,
		})
	}

	return stats
}

// networkDialerAdapter adapts our network.Dialer to the dialer.WithContext interface
// expected by torrent.NetworkDialer.
type networkDialerAdapter struct {
	dialer *network.Dialer
}

func (a *networkDialerAdapter) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return a.dialer.DialContext(ctx, network, addr)
}
