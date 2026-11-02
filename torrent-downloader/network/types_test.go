package network

import (
	"testing"
	"time"
)

func TestPickNetwork(t *testing.T) {
	tests := []struct {
		name     string
		networks []NetworkLoad
		want     string
	}{
		{
			name:     "empty list",
			networks: []NetworkLoad{},
			want:     "",
		},
		{
			name:     "single network",
			networks: []NetworkLoad{{ID: "wifi", Peers: 5}},
			want:     "wifi",
		},
		{
			name: "fewest peers wins",
			networks: []NetworkLoad{
				{ID: "wifi", Peers: 10},
				{ID: "eth", Peers: 3},
				{ID: "usb", Peers: 7},
			},
			want: "eth",
		},
		{
			name: "tie goes to first",
			networks: []NetworkLoad{
				{ID: "wifi", Peers: 5},
				{ID: "eth", Peers: 5},
			},
			want: "wifi",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PickNetwork(tc.networks)
			if got != tc.want {
				t.Errorf("PickNetwork() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCreditPiece(t *testing.T) {
	tests := []struct {
		name     string
		received map[string]int64
		length   int64
		fallback string
		want     map[string]int64
	}{
		{
			name:     "no bytes received",
			received: map[string]int64{},
			length:   1000,
			fallback: "wifi",
			want:     map[string]int64{"wifi": 1000},
		},
		{
			name:     "single network",
			received: map[string]int64{"wifi": 1000},
			length:   1000,
			fallback: "eth",
			want:     map[string]int64{"wifi": 1000},
		},
		{
			name: "proportional split",
			received: map[string]int64{
				"wifi": 600,
				"eth":  400,
			},
			length:   1000,
			fallback: "usb",
			want: map[string]int64{
				"wifi": 600,
				"eth":  400,
			},
		},
		{
			name: "remainder to largest",
			received: map[string]int64{
				"wifi": 333,
				"eth":  333,
				"usb":  334,
			},
			length:   1000,
			fallback: "other",
			want: map[string]int64{
				"wifi": 333,
				"eth":  333,
				"usb":  334,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CreditPiece(tc.received, tc.length, tc.fallback)
			// Check total
			var total int64
			for _, v := range got {
				total += v
			}
			if total != tc.length {
				t.Errorf("CreditPiece total = %d, want %d", total, tc.length)
			}
			// Check each network
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("CreditPiece[%s] = %d, want %d", k, got[k], v)
				}
			}
		})
	}
}

func TestBitfieldOf(t *testing.T) {
	tests := []struct {
		name string
		done []bool
		want []byte
	}{
		{
			name: "empty",
			done: []bool{},
			want: nil,
		},
		{
			name: "single piece done",
			done: []bool{true},
			want: []byte{0x80},
		},
		{
			name: "first 8 pieces",
			done: []bool{true, true, true, true, true, true, true, true},
			want: []byte{0xFF},
		},
		{
			name: "9 pieces, first byte full, second has 1 bit",
			done: []bool{true, true, true, true, true, true, true, true, true},
			want: []byte{0xFF, 0x80},
		},
		{
			name: "alternating",
			done: []bool{true, false, true, false, true, false, true, false},
			want: []byte{0xAA},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BitfieldOf(tc.done)
			if len(got) != len(tc.want) {
				t.Errorf("BitfieldOf len = %d, want %d", len(got), len(tc.want))
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("BitfieldOf[%d] = 0x%02x, want 0x%02x", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestPeriodKey(t *testing.T) {
	// Fixed time for deterministic tests
	tm := time.Date(2026, 1, 15, 12, 30, 0, 0, time.Local)

	tests := []struct {
		period DataLimitPeriod
		want   string
	}{
		{PeriodDay, "2026-01-15"},
		{PeriodWeek, "2026-01-12-week"}, // Monday of that week
		{PeriodMonth, "2026-01"},
	}

	for _, tc := range tests {
		t.Run(string(tc.period), func(t *testing.T) {
			got := PeriodKey(tc.period, tm)
			if got != tc.want {
				t.Errorf("PeriodKey(%s) = %q, want %q", tc.period, got, tc.want)
			}
		})
	}
}
