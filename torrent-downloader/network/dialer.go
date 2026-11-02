package network

import (
	"context"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"torrent-downloader/config"
)

// trackedConn wraps a net.Conn of an outbound-dialed connection to apply
// per-network download rate limiting. Speed and usage accounting happen at the
// OS-interface level (Monitor counters → Limits.AddTraffic), which also covers
// inbound connections that never pass through here.
type trackedConn struct {
	net.Conn
	networkID string
	limits    *Limits
}

func (c *trackedConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if n > 0 && c.limits != nil {
		c.limits.Take(c.networkID, int64(n))
	}
	return n, err
}

// Dialer implements torrent.Dialer to bind outgoing connections to specific network interfaces.
// It also handles twin peers: dialing the same peer on multiple interfaces.
type Dialer struct {
	monitor        *Monitor
	prefs          map[string]config.NetworkPreference
	limits         *Limits
	peerNetworks   map[string]map[string]bool // peerAddr -> networkID -> bool (dialled out)
	peerNetworksMu sync.Mutex
	twinRetryAt    map[string]time.Time // twinKey -> next retry time
	twinRetryMu    sync.Mutex
	twinCounter    atomic.Uint64 // For generating unique peer IDs per network
}

// TwinRetryInterval is how long to wait before retrying a twin peer on another network.
const TwinRetryInterval = 30 * time.Second

// NewDialer creates a new interface-aware dialer.
func NewDialer(monitor *Monitor, prefs map[string]config.NetworkPreference, limits *Limits) *Dialer {
	return &Dialer{
		monitor:      monitor,
		prefs:        prefs,
		limits:       limits,
		peerNetworks: make(map[string]map[string]bool),
		twinRetryAt:  make(map[string]time.Time),
	}
}

// Dial implements torrent.Dialer.
// It selects the best network for the peer address and binds the connection to it.
func (d *Dialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

// DialContext implements torrent.Dialer with context support.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	// Parse IP to determine family
	ip := net.ParseIP(host)
	if ip == nil {
		// Hostname - resolve it
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, err
		}
		ip = ips[0]
	}

	family := 4
	if ip.To4() == nil {
		family = 6
	}

	// Get usable networks for this address family
	usable := d.usableNetworks(family)
	if len(usable) == 0 {
		// No suitable network - return a connection that will fail quickly
		dialer := &net.Dialer{}
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		// Close immediately with error
		go func() {
			time.Sleep(10 * time.Millisecond)
			conn.Close()
		}()
		return conn, nil
	}

	// Pick network with fewest peers
	networkID := d.pickNetwork(usable)
	if networkID == "" {
		dialer := &net.Dialer{}
		return dialer.DialContext(ctx, network, address)
	}

	iface := d.monitor.Find(networkID)
	if iface == nil {
		dialer := &net.Dialer{}
		return dialer.DialContext(ctx, network, address)
	}

	// Track that we dialled out to this peer on this network
	peerKey := net.JoinHostPort(host, portStr)
	d.recordDialOut(peerKey, networkID)

	// Create a dialer that binds to the interface
	dialer := &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return d.bindToInterface(c, iface)
		},
	}

	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	// Wrap connection to track bytes per network
	tracked := &trackedConn{
		Conn:      conn,
		networkID: networkID,
		limits:    d.limits,
	}

	// Schedule twin peers for this peer on other networks
	go d.scheduleTwins(peerKey, networkID, host, portStr)

	return tracked, nil
}

// usableNetworks returns networks that are enabled and can reach the given IP family.
func (d *Dialer) usableNetworks(family int) []NetworkInterfaceInfo {
	d.monitor.mu.RLock()
	defer d.monitor.mu.RUnlock()

	var usable []NetworkInterfaceInfo
	for _, iface := range d.monitor.current {
		pref := d.prefs[iface.ID]
		if pref.Off {
			continue
		}
		// Check if interface has an address of the right family
		hasFamily := false
		for _, addr := range iface.Addresses {
			if addr.Family == family {
				hasFamily = true
				break
			}
		}
		if !hasFamily {
			continue
		}
		usable = append(usable, iface)
	}
	return usable
}

// pickNetwork selects the network with the fewest active peers.
// This matches plexo's pickNetwork logic.
func (d *Dialer) pickNetwork(networks []NetworkInterfaceInfo) string {
	if len(networks) == 0 {
		return ""
	}
	if len(networks) == 1 {
		return networks[0].ID
	}

	// Count peers per network
	loads := make([]NetworkLoad, 0, len(networks))
	for _, iface := range networks {
		count := d.countPeersOnNetwork(iface.ID)
		loads = append(loads, NetworkLoad{ID: iface.ID, Peers: count})
	}

	return PickNetwork(loads)
}

// countPeersOnNetwork returns the number of peers currently on a network.
func (d *Dialer) countPeersOnNetwork(networkID string) int {
	d.peerNetworksMu.Lock()
	defer d.peerNetworksMu.Unlock()

	count := 0
	for _, nets := range d.peerNetworks {
		if nets[networkID] {
			count++
		}
	}
	return count
}

// recordDialOut marks that we initiated a connection to peer on network.
func (d *Dialer) recordDialOut(peerAddr, networkID string) {
	d.peerNetworksMu.Lock()
	defer d.peerNetworksMu.Unlock()

	if d.peerNetworks[peerAddr] == nil {
		d.peerNetworks[peerAddr] = make(map[string]bool)
	}
	d.peerNetworks[peerAddr][networkID] = true
}

// removePeer removes a peer from tracking (when connection closes).
func (d *Dialer) removePeer(peerAddr, networkID string) {
	d.peerNetworksMu.Lock()
	defer d.peerNetworksMu.Unlock()

	if nets, ok := d.peerNetworks[peerAddr]; ok {
		delete(nets, networkID)
		if len(nets) == 0 {
			delete(d.peerNetworks, peerAddr)
		}
	}
}

// scheduleTwins dials the same peer on other available networks (twin peers).
// This is ported from plexo's dialTwins logic.
func (d *Dialer) scheduleTwins(peerAddr, originalNetwork, host, port string) {
	time.Sleep(TwinRetryInterval)

	d.twinRetryMu.Lock()
	twinKey := peerAddr + "#twin:" + originalNetwork
	if retryAt, ok := d.twinRetryAt[twinKey]; ok && time.Now().Before(retryAt) {
		d.twinRetryMu.Unlock()
		return
	}
	d.twinRetryAt[twinKey] = time.Now().Add(TwinRetryInterval)
	d.twinRetryMu.Unlock()

	// Get other usable networks
	usable := d.usableNetworks(4) // Assume IPv4 for twin peers; could check host
	if len(usable) < 2 {
		return
	}

	for _, iface := range usable {
		if iface.ID == originalNetwork {
			continue
		}
		// Check if we already have a twin on this network
		d.peerNetworksMu.Lock()
		hasTwin := d.peerNetworks[peerAddr][iface.ID]
		d.peerNetworksMu.Unlock()
		if hasTwin {
			continue
		}

		// Dial twin peer
		twinAddr := net.JoinHostPort(host, port)
		dialer := &net.Dialer{
			Control: func(network, address string, c syscall.RawConn) error {
				return d.bindToInterface(c, &iface)
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		conn, err := dialer.DialContext(ctx, "tcp", twinAddr)
		cancel()
		if err != nil {
			continue
		}
		// Track twin
		d.recordDialOut(peerAddr, iface.ID)
		_ = conn.Close() // Twin is managed by torrent client; we just initiate
	}
}

// bindToInterface binds a socket to a specific network interface.
// Uses SO_BINDTODEVICE on Linux, IP_BOUND_IF on macOS/BSD.
// Requires appropriate permissions (CAP_NET_RAW on Linux).
func (d *Dialer) bindToInterface(rawConn syscall.RawConn, iface *NetworkInterfaceInfo) error {
	var err error
	rawConn.Control(func(fd uintptr) {
		switch runtime.GOOS {
		case "linux":
			err = bindToDeviceLinux(fd, iface)
		case "darwin", "freebsd":
			err = bindToInterfaceBSD(fd, iface)
		case "windows":
			err = bindToInterfaceWindows(fd, iface)
		default:
			err = nil // Unsupported
		}
	})
	return err
}

// bindToDeviceLinux uses SO_BINDTODEVICE to bind socket to interface.
// Requires CAP_NET_RAW or root.
func bindToDeviceLinux(fd uintptr, iface *NetworkInterfaceInfo) error {
	// SO_BINDTODEVICE = 25
	const SO_BINDTODEVICE = 25
	name := iface.ID + "\x00" // Null-terminated
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, SO_BINDTODEVICE, name)
}

// bindToInterfaceBSD uses IP_BOUND_IF / IPV6_BOUND_IF to bind to interface index.
func bindToInterfaceBSD(fd uintptr, iface *NetworkInterfaceInfo) error {
	// Get interface index
	nativeIface, err := net.InterfaceByName(iface.ID)
	if err != nil {
		return err
	}

	// IP_BOUND_IF = 25 (IPPROTO_IP), IPV6_BOUND_IF = 62 (IPPROTO_IPV6)
	// We'll try both
	var err1, err2 error
	err1 = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, 25, nativeIface.Index)
	err2 = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, 62, nativeIface.Index)
	if err1 != nil && err2 != nil {
		return err1
	}
	return nil
}

// bindToInterfaceWindows uses SetAdapterId or similar (not directly supported in Go stdlib).
// For Windows, we rely on the routing table to pick the right interface based on source IP.
func bindToInterfaceWindows(fd uintptr, iface *NetworkInterfaceInfo) error {
	// Windows doesn't have a simple socket option for this.
	// The best we can do is bind to the interface's IP address as source.
	// This is handled by the routing table automatically.
	return nil
}

// TwinPeerKey generates a unique key for a twin peer (peerAddr + network).
func TwinPeerKey(peerAddr, networkID string) string {
	return peerAddr + "#twin:" + networkID
}

// GenerateTwinPeerID generates a unique peer ID for a twin on a specific network.
// Prefix is from the main client's peer ID; rest is random.
func (d *Dialer) GenerateTwinPeerID(prefix string, networkID string) string {
	counter := d.twinCounter.Add(1)
	// Use prefix (first 16 chars) + random suffix
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	suffixLen := (40 - len(prefix)) / 2
	suffix := make([]byte, suffixLen)
	for i := range suffix {
		suffix[i] = byte('0' + (counter % 10))
		counter /= 10
	}
	return prefix + string(suffix)
}

// NotifyPeerConnected informs the dialer that a peer connected on a network.
// This helps with load balancing.
func (d *Dialer) NotifyPeerConnected(peerAddr, networkID string) {
	d.peerNetworksMu.Lock()
	defer d.peerNetworksMu.Unlock()

	if d.peerNetworks[peerAddr] == nil {
		d.peerNetworks[peerAddr] = make(map[string]bool)
	}
	d.peerNetworks[peerAddr][networkID] = true
}

// HandleInboundConnection determines the network for an inbound connection
// based on the local address of the socket.
func (d *Dialer) HandleInboundConnection(localAddr, remoteAddr net.Addr) string {
	localIP := extractIP(localAddr)
	if localIP == nil {
		return ""
	}

	iface := d.monitor.InterfaceForAddress(localIP)
	if iface == nil {
		return ""
	}

	pref := d.prefs[iface.ID]
	if pref.Off {
		return ""
	}

	networkID := iface.ID
	d.NotifyPeerConnected(extractAddr(remoteAddr), networkID)
	return networkID
}

// extractIP extracts the IP from a net.Addr.
func extractIP(addr net.Addr) net.IP {
	switch a := addr.(type) {
	case *net.TCPAddr:
		return a.IP
	case *net.UDPAddr:
		return a.IP
	case *net.IPAddr:
		return a.IP
	}
	// Try parsing string representation
	host, _, err := net.SplitHostPort(addr.String())
	if err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(addr.String())
}

// extractAddr extracts the host:port from a net.Addr.
func extractAddr(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return net.JoinHostPort(host, port)
}

// NotifyPeerDisconnected informs the dialer that a peer disconnected.
func (d *Dialer) NotifyPeerDisconnected(peerAddr, networkID string) {
	d.removePeer(peerAddr, networkID)
}

// GetPeerNetworks returns a copy of the peer->network mapping.
// Key is peer address (ip:port), value is the network ID it was dialed on.
func (d *Dialer) GetPeerNetworks() map[string]string {
	d.peerNetworksMu.Lock()
	defer d.peerNetworksMu.Unlock()

	result := make(map[string]string, len(d.peerNetworks))
	for peerAddr, networks := range d.peerNetworks {
		// Return the first network (should only be one for outbound)
		for netID := range networks {
			result[peerAddr] = netID
			break
		}
	}
	return result
}
