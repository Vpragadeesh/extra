//go:build darwin

package network

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// listPlatformDevices discovers network interfaces on macOS via networksetup.
func listPlatformDevices() ([]PlatformDevice, error) {
	// Get hardware port names from networksetup
	portNames, err := getMacHardwarePortNames()
	if err != nil {
		// Fall back to raw device names if networksetup fails
		portNames = make(map[string]string)
	}

	// Get interfaces from OS
	ifaces, err := netInterfaces()
	if err != nil {
		return nil, err
	}

	var devices []PlatformDevice
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := iface.Name
		displayName := portNames[name]
		if displayName == "" {
			displayName = name
		}
		kind := classifyMacDevice(displayName)

		// Get MAC address
		var mac string
		if iface.HardwareAddr != nil {
			mac = iface.HardwareAddr.String()
		}

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

// getMacHardwarePortNames runs `networksetup -listallhardwareports` and parses the output.
// Returns a map of device name (e.g. "en0") to hardware port name (e.g. "Wi-Fi").
func getMacHardwarePortNames() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "networksetup", "-listallhardwareports")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	deviceToName := make(map[string]string)
	blocks := strings.Split(strings.TrimSpace(string(output)), "\n\n")

	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		var port, device string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Hardware Port:") {
				port = strings.TrimSpace(strings.TrimPrefix(line, "Hardware Port:"))
			} else if strings.HasPrefix(line, "Device:") {
				device = strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
			}
		}
		// A BSD device name (en0, bridge0) is how a real block is told from trailing VLAN list
		if port != "" && device != "" && isBSIDevice(device) {
			deviceToName[device] = port
		}
	}

	return deviceToName, nil
}

func isBSIDevice(device string) bool {
	// en0, en1, bridge0, etc.
	return len(device) > 0 && (device[0] == 'e' || device[0] == 'b') && strings.ContainsAny(device, "0123456789")
}

func classifyMacDevice(hardwarePortName string) NetworkInterfaceKind {
	name := strings.ToLower(hardwarePortName)
	switch {
	case strings.Contains(name, "wi-fi") || strings.Contains(name, "wifi") ||
		strings.Contains(name, "wireless") || strings.Contains(name, "wlan") ||
		strings.Contains(name, "802.11") || strings.Contains(name, "airport"):
		return KindWiFi
	case strings.Contains(name, "rndis") || strings.Contains(name, "remote ndis") ||
		strings.Contains(name, "tether") || strings.Contains(name, "apple mobile device"):
		return KindUSB
	case strings.Contains(name, "iphone") || strings.Contains(name, "ipad") || strings.Contains(name, "usb"):
		return KindUSB
	case strings.Contains(name, "bridge"):
		return KindBridge
	case strings.Contains(name, "ethernet") || strings.Contains(name, "lan"):
		return KindEthernet
	default:
		return KindOther
	}
}

// netInterfaces is a wrapper to allow testing.
var netInterfaces = func() ([]net.Interface, error) {
	return net.Interfaces()
}

// interfaceTrafficCounters reads cumulative per-interface RX/TX byte counters
// from `netstat -ib`, preferring the <Link#N> row which counts all families.
func interfaceTrafficCounters() (map[string]TrafficCounters, error) {
	out, err := exec.Command("netstat", "-ib").Output()
	if err != nil {
		return nil, err
	}

	counters := make(map[string]TrafficCounters)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	first := true
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if first {
			first = false
			if len(fields) > 0 && fields[0] == "Name" {
				continue // header: Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll
			}
		}
		if len(fields) < 11 {
			continue
		}
		// Per-family rows repeat (or partially) count; keep the first row seen
		// per name, which netstat places as the <Link> row.
		if _, seen := counters[fields[0]]; seen {
			continue
		}
		down, err := strconv.ParseInt(fields[6], 10, 64)
		if err != nil {
			continue
		}
		up, err := strconv.ParseInt(fields[10], 10, 64)
		if err != nil {
			continue
		}
		counters[fields[0]] = TrafficCounters{Down: down, Up: up}
	}
	return counters, nil
}
