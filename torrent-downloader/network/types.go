package network

import (
	"encoding/json"
	"time"
)

// NetworkInterfaceKind classifies the type of network interface.
type NetworkInterfaceKind string

const (
	KindWiFi     NetworkInterfaceKind = "wifi"
	KindUSB      NetworkInterfaceKind = "usb"
	KindEthernet NetworkInterfaceKind = "ethernet"
	KindBridge   NetworkInterfaceKind = "bridge"
	KindOther    NetworkInterfaceKind = "other"
)

// NetworkAddress represents a single IP address on an interface.
type NetworkAddress struct {
	Address string `json:"address"`
	Family  int    `json:"family"` // 4 or 6
	Netmask string `json:"netmask,omitempty"`
	Subnet  string `json:"subnet,omitempty"` // CIDR notation, e.g. "192.168.1.0/24"
}

// NetworkInterfaceInfo describes a network interface as seen by the OS.
type NetworkInterfaceInfo struct {
	ID          string               `json:"id"`           // OS device name: "en0", "wlp2s0", "eth0"
	Device      string               `json:"device"`       // Same as ID on Linux; adapter name on Windows
	DisplayName string               `json:"display_name"` // Human-readable: "Wi-Fi", "Ethernet", "USB Network"
	Addresses   []NetworkAddress     `json:"addresses"`
	Kind        NetworkInterfaceKind `json:"kind"`
	MAC         string               `json:"mac,omitempty"`
}

// NetworkStatus represents the operational state of a network for a download.
type NetworkStatus string

const (
	StatusOn          NetworkStatus = "on"
	StatusOff         NetworkStatus = "off"
	StatusOffline     NetworkStatus = "offline"
	StatusUnreachable NetworkStatus = "unreachable"
	StatusFailed      NetworkStatus = "failed"
	StatusLimit       NetworkStatus = "limit"
)

// ProxyConfig holds per-interface proxy settings.
type ProxyConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Type     string `yaml:"type" json:"type"` // http, https, socks4, socks5
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Username string `yaml:"username" json:"username,omitempty"`
	Password string `yaml:"password" json:"password,omitempty"`
}

// NetworkPreference is re-exported from config package.
// type NetworkPreference = config.NetworkPreference

// NetworkLoad is used for peer load balancing across networks.
type NetworkLoad struct {
	ID    string `json:"id"`
	Peers int    `json:"peers"`
}

// NetworkProgress represents per-network transfer statistics for the TUI.
type NetworkProgress struct {
	InterfaceID     string  `json:"interface_id"`
	Label           string  `json:"label"`
	Kind            string  `json:"kind"`
	DownloadSpeed   float64 `json:"download_speed"` // bytes/sec
	UploadSpeed     float64 `json:"upload_speed"`   // bytes/sec
	ActivePeers     int     `json:"active_peers"`
	BytesDownloaded int64   `json:"bytes_downloaded"`
	BytesUploaded   int64   `json:"bytes_uploaded"`
	Status          string  `json:"status"`
	DataUsed        int64   `json:"data_used"`         // bytes in current period
	DataLimit       int64   `json:"data_limit"`        // bytes, 0 = unlimited
	DataLimitPeriod string  `json:"data_limit_period"` // "day" | "week" | "month"
}

// PickNetwork selects the network with the fewest peers (ported from plexo peers.ts).
// Returns empty string if no networks provided.
func PickNetwork(networks []NetworkLoad) string {
	var best *NetworkLoad
	for i := range networks {
		if best == nil || networks[i].Peers < best.Peers {
			best = &networks[i]
		}
	}
	if best == nil {
		return ""
	}
	return best.ID
}

// CreditPiece splits a verified piece's length between the networks that delivered it,
// in proportion to what each received. Whole bytes that add up to length exactly;
// remainder goes to the largest share. If no network delivered bytes, all goes to fallback.
func CreditPiece(received map[string]int64, length int64, fallback string) map[string]int64 {
	var total int64
	var largestID string
	var largestBytes int64

	for id, bytes := range received {
		if bytes <= 0 {
			continue
		}
		total += bytes
		if bytes > largestBytes {
			largestBytes = bytes
			largestID = id
		}
	}

	if total == 0 {
		return map[string]int64{fallback: length}
	}

	shares := make(map[string]int64, len(received))
	var given int64
	for id, bytes := range received {
		if bytes <= 0 {
			continue
		}
		shares[id] = (length * bytes) / total
		given += shares[id]
	}

	if largestID != "" {
		shares[largestID] += length - given
	}

	return shares
}

// BitfieldOf creates a bitfield from a slice of piece completion booleans.
// First piece is the highest bit of the first byte (matching webtorrent/anacrolix).
func BitfieldOf(done []bool) []byte {
	if len(done) == 0 {
		return nil
	}
	bits := make([]byte, (len(done)+7)/8)
	for i, isDone := range done {
		if isDone {
			bits[i>>3] |= 0x80 >> (i & 7)
		}
	}
	return bits
}

// DataLimitPeriod represents the calendar period for data caps.
type DataLimitPeriod string

const (
	PeriodDay   DataLimitPeriod = "day"
	PeriodWeek  DataLimitPeriod = "week"
	PeriodMonth DataLimitPeriod = "month"
)

// PeriodKey returns a unique key for a period at a given time (local timezone).
// Matches plexo's dataPeriodKey logic.
func PeriodKey(period DataLimitPeriod, t time.Time) string {
	// Use local time for period boundaries
	year, month, _ := t.Date()
	switch period {
	case PeriodDay:
		return t.Format("2006-01-02")
	case PeriodWeek:
		// Week starts Monday. Find the Monday of this week.
		weekday := t.Weekday()
		daysToMonday := int(weekday - time.Monday)
		if daysToMonday < 0 {
			daysToMonday += 7
		}
		monday := t.AddDate(0, 0, -daysToMonday)
		return monday.Format("2006-01-02") + "-week"
	case PeriodMonth:
		return time.Date(year, month, 1, 0, 0, 0, 0, time.Local).Format("2006-01")
	default:
		return t.Format("2006-01")
	}
}

// MarshalJSON implements custom JSON marshaling for NetworkInterfaceKind.
func (k NetworkInterfaceKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(k))
}

// UnmarshalJSON implements custom JSON unmarshaling for NetworkInterfaceKind.
func (k *NetworkInterfaceKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*k = NetworkInterfaceKind(s)
	return nil
}

// MarshalJSON implements custom JSON marshaling for NetworkStatus.
func (s NetworkStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// UnmarshalJSON implements custom JSON unmarshaling for NetworkStatus.
func (s *NetworkStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = NetworkStatus(str)
	return nil
}

// MarshalJSON implements custom JSON marshaling for DataLimitPeriod.
func (p DataLimitPeriod) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(p))
}

// UnmarshalJSON implements custom JSON unmarshaling for DataLimitPeriod.
func (p *DataLimitPeriod) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*p = DataLimitPeriod(str)
	return nil
}
