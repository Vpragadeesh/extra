package netlimit

import (
	"time"
)

// Limiter provides bandwidth rate limiting
type Limiter struct {
	maxBytesPerSec int64
	lastTime       time.Time
	bytesSent      int64
}

// NewLimiter creates a new bandwidth limiter
func NewLimiter(maxBytesPerSec int64) *Limiter {
	return &Limiter{
		maxBytesPerSec: maxBytesPerSec,
		lastTime:       time.Now(),
	}
}

// Wait blocks if necessary to enforce the rate limit
func (l *Limiter) Wait(bytes int64) {
	if l.maxBytesPerSec <= 0 {
		return // No limit
	}

	l.bytesSent += bytes
	now := time.Now()
	elapsed := now.Sub(l.lastTime).Seconds()

	if elapsed >= 1.0 {
		l.bytesSent = 0
		l.lastTime = now
		return
	}

	allowedBytes := int64(float64(l.maxBytesPerSec) * elapsed)
	if l.bytesSent > allowedBytes {
		sleepTime := time.Duration(float64(time.Second) * float64(l.bytesSent-allowedBytes) / float64(l.maxBytesPerSec))
		time.Sleep(sleepTime)
	}
}
