package network

import (
	"torrent-downloader/config"
)

// MergeNetworkPreferences updates the config with discovered network interfaces.
// Called on startup to auto-populate preferences for new interfaces.
func MergeNetworkPreferences(cfg *config.Config, interfaces []NetworkInterfaceInfo) {
	if cfg.NetworkPreferences == nil {
		cfg.NetworkPreferences = make(map[string]config.NetworkPreference)
	}

	for _, iface := range interfaces {
		if _, exists := cfg.NetworkPreferences[iface.ID]; !exists {
			cfg.NetworkPreferences[iface.ID] = config.NetworkPreference{
				CustomName:      iface.DisplayName,
				Off:             false,
				SpeedLimit:      0,
				DataLimit:       0,
				DataLimitPeriod: string(PeriodMonth),
			}
		}
	}
}

// GetEnabledNetworks returns the list of network IDs that are enabled (not Off).
func GetEnabledNetworks(cfg *config.Config) []string {
	var enabled []string
	for id, pref := range cfg.NetworkPreferences {
		if !pref.Off {
			enabled = append(enabled, id)
		}
	}
	return enabled
}

// GetNetworkPreference returns the preference for a network, or default if not set.
func GetNetworkPreference(cfg *config.Config, networkID string) config.NetworkPreference {
	if pref, ok := cfg.NetworkPreferences[networkID]; ok {
		return pref
	}
	return config.NetworkPreference{
		Off:             false,
		SpeedLimit:      0,
		DataLimit:       0,
		DataLimitPeriod: string(PeriodMonth),
	}
}

// SetNetworkEnabled enables or disables a network.
func SetNetworkEnabled(cfg *config.Config, networkID string, enabled bool) {
	if cfg.NetworkPreferences == nil {
		cfg.NetworkPreferences = make(map[string]config.NetworkPreference)
	}
	pref := cfg.NetworkPreferences[networkID]
	pref.Off = !enabled
	cfg.NetworkPreferences[networkID] = pref
}

// SetNetworkSpeedLimit sets the speed limit for a network (0 = unlimited).
func SetNetworkSpeedLimit(cfg *config.Config, networkID string, limit int64) {
	if cfg.NetworkPreferences == nil {
		cfg.NetworkPreferences = make(map[string]config.NetworkPreference)
	}
	pref := cfg.NetworkPreferences[networkID]
	pref.SpeedLimit = limit
	cfg.NetworkPreferences[networkID] = pref
}

// SetNetworkDataLimit sets the data cap for a network.
func SetNetworkDataLimit(cfg *config.Config, networkID string, limit int64, period string) {
	if cfg.NetworkPreferences == nil {
		cfg.NetworkPreferences = make(map[string]config.NetworkPreference)
	}
	pref := cfg.NetworkPreferences[networkID]
	pref.DataLimit = limit
	if period != "" {
		pref.DataLimitPeriod = period
	}
	cfg.NetworkPreferences[networkID] = pref
}
