package network

import (
	"path/filepath"
	"testing"
	"time"

	"torrent-downloader/config"
)

func TestBucket(t *testing.T) {
	t.Run("unlimited bucket returns no wait", func(t *testing.T) {
		b := NewBucket(0)
		wait := b.Take(1000)
		if wait != 0 {
			t.Errorf("unlimited bucket wait = %v, want 0", wait)
		}
	})

	t.Run("bucket allows burst up to rate", func(t *testing.T) {
		rate := 1000.0 // 1 KB/s
		b := NewBucket(rate)

		// Should allow 1 second worth immediately
		wait := b.Take(1000)
		if wait != 0 {
			t.Errorf("first take wait = %v, want 0", wait)
		}

		// Next take should wait
		wait = b.Take(1000)
		if wait < 900*time.Millisecond || wait > 1100*time.Millisecond {
			t.Errorf("second take wait = %v, want ~1s", wait)
		}
	})

	t.Run("bucket refills over time", func(t *testing.T) {
		rate := 1000.0
		b := NewBucket(rate)

		// Drain
		b.Take(1000)
		wait := b.Take(1000)
		if wait == 0 {
			t.Errorf("expected wait after drain")
		}

		// Wait for refill: need 1s to pay off debt + 1s to accumulate 1000 tokens = 2s
		time.Sleep(2100 * time.Millisecond)

		wait = b.Take(1000)
		if wait != 0 {
			t.Errorf("after refill wait = %v, want 0", wait)
		}
	})

	t.Run("SetRate updates dynamically", func(t *testing.T) {
		b := NewBucket(1000)
		b.Take(1000) // drain

		b.SetRate(2000) // double rate
		// SetRate doesn't grant tokens; need to wait for refill at new rate
		// At 2000 bytes/sec, 1500 bytes takes 750ms to accumulate
		time.Sleep(800 * time.Millisecond)
		wait := b.Take(1500)
		if wait != 0 {
			t.Errorf("after SetRate wait = %v, want 0", wait)
		}
	})
}

func TestLimitsGlobalAndPerNetwork(t *testing.T) {
	t.Run("global limit applies", func(t *testing.T) {
		tmpDir := t.TempDir()
		savePath := filepath.Join(tmpDir, "usage.json")

		limits := NewLimits(nil, savePath)
		defer limits.Close()

		limits.Configure(10000, map[string]config.NetworkPreference{
			"wifi": {SpeedLimit: 5000, Off: false},
			"eth":  {SpeedLimit: 5000, Off: false},
		})

		wait := limits.Take("wifi", 5000)
		if wait != 0 {
			t.Errorf("first take wait = %v", wait)
		}

		// Global limit is 10000/s, we took 5000, next 6000 should wait
		wait = limits.Take("wifi", 6000)
		if wait < 500*time.Millisecond {
			t.Errorf("global limit wait = %v, want ~500ms", wait)
		}
	})

	t.Run("per-network limit applies", func(t *testing.T) {
		tmpDir := t.TempDir()
		savePath := filepath.Join(tmpDir, "usage.json")

		limits := NewLimits(nil, savePath)
		defer limits.Close()

		limits.Configure(10000, map[string]config.NetworkPreference{
			"wifi": {SpeedLimit: 5000, Off: false},
			"eth":  {SpeedLimit: 5000, Off: false},
		})

		// eth limit is 5000/s
		wait := limits.Take("eth", 5000)
		if wait != 0 {
			t.Errorf("eth first take wait = %v", wait)
		}

		wait = limits.Take("eth", 5000)
		if wait < 900*time.Millisecond {
			t.Errorf("eth limit wait = %v, want ~1s", wait)
		}
	})

	t.Run("max of global and per-network", func(t *testing.T) {
		tmpDir := t.TempDir()
		savePath := filepath.Join(tmpDir, "usage.json")

		limits := NewLimits(nil, savePath)
		defer limits.Close()

		limits.Configure(10000, map[string]config.NetworkPreference{
			"wifi": {SpeedLimit: 5000, Off: false},
			"eth":  {SpeedLimit: 5000, Off: false},
		})

		// Pre-load global bucket with 11000 bytes (10000 rate = 1s debt)
		limits.Take("wifi", 5000)
		limits.Take("wifi", 6000)

		// Now wifi per-network has 5000 taken, global has 11000 taken (1000 debt)
		// Take 10000 on wifi: global needs ~100ms, per-network needs ~1s
		wait := limits.Take("wifi", 10000)
		// Max of global (~100ms) and per-network (~1s) should be ~1s
		if wait < 900*time.Millisecond {
			t.Errorf("combined wait = %v, want ~1s", wait)
		}
	})
}

func TestLimitsDataCap(t *testing.T) {
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "usage.json")

	var limitReachedNetwork string
	limits := NewLimits(func(id string) { limitReachedNetwork = id }, savePath)
	defer limits.Close()

	limits.Configure(0, map[string]config.NetworkPreference{
		"wifi": {DataLimit: 1000, DataLimitPeriod: string(PeriodDay), Off: false},
	})

	t.Run("tracks usage", func(t *testing.T) {
		limits.AddTraffic("wifi", 500, 0)
		used := limits.UsedByPeriod()
		if used["wifi"] != 500 {
			t.Errorf("usage = %d, want 500", used["wifi"])
		}

		limits.AddTraffic("wifi", 300, 0)
		used = limits.UsedByPeriod()
		if used["wifi"] != 800 {
			t.Errorf("usage = %d, want 800", used["wifi"])
		}
	})

	t.Run("LimitReached triggers callback", func(t *testing.T) {
		// 800 used, limit is 1000, add 200 more -> should trigger
		limits.AddTraffic("wifi", 200, 0)
		time.Sleep(10 * time.Millisecond) // allow callback
		if limitReachedNetwork != "wifi" {
			t.Errorf("limit callback not triggered, got %q", limitReachedNetwork)
		}
	})

	t.Run("LimitReached returns true after limit", func(t *testing.T) {
		if !limits.LimitReached("wifi") {
			t.Errorf("LimitReached = false, want true")
		}
	})

	t.Run("ResetUsage clears current period", func(t *testing.T) {
		err := limits.ResetUsage("wifi")
		if err != nil {
			t.Errorf("ResetUsage error: %v", err)
		}
		if limits.LimitReached("wifi") {
			t.Errorf("LimitReached still true after reset")
		}
		used := limits.UsedByPeriod()
		if used["wifi"] != 0 {
			t.Errorf("usage after reset = %d, want 0", used["wifi"])
		}
	})
}

func TestLimitsPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "usage.json")

	// Create limits and add usage
	limits1 := NewLimits(nil, savePath)
	limits1.Configure(0, map[string]config.NetworkPreference{
		"wifi": {DataLimit: 10000, DataLimitPeriod: string(PeriodDay), Off: false},
	})
	limits1.AddTraffic("wifi", 5000, 0)
	limits1.Save()
	limits1.Close()

	// Reload
	limits2 := NewLimits(nil, savePath)
	defer limits2.Close()

	used := limits2.UsedByPeriod()
	if used["wifi"] != 5000 {
		t.Errorf("persisted usage = %d, want 5000", used["wifi"])
	}
}

func TestLimitsOffNetwork(t *testing.T) {
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "usage.json")

	limits := NewLimits(nil, savePath)
	defer limits.Close()

	limits.Configure(10000, map[string]config.NetworkPreference{
		"wifi": {SpeedLimit: 5000, Off: true}, // Disabled
		"eth":  {SpeedLimit: 5000, Off: false},
	})

	// wifi is off, should not be rate limited
	wait := limits.Take("wifi", 100000)
	if wait != 0 {
		t.Errorf("off network wait = %v, want 0", wait)
	}

	// eth is on, should be rate limited
	wait = limits.Take("eth", 5000)
	if wait != 0 {
		t.Errorf("eth first take wait = %v", wait)
	}
	wait = limits.Take("eth", 5000)
	if wait < 900*time.Millisecond {
		t.Errorf("eth limit wait = %v, want ~1s", wait)
	}
}
