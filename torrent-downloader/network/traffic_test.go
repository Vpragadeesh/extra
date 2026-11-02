package network

import (
	"path/filepath"
	"testing"
	"time"

	"torrent-downloader/config"
)

func TestComputeTrafficDeltas(t *testing.T) {
	t.Run("fresh interface has no rate or delta", func(t *testing.T) {
		cur := map[string]TrafficCounters{"wlan0": {Down: 5000, Up: 200}}
		rates, deltas := computeTrafficDeltas(nil, cur, 1.0)
		if rates["wlan0"].Down != 0 || rates["wlan0"].Up != 0 {
			t.Errorf("fresh interface rates = %+v, want 0", rates["wlan0"])
		}
		if deltas["wlan0"].Down != 0 || deltas["wlan0"].Up != 0 {
			t.Errorf("fresh interface deltas = %+v, want 0", deltas["wlan0"])
		}
	})

	t.Run("second sample computes rate and delta", func(t *testing.T) {
		last := map[string]TrafficCounters{"wlan0": {Down: 10000, Up: 1000}}
		cur := map[string]TrafficCounters{"wlan0": {Down: 20000, Up: 1200}}
		rates, deltas := computeTrafficDeltas(last, cur, 2.0)
		if rates["wlan0"].Down != 5000 {
			t.Errorf("rate down = %d, want 5000", rates["wlan0"].Down)
		}
		if rates["wlan0"].Up != 100 {
			t.Errorf("rate up = %d, want 100", rates["wlan0"].Up)
		}
		if deltas["wlan0"].Down != 10000 {
			t.Errorf("delta down = %d, want 10000", deltas["wlan0"].Down)
		}
		if deltas["wlan0"].Up != 200 {
			t.Errorf("delta up = %d, want 200", deltas["wlan0"].Up)
		}
	})

	t.Run("counter reset yields zeros", func(t *testing.T) {
		last := map[string]TrafficCounters{"wlan0": {Down: 100000, Up: 50000}}
		cur := map[string]TrafficCounters{"wlan0": {Down: 100, Up: 10}}
		rates, deltas := computeTrafficDeltas(last, cur, 1.0)
		if rates["wlan0"].Down != 0 || rates["wlan0"].Up != 0 {
			t.Errorf("after reset rates = %+v, want 0", rates["wlan0"])
		}
		if deltas["wlan0"].Down != 0 || deltas["wlan0"].Up != 0 {
			t.Errorf("after reset deltas = %+v, want 0", deltas["wlan0"])
		}
	})
}

func TestMonitorTrafficSampling(t *testing.T) {
	m := NewMonitor(nil)
	m.update([]NetworkInterfaceInfo{{ID: "wlan0"}})
	// Inject fake counter source.
	values := []map[string]TrafficCounters{
		{"wlan0": {Down: 1000, Up: 100}},
		{"wlan0": {Down: 4000, Up: 200}}, // +3000 down / +100 up over ~1s
	}
	steps := 0
	m.countersFn = func() (map[string]TrafficCounters, error) {
		i := steps
		if i >= len(values) {
			i = len(values) - 1
		}
		return values[i], nil
	}

	// First sample: establishes baseline, no rates.
	m.sampleTraffic()
	steps = 1
	// Second sample: force elapsed time so the rate is deterministic.
	m.lastCountersAt = time.Now().Add(-2 * time.Second)
	m.sampleTraffic()

	rates := m.TrafficRates()
	// Elapsed is slightly more than the backdated 2s (time.Now() runs later),
	// so rates truncate below the ideal value; assert with tolerance.
	if rates["wlan0"].Down < 1400 || rates["wlan0"].Down > 1500 {
		t.Errorf("rate down = %d, want ~1500 (3000 bytes / ~2s)", rates["wlan0"].Down)
	}
	if rates["wlan0"].Up < 45 || rates["wlan0"].Up > 50 {
		t.Errorf("rate up = %d, want ~50 (100 bytes / ~2s)", rates["wlan0"].Up)
	}
}

func TestMonitorTrafficCallback(t *testing.T) {
	var got map[string]TrafficDelta
	m := NewMonitor(nil)
	m.SetTrafficCallback(func(deltas map[string]TrafficDelta) { got = deltas })
	m.countersFn = func() (map[string]TrafficCounters, error) {
		return map[string]TrafficCounters{"eth": {Down: 100, Up: 10}}, nil
	}
	m.update([]NetworkInterfaceInfo{{ID: "eth"}})

	// Baseline pass, then delta pass.
	m.sampleTraffic()
	m.countersFn = func() (map[string]TrafficCounters, error) {
		return map[string]TrafficCounters{"eth": {Down: 500, Up: 30}}, nil
	}
	m.lastCountersAt = time.Now().Add(-1 * time.Second)
	m.sampleTraffic()

	if got == nil {
		t.Fatal("traffic callback not invoked")
	}
	if d := got["eth"]; d.Down != 400 || d.Up != 20 {
		t.Errorf("callback deltas = %+v, want {400 20}", d)
	}
}

func TestAddTraffic(t *testing.T) {
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "usage.json")

	var reached string
	limits := NewLimits(func(id string) { reached = id }, savePath)
	defer limits.Close()

	limits.Configure(0, map[string]config.NetworkPreference{
		"wifi": {DataLimit: 1000, DataLimitPeriod: string(PeriodDay), Off: false},
	})

	limits.AddTraffic("wifi", 750, 100)
	used := limits.UsedByPeriod()
	if used["wifi"] != 850 {
		t.Errorf("usage after AddTraffic = %d, want 850", used["wifi"])
	}

	// Crossing the cap (850 + 200 >= 1000) fires the limit callback.
	limits.AddTraffic("wifi", 200, 0)
	time.Sleep(10 * time.Millisecond)
	if reached != "wifi" {
		t.Errorf("limit callback not triggered by AddTraffic, got %q", reached)
	}
}

func TestTakeDoesNotRecordUsage(t *testing.T) {
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "usage.json")

	limits := NewLimits(nil, savePath)
	defer limits.Close()

	limits.Configure(0, map[string]config.NetworkPreference{
		"wifi": {SpeedLimit: 5000, Off: false},
	})

	// Usage comes from OS-level interface counters (AddTraffic), not from Take.
	limits.AddTraffic("wifi", 100, 0)
	limits.Take("wifi", 5000)
	limits.Take("wifi", 5000)

	used := limits.UsedByPeriod()
	if used["wifi"] != 100 {
		t.Errorf("usage after Take = %d, want 100 (Take must not record usage)", used["wifi"])
	}
}
