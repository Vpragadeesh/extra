package manager

import (
	"testing"

	"torrent-downloader/config"

	"golang.org/x/time/rate"
)

// Regression test for the "stuck on Downloading metadata" bug.
//
// The torrent client refuses to establish ANY new peer connection while
// DownloadRateLimiter.Tokens() <= 0 (see newConnsAllowed in anacrolix/torrent).
// A limiter created with burst 0 can never accumulate tokens, so an unlimited
// limiter built that way stopped the client from dialing peers: DHT/tracker
// kept queueing addresses but no connection was ever made, so metadata never
// arrived and the TUI hung forever.
func TestDownloadLimiterUnlimitedAllowsNewConnections(t *testing.T) {
	for _, limit := range []int64{0, -1} {
		lim := newDownloadLimiter(limit)

		if lim.Limit() != rate.Inf {
			t.Errorf("newDownloadLimiter(%d): limit = %v, want rate.Inf", limit, lim.Limit())
		}
		// This is the exact check the torrent client makes before dialing.
		if tokens := lim.Tokens(); tokens <= 0 {
			t.Errorf("newDownloadLimiter(%d): Tokens() = %v, want > 0 "+
				"(client will refuse to dial peers and metadata will never arrive)",
				limit, tokens)
		}
	}
}

func TestDownloadLimiterLimited(t *testing.T) {
	const limit = 5000 * 1024 // 5000 KB/s
	lim := newDownloadLimiter(limit)

	if got := lim.Limit(); got != rate.Limit(limit) {
		t.Errorf("limit = %v, want %v", got, rate.Limit(limit))
	}
	if lim.Burst() < 64<<10 {
		t.Errorf("burst = %d, want at least %d", lim.Burst(), 64<<10)
	}
	if tokens := lim.Tokens(); tokens <= 0 {
		t.Errorf("Tokens() = %v, want > 0 so dialing is not blocked", tokens)
	}
}

// Toggling speed in the TUI must never end up in a state that blocks dialing,
// including the unlimited case reachable via the "0" key.
func TestSetSpeedLimitNeverBlocksDialing(t *testing.T) {
	m := &Manager{
		config:      config.DefaultConfig(),
		rateLimiter: newDownloadLimiter(0),
	}

	for _, limit := range []int64{0, 10 * 1024 * 1024, 0, 1} {
		m.SetSpeedLimit(limit)

		if tokens := m.rateLimiter.Tokens(); tokens <= 0 {
			t.Errorf("after SetSpeedLimit(%d): Tokens() = %v, want > 0", limit, tokens)
		}
		if got := m.GetSpeedLimit(); got != limit {
			t.Errorf("after SetSpeedLimit(%d): GetSpeedLimit() = %d", limit, got)
		}
	}
}
