package network

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"torrent-downloader/config"
)

// Bucket is a token bucket rate limiter.
// It refills at `rate` bytes per second and holds up to 1 second's worth of tokens.
// Taking more than available leaves it in debt; the debt determines the wait time.
type Bucket struct {
	mu      sync.Mutex
	rate    float64 // bytes per second
	tokens  float64
	updated time.Time
}

// NewBucket creates a new token bucket with the given rate (bytes/sec).
// Rate <= 0 means unlimited (no waiting).
func NewBucket(rate float64) *Bucket {
	if rate <= 0 {
		return nil
	}
	return &Bucket{
		rate:    rate,
		tokens:  rate, // Start full (1 second burst)
		updated: time.Now(),
	}
}

// SetRate updates the bucket's rate dynamically.
func (b *Bucket) SetRate(rate float64) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.updated).Seconds()
	// Refill before changing rate
	newTokens := b.tokens + elapsed*b.rate
	if newTokens > b.rate {
		newTokens = b.rate
	}
	b.tokens = newTokens
	b.updated = now

	b.rate = rate
	// Cap tokens at new rate if positive
	if rate > 0 && b.tokens > rate {
		b.tokens = rate
	}
}

// Take consumes `bytes` tokens and returns the wait duration in nanoseconds.
// Returns 0 if unlimited or tokens available.
func (b *Bucket) Take(bytes int64) time.Duration {
	if b == nil || bytes <= 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.updated).Seconds()
	b.updated = now

	// Refill tokens based on elapsed time.
	// Tokens can go negative (debt). Only cap at rate when positive.
	newTokens := b.tokens + elapsed*b.rate
	if newTokens > b.rate {
		newTokens = b.rate
	}
	b.tokens = newTokens

	b.tokens -= float64(bytes)
	if b.tokens >= 0 {
		return 0
	}

	// In debt: calculate wait time to replenish
	waitSecs := -b.tokens / b.rate
	return time.Duration(waitSecs * float64(time.Second))
}

// Limits manages global and per-network rate limits, plus data caps.
type Limits struct {
	mu             sync.Mutex
	global         *Bucket
	perNetwork     map[string]*Bucket
	dataLimits     map[string]int64            // networkID -> bytes per period
	periods        map[string]DataLimitPeriod  // networkID -> period
	offNetworks    map[string]bool             // networkID -> true if disabled
	usage          map[string]map[string]int64 // periodKey -> networkID -> bytes
	onLimitReached func(networkID string)
	savePath       string
	saveTimer      *time.Timer
	saveInterval   time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewLimits creates a new Limits manager.
// onLimitReached is called when a network crosses its data limit threshold.
// savePath is the file path to persist usage data.
func NewLimits(onLimitReached func(networkID string), savePath string) *Limits {
	ctx, cancel := context.WithCancel(context.Background())
	l := &Limits{
		perNetwork:     make(map[string]*Bucket),
		dataLimits:     make(map[string]int64),
		periods:        make(map[string]DataLimitPeriod),
		offNetworks:    make(map[string]bool),
		usage:          make(map[string]map[string]int64),
		onLimitReached: onLimitReached,
		savePath:       savePath,
		saveInterval:   10 * time.Second,
		ctx:            ctx,
		cancel:         cancel,
	}
	// Load persisted usage
	l.loadUsage()
	return l
}

// Configure updates limits from user preferences.
func (l *Limits) Configure(globalLimit int64, prefs map[string]config.NetworkPreference) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Global limit (or slow mode)
	if globalLimit > 0 {
		l.global = NewBucket(float64(globalLimit))
	} else {
		l.global = nil
	}

	// Per-network limits
	newPerNetwork := make(map[string]*Bucket)
	newDataLimits := make(map[string]int64)
	newPeriods := make(map[string]DataLimitPeriod)
	newOffNetworks := make(map[string]bool)

	for id, pref := range prefs {
		if pref.Off {
			newOffNetworks[id] = true
			continue
		}
		if pref.SpeedLimit > 0 {
			newPerNetwork[id] = NewBucket(float64(pref.SpeedLimit))
		}
		if pref.DataLimit > 0 {
			newDataLimits[id] = pref.DataLimit
			period := DataLimitPeriod(pref.DataLimitPeriod)
			if period == "" {
				period = PeriodMonth
			}
			newPeriods[id] = period
		}
	}

	l.perNetwork = newPerNetwork
	l.dataLimits = newDataLimits
	l.periods = newPeriods
	l.offNetworks = newOffNetworks
}

// Take applies rate limiting for `bytes` about to flow on `networkID` and
// returns the wait duration (max of global and per-network bucket waits).
// It does NOT record usage: data usage now comes from OS-level interface
// counters via AddTraffic, which captures inbound connections too.
func (l *Limits) Take(networkID string, bytes int64) time.Duration {
	if bytes <= 0 {
		return 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Skip rate limiting entirely for disabled networks
	if l.offNetworks[networkID] {
		return 0
	}

	var globalWait, networkWait time.Duration
	if l.global != nil {
		globalWait = l.global.Take(bytes)
	}
	if bucket, ok := l.perNetwork[networkID]; ok {
		networkWait = bucket.Take(bytes)
	}

	return max(globalWait, networkWait)
}

// AddTraffic records bytes transferred on a network interface over the last
// sampling interval (from the Monitor's OS-level counters). It accumulates
// usage for data caps across ALL interface traffic (download + upload),
// matching metered-connection semantics, and fires onLimitReached on crossing.
func (l *Limits) AddTraffic(networkID string, down, up int64) {
	if down <= 0 && up <= 0 {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.rollPeriods(now)

	period := l.periods[networkID]
	if period == "" {
		period = PeriodMonth
	}
	periodKey := PeriodKey(period, now)

	if l.usage[periodKey] == nil {
		l.usage[periodKey] = make(map[string]int64)
	}
	before := l.usage[periodKey][networkID]
	l.usage[periodKey][networkID] = before + down + up

	// Check data limit
	limit := l.dataLimits[networkID]
	if limit > 0 && before < limit && before+down+up >= limit {
		// Fire callback asynchronously to avoid deadlock
		if l.onLimitReached != nil {
			go l.onLimitReached(networkID)
		}
	}

	// Schedule periodic save
	l.scheduleSave()
}

// LimitReached returns true if the network has exceeded its data cap for the current period.
func (l *Limits) LimitReached(networkID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rollPeriods(time.Now())
	period := l.periods[networkID]
	if period == "" {
		period = PeriodMonth
	}
	periodKey := PeriodKey(period, time.Now())
	limit := l.dataLimits[networkID]
	if limit == 0 {
		return false
	}
	used := l.usage[periodKey][networkID]
	return used >= limit
}

// UsedByPeriod returns the current period's usage per network.
func (l *Limits) UsedByPeriod() map[string]int64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rollPeriods(time.Now())
	now := time.Now()
	result := make(map[string]int64)
	for periodKey, networks := range l.usage {
		for id, used := range networks {
			// Return usage for current period of each network.
			// If network has a configured period, use that; otherwise check all period types.
			if p, ok := l.periods[id]; ok {
				if PeriodKey(p, now) == periodKey {
					result[id] = used
				}
			} else {
				// No configured period - check if this key matches any current period
				if PeriodKey(PeriodDay, now) == periodKey ||
					PeriodKey(PeriodWeek, now) == periodKey ||
					PeriodKey(PeriodMonth, now) == periodKey {
					result[id] = used
				}
			}
		}
	}
	return result
}

// ToggleNetwork toggles the enabled/disabled state of a network.
func (l *Limits) ToggleNetwork(networkID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.offNetworks[networkID] = !l.offNetworks[networkID]
}

// ResetUsage resets the current period's usage for a specific network.
func (l *Limits) ResetUsage(networkID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rollPeriods(time.Now())
	period := l.periods[networkID]
	if period == "" {
		period = PeriodMonth
	}
	periodKey := PeriodKey(period, time.Now())
	previous := l.usage[periodKey][networkID]
	l.usage[periodKey][networkID] = 0

	// Persist immediately
	if err := l.saveLocked(); err != nil {
		l.usage[periodKey][networkID] = previous
		return err
	}
	return nil
}

// Save writes usage data to disk.
func (l *Limits) Save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.saveLocked()
}

func (l *Limits) saveLocked() error {
	if l.savePath == "" {
		return nil
	}
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(l.savePath), 0755); err != nil {
		return err
	}
	data := struct {
		Version int                         `json:"version"`
		Periods map[string]map[string]int64 `json:"periods"`
	}{
		Version: 2,
		Periods: l.usage,
	}
	tmpPath := l.savePath + ".tmp"
	if err := os.WriteFile(tmpPath, mustJSON(data), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, l.savePath)
}

func (l *Limits) loadUsage() {
	if l.savePath == "" {
		return
	}
	data, err := os.ReadFile(l.savePath)
	if err != nil {
		return
	}
	var saved struct {
		Version int                         `json:"version"`
		Periods map[string]map[string]int64 `json:"periods"`
		// Legacy v1 format
		Month string           `json:"month"`
		Bytes map[string]int64 `json:"bytes"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return
	}
	if saved.Version == 2 && saved.Periods != nil {
		l.usage = saved.Periods
	} else if saved.Month != "" && saved.Bytes != nil {
		// Migrate v1 (monthly only) to v2
		l.usage = map[string]map[string]int64{
			saved.Month + "-month": saved.Bytes,
		}
	}
}

func (l *Limits) scheduleSave() {
	if l.saveTimer != nil {
		l.saveTimer.Reset(l.saveInterval)
		return
	}
	l.saveTimer = time.AfterFunc(l.saveInterval, func() {
		l.mu.Lock()
		_ = l.saveLocked()
		l.mu.Unlock()
	})
}

func (l *Limits) rollPeriods(now time.Time) {
	changed := false
	for _, key := range map[DataLimitPeriod]string{
		PeriodDay:   PeriodKey(PeriodDay, now),
		PeriodWeek:  PeriodKey(PeriodWeek, now),
		PeriodMonth: PeriodKey(PeriodMonth, now),
	} {
		if l.usage[key] == nil {
			l.usage[key] = make(map[string]int64)
			changed = true
		}
	}
	// Clean up old period keys (keep only current ones)
	currentKeys := map[string]bool{
		PeriodKey(PeriodDay, now):   true,
		PeriodKey(PeriodWeek, now):  true,
		PeriodKey(PeriodMonth, now): true,
	}
	for key := range l.usage {
		if !currentKeys[key] {
			delete(l.usage, key)
			changed = true
		}
	}
	if changed {
		l.scheduleSave()
	}
}

// Close stops the periodic save timer and saves final state.
func (l *Limits) Close() error {
	l.cancel()
	if l.saveTimer != nil {
		l.saveTimer.Stop()
	}
	return l.Save()
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
