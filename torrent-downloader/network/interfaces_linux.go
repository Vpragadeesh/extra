//go:build linux

package network

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"os/exec"
)

// listPlatformDevices discovers network interfaces on Linux via sysfs and udevadm.
func listPlatformDevices() ([]PlatformDevice, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}

	var devices []PlatformDevice
	for _, entry := range entries {
		name := entry.Name()
		if name == "lo" {
			continue
		}
		// Filter out common virtual/container interfaces
		if isVirtualInterface(name) {
			continue
		}

		sysPath := "/sys/class/net/" + name

		// Check if it's a pure virtual device with no hardware backing
		if isPureVirtual(sysPath) {
			continue
		}

		kind := classifyLinuxDevice(name, sysPath)
		displayName := getLinuxDisplayName(name, sysPath, kind)
		mac := readMAC(sysPath)

		devices = append(devices, PlatformDevice{
			ID:          name,
			Device:      name,
			DisplayName: displayName,
			Kind:        kind,
			MAC:         mac,
		})
	}

	return devices, nil
}

func isVirtualInterface(name string) bool {
	virtualPrefixes := []string{
		"docker", "br-", "veth", "virbr", "vboxnet", "vmnet",
		"cni", "flannel", "cbr", "dummy", "geneve", "vxlan",
		"tun", "tap", "ip6tnl", "ip6gre", "sit", "gre", "gretap",
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isPureVirtual(sysPath string) bool {
	// Check if it's a virtual device with no hardware backing
	// and no wireless/phy capabilities
	linkPath := sysPath
	if link, err := os.Readlink(linkPath); err == nil {
		if strings.Contains(link, "/devices/virtual/") {
			// No device, no wireless, no phy80211 = pure virtual
			if _, err := os.Stat(sysPath + "/device"); err != nil {
				if _, err := os.Stat(sysPath + "/wireless"); err != nil {
					if _, err := os.Stat(sysPath + "/phy80211"); err != nil {
						return true
					}
				}
			}
		}
	}
	return false
}

func classifyLinuxDevice(name, sysPath string) NetworkInterfaceKind {
	// Wireless check
	if _, err := os.Stat(sysPath + "/wireless"); err == nil {
		return KindWiFi
	}
	if _, err := os.Stat(sysPath + "/phy80211"); err == nil {
		return KindWiFi
	}
	if strings.HasPrefix(name, "wl") {
		return KindWiFi
	}

	// USB tethering / RNDIS check
	if strings.HasPrefix(name, "usb") || strings.HasPrefix(name, "rndis") {
		return KindUSB
	}
	if strings.HasPrefix(name, "enx") { // MAC-based name often USB
		return KindUSB
	}
	if strings.Contains(name, "u") && strings.HasPrefix(name, "en") {
		// enp0s20f0u1 pattern: USB in PCI path
		if matched, _ := regexp.MatchString(`^en\w*u\d+`, name); matched {
			return KindUSB
		}
	}

	// Check udevadm for USB bus
	if isUSBViaUdev(sysPath) {
		return KindUSB
	}

	// Ethernet
	if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "en") {
		return KindEthernet
	}

	// Bridge
	if strings.HasPrefix(name, "br") || strings.HasPrefix(name, "virbr") {
		return KindBridge
	}

	return KindOther
}

func isUSBViaUdev(sysPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "udevadm", "info", "-q", "property", sysPath)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID_BUS=usb") {
			return true
		}
		if strings.HasPrefix(line, "DEVTYPE=wlan") {
			// This would be WiFi, not USB
		}
	}
	return false
}

func getLinuxDisplayName(name, sysPath string, kind NetworkInterfaceKind) string {
	// Try udevadm for model name
	if model := getUdevModel(sysPath); model != "" {
		return model
	}

	// Fallback to kind-based names
	switch kind {
	case KindWiFi:
		return "Wi-Fi"
	case KindEthernet:
		return "Ethernet"
	case KindUSB:
		return "USB Network"
	case KindBridge:
		return "Bridge"
	default:
		return name
	}
}

func getUdevModel(sysPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "udevadm", "info", "-q", "property", sysPath)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID_MODEL_FROM_DATABASE=") {
			return strings.TrimPrefix(line, "ID_MODEL_FROM_DATABASE=")
		}
	}
	return ""
}

func readMAC(sysPath string) string {
	data, err := os.ReadFile(sysPath + "/address")
	if err != nil {
		return ""
	}
	mac := strings.TrimSpace(string(data))
	if mac == "00:00:00:00:00:00" {
		return ""
	}
	return mac
}

// interfaceTrafficCounters reads cumulative per-interface RX/TX byte counters
// from /proc/net/dev. This captures ALL traffic on each interface (including
// inbound connections), unlike the connection-level trackedConn accounting.
func interfaceTrafficCounters() (map[string]TrafficCounters, error) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, err
	}

	out := make(map[string]TrafficCounters)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i < 2 { // two header lines
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		fields := strings.Fields(line[idx+1:])
		// Fields: rx_bytes rx_packets rx_errs rx_drop rx_fifo rx_frame
		//         tx_bytes tx_packets tx_errs tx_drop ...
		if len(fields) < 10 {
			continue
		}
		rx, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		tx, err := strconv.ParseInt(fields[8], 10, 64)
		if err != nil {
			continue
		}
		out[name] = TrafficCounters{Down: rx, Up: tx}
	}
	return out, nil
}
