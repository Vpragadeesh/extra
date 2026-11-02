package network

import (
	"encoding/json"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

// ListActiveInterfaces returns all active non-loopback interfaces with usable addresses.
// This is the common entry point; platform-specific files provide the raw device list.
func ListActiveInterfaces() ([]NetworkInterfaceInfo, error) {
	devices, err := listPlatformDevices()
	if err != nil {
		return nil, err
	}

	// Get all interface addresses from the OS
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	// Build a map of device name -> addresses
	addrMap := make(map[string][]NetworkAddress)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		var usable []NetworkAddress
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			// Skip loopback, link-local, and unspecified
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				continue
			}
			// Skip 169.254.x.x (IPv4 link-local) and fe80:: (IPv6 link-local)
			if ip4 := ip.To4(); ip4 != nil {
				if ip4[0] == 169 && ip4[1] == 254 {
					continue
				}
			} else if strings.HasPrefix(ip.String(), "fe80:") {
				continue
			}

			family := 4
			if ip.To4() == nil {
				family = 6
			}

			var subnet string
			if family == 4 && ipNet.Mask != nil {
				if cidr := ipv4CIDR(ip, ipNet.Mask); cidr != "" {
					subnet = cidr
				}
			}

			usable = append(usable, NetworkAddress{
				Address: ip.String(),
				Family:  family,
				Netmask: net.IP(ipNet.Mask).String(),
				Subnet:  subnet,
			})
		}
		if len(usable) > 0 {
			addrMap[iface.Name] = usable
		}
	}

	// Merge platform device info with addresses
	var result []NetworkInterfaceInfo
	for _, dev := range devices {
		addrs := addrMap[dev.ID]
		if len(addrs) == 0 {
			// Interface has no usable addresses; skip
			continue
		}
		result = append(result, NetworkInterfaceInfo{
			ID:          dev.ID,
			Device:      dev.Device,
			DisplayName: dev.DisplayName,
			Addresses:   addrs,
			Kind:        dev.Kind,
			MAC:         dev.MAC,
		})
	}

	// Stable sort by ID for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// PlatformDevice is the raw device info from platform-specific discovery.
type PlatformDevice struct {
	ID          string
	Device      string
	DisplayName string
	Kind        NetworkInterfaceKind
	MAC         string
}

// listPlatformDevices is implemented in platform-specific files (interfaces_linux.go, interfaces_darwin.go, interfaces_windows.go).
// This file contains shared logic only.

// ipv4CIDR computes the CIDR notation (e.g. "192.168.1.0/24") from IP and netmask.
func ipv4CIDR(ip net.IP, mask net.IPMask) string {
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	mask4 := net.IP(mask).To4()
	if mask4 == nil {
		return ""
	}
	// Convert to netip.Prefix for easy CIDR formatting
	prefix := netip.PrefixFrom(netip.AddrFrom4([4]byte{ip4[0], ip4[1], ip4[2], ip4[3]}), onesCount(mask4))
	return prefix.String()
}

// onesCount returns the number of leading 1 bits in an IPv4 netmask.
func onesCount(mask net.IP) int {
	var count int
	for _, b := range mask {
		for i := 7; i >= 0; i-- {
			if b&(1<<i) != 0 {
				count++
			} else {
				return count
			}
		}
	}
	return count
}

// Monitor polls for network interface changes and notifies via callback.
type Monitor struct {
	mu           sync.RWMutex
	current      []NetworkInterfaceInfo
	onChange     func([]NetworkInterfaceInfo)
	pollInterval time.Duration
	stopCh       chan struct{}
	stopped      bool

	// Traffic sampling: OS-level per-interface byte counters.
	countersFn     func() (map[string]TrafficCounters, error) // injectable for tests
	lastCounters   map[string]TrafficCounters
	lastCountersAt time.Time
	trafficRates   map[string]TrafficRate
	onTraffic      func(map[string]TrafficDelta)
}

// TrafficCounters holds cumulative per-interface RX/TX byte counters from the OS.
type TrafficCounters struct {
	Down int64
	Up   int64
}

// TrafficRate is the measured throughput per direction for the last interval, in bytes/sec.
type TrafficRate struct {
	Down int64
	Up   int64
}

// TrafficDelta is the number of bytes transferred over one sampling interval.
type TrafficDelta struct {
	Down int64
	Up   int64
}

// NewMonitor creates a new network monitor.
// onChange is called whenever the interface list changes (add/remove/address change).
func NewMonitor(onChange func([]NetworkInterfaceInfo)) *Monitor {
	return &Monitor{
		onChange:     onChange,
		pollInterval: time.Second,
		stopCh:       make(chan struct{}),
	}
}

// SetTrafficCallback registers a callback invoked each poll with the bytes
// transferred per interface since the previous poll (~1 second apart).
func (m *Monitor) SetTrafficCallback(cb func(map[string]TrafficDelta)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTraffic = cb
}

// TrafficRates returns the measured throughput per interface (bytes/sec).
func (m *Monitor) TrafficRates() map[string]TrafficRate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rates := make(map[string]TrafficRate, len(m.trafficRates))
	for id, r := range m.trafficRates {
		rates[id] = r
	}
	return rates
}

// Start begins polling for interface changes.
func (m *Monitor) Start() {
	go m.pollLoop()
}

// Stop halts the monitor.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	close(m.stopCh)
	m.mu.Unlock()
}

// Current returns the last known interface list (nil until first poll completes).
func (m *Monitor) Current() []NetworkInterfaceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Find returns the interface with the given ID, or nil if not found.
func (m *Monitor) Find(id string) *NetworkInterfaceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.current {
		if m.current[i].ID == id {
			return &m.current[i]
		}
	}
	return nil
}

// Refresh forces an immediate poll.
func (m *Monitor) Refresh() error {
	ifaces, err := ListActiveInterfaces()
	if err != nil {
		return err
	}
	m.update(ifaces)
	return nil
}

func (m *Monitor) pollLoop() {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	// Initial poll
	if ifaces, err := ListActiveInterfaces(); err == nil {
		m.update(ifaces)
	}
	m.sampleTraffic()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			ifaces, err := ListActiveInterfaces()
			if err != nil {
				continue
			}
			m.update(ifaces)
			m.sampleTraffic()
		}
	}
}

// sampleTraffic reads the OS interface byte counters, computes per-interface
// throughput for the elapsed interval, and reports deltas via onTraffic.
// Deltas are gated to interfaces present in the current active list.
func (m *Monitor) sampleTraffic() {
	get := m.countersFn
	if get == nil {
		get = interfaceTrafficCounters
	}
	cur, err := get()
	if err != nil {
		return
	}

	now := time.Now()
	var deltas map[string]TrafficDelta

	m.mu.Lock()
	if !m.lastCountersAt.IsZero() {
		secs := now.Sub(m.lastCountersAt).Seconds()
		if secs > 0 {
			// Restrict deltas to currently active interfaces.
			active := make(map[string]bool, len(m.current))
			limited := make(map[string]TrafficCounters, len(m.current))
			for _, iface := range m.current {
				active[iface.ID] = true
				limited[iface.ID] = cur[iface.ID]
			}
			m.trafficRates, deltas = computeTrafficDeltas(m.lastCounters, limited, secs)
		}
	}
	m.lastCounters = cur
	m.lastCountersAt = now
	cb := m.onTraffic
	m.mu.Unlock()

	if cb != nil && len(deltas) > 0 {
		cb(deltas)
	}
}

// computeTrafficDeltas turns two consecutive counter snapshots into per-interface
// rates (bytes/sec) and absolute deltas. Interfaces missing from the previous
// snapshot report zero (first sample establishes the baseline). A counter that
// went backwards (interface restart/reset) reports zero for that interval.
func computeTrafficDeltas(last, cur map[string]TrafficCounters, secs float64) (map[string]TrafficRate, map[string]TrafficDelta) {
	rates := make(map[string]TrafficRate, len(cur))
	deltas := make(map[string]TrafficDelta, len(cur))
	for id, c := range cur {
		l, ok := last[id]
		if !ok || c.Down < l.Down || c.Up < l.Up {
			rates[id] = TrafficRate{}
			deltas[id] = TrafficDelta{}
			continue
		}
		dDown, dUp := c.Down-l.Down, c.Up-l.Up
		rates[id] = TrafficRate{
			Down: int64(float64(dDown) / secs),
			Up:   int64(float64(dUp) / secs),
		}
		deltas[id] = TrafficDelta{Down: dDown, Up: dUp}
	}
	return rates, deltas
}

func (m *Monitor) update(newList []NetworkInterfaceInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Compare JSON-serialized lists to detect any change
	oldJSON, _ := json.Marshal(m.current)
	newJSON, _ := json.Marshal(newList)

	if string(oldJSON) != string(newJSON) {
		m.current = newList
		if m.onChange != nil {
			go m.onChange(newList)
		}
	}
}

// InterfaceForAddress returns the interface that owns the given IP address,
// using the monitor's cached interface list.
func (m *Monitor) InterfaceForAddress(ip net.IP) *NetworkInterfaceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, iface := range m.current {
		for _, addr := range iface.Addresses {
			ipNet, ok := parseCIDR(addr.Subnet)
			if !ok {
				continue
			}
			if ipNet.Contains(ip) {
				return &iface
			}
		}
	}
	return nil
}

// parseCIDR parses a CIDR string (e.g., "192.168.1.0/24") into an IPNet.
func parseCIDR(cidr string) (*net.IPNet, bool) {
	if cidr == "" {
		return nil, false
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, false
	}
	return ipNet, true
}
