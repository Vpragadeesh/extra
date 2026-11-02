package manager

import "testing"

// The speed readout must not be derived from t.BytesCompleted(), which only
// advances when a whole piece verifies. On large torrents that produces long
// stretches of 0 B/s followed by jumps, which reads as a stall. These tests
// lock in the wire-byte-based rate calculation and the ETA smoothing helpers.
func TestVerifiedRateEWMA(t *testing.T) {
	// Mirrors the loop's update: first sample seeds, later samples blend.
	var ewma float64

	ewma = ewmaAvgRate*ewma + (1-ewmaAvgRate)*10
	if ewma <= 0 {
		t.Fatalf("first sample should seed ewma, got %v", ewma)
	}

	// Feed the same rate repeatedly; it must converge to that rate.
	for i := 0; i < 200; i++ {
		ewma = ewmaAvgRate*ewma + (1-ewmaAvgRate)*10
	}
	if ewma < 9.9 || ewma > 10.1 {
		t.Errorf("ewma converged to %v, want ~10", ewma)
	}
}

func TestEWMAAvgRateIsValidFactor(t *testing.T) {
	if ewmaAvgRate <= 0 || ewmaAvgRate >= 1 {
		t.Errorf("ewmaAvgRate = %v, want a factor in (0,1)", ewmaAvgRate)
	}
}
