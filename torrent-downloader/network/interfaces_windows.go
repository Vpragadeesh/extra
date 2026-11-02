//go:build windows

package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// listPlatformDevices discovers network interfaces on Windows via Get-NetAdapter.
func listPlatformDevices() ([]PlatformDevice, error) {
	adapters, err := getWindowsAdapters()
	if err != nil {
		// Fall back to raw device names from net.Interfaces()
		return fallbackDevices()
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var devices []PlatformDevice
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := iface.Name
		adapter := adapters[name]
		if adapter == nil {
			continue
		}

		kind := KindOther
		// NDIS Physical Medium: 1 = Wireless LAN, 9 = Native 802.11, 14 = Ethernet
		if adapter.NdisPhysicalMedium == 1 || adapter.NdisPhysicalMedium == 9 {
			kind = KindWiFi
		} else if adapter.NdisPhysicalMedium == 14 {
			kind = KindEthernet
		} else {
			// Fallback to description-based classification
			kind = classifyWindowsDescription(adapter.InterfaceDescription)
		}

		displayName := adapter.Name // Windows shows friendly name here
		if adapter.InterfaceDescription != "" {
			displayName = adapter.InterfaceDescription
		}

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

type windowsAdapter struct {
	Name                 string `json:"Name"`
	InterfaceDescription string `json:"InterfaceDescription"`
	NdisPhysicalMedium   int    `json:"NdisPhysicalMedium"`
}

func getWindowsAdapters() (map[string]*windowsAdapter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"powershell.exe",
		"-NoLogo", "-NoProfile", "-NonInteractive",
		"-Command",
		"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-NetAdapter -ErrorAction Stop | Select-Object Name, InterfaceDescription, NdisPhysicalMedium | ConvertTo-Json -Compress",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var adapters []windowsAdapter
	if err := json.Unmarshal(bytes.TrimSpace(output), &adapters); err != nil {
		// Could be a single object instead of array
		var single windowsAdapter
		if err := json.Unmarshal(bytes.TrimSpace(output), &single); err == nil {
			adapters = []windowsAdapter{single}
		} else {
			return nil, err
		}
	}

	result := make(map[string]*windowsAdapter)
	for i := range adapters {
		if adapters[i].Name != "" {
			result[adapters[i].Name] = &adapters[i]
		}
	}
	return result, nil
}

func classifyWindowsDescription(desc string) NetworkInterfaceKind {
	desc = strings.ToLower(desc)
	switch {
	case strings.Contains(desc, "wireless") || strings.Contains(desc, "wi-fi") || strings.Contains(desc, "wifi") || strings.Contains(desc, "802.11"):
		return KindWiFi
	case strings.Contains(desc, "rndis") || strings.Contains(desc, "tether") || strings.Contains(desc, "mobile"):
		return KindUSB
	case strings.Contains(desc, "ethernet") || strings.Contains(desc, "lan") || strings.Contains(desc, "gigabit"):
		return KindEthernet
	case strings.Contains(desc, "bridge"):
		return KindBridge
	default:
		return KindOther
	}
}

func fallbackDevices() ([]PlatformDevice, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var devices []PlatformDevice
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		kind := KindOther
		name := strings.ToLower(iface.Name)
		if strings.HasPrefix(name, "wi") || strings.HasPrefix(name, "wl") {
			kind = KindWiFi
		} else if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "en") {
			kind = KindEthernet
		}
		var mac string
		if iface.HardwareAddr != nil {
			mac = iface.HardwareAddr.String()
		}
		devices = append(devices, PlatformDevice{
			ID:          iface.Name,
			Device:      iface.Name,
			DisplayName: iface.Name,
			Kind:        kind,
			MAC:         mac,
		})
	}
	return devices, nil
}

// winAdapterStats mirrors fields of Get-NetAdapterStatistics.
type winAdapterStats struct {
	Name          string
	ReceivedBytes int64
	SentBytes     int64
}

// interfaceTrafficCounters reads cumulative per-interface RX/TX byte counters
// from Get-NetAdapterStatistics.
func interfaceTrafficCounters() (map[string]TrafficCounters, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		"Get-NetAdapterStatistics | Select-Object Name, ReceivedBytes, SentBytes | ConvertTo-Json -Compress")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	counters := make(map[string]TrafficCounters)

	var rows []winAdapterStats
	var once winAdapterStats
	if err := json.Unmarshal(output, &rows); err != nil {
		// Single object (one adapter) is emitted without the array wrapper.
		if err := json.Unmarshal(output, &once); err != nil {
			return nil, err
		}
		rows = []winAdapterStats{once}
	}
	for _, s := range rows {
		if s.Name == "" {
			continue
		}
		counters[s.Name] = TrafficCounters{Down: s.ReceivedBytes, Up: s.SentBytes}
	}
	return counters, nil
}
