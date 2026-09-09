package security

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	authRateMaxEnv       = "SPEAKEASY_AUTH_RATE_MAX"
	defaultAuthRateMax   = 20
	defaultAuthRateWindow = time.Minute
)

type failureLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	hits    map[string][]time.Time
}

var authFailures = &failureLimiter{
	window: defaultAuthRateWindow,
	hits:   make(map[string][]time.Time),
}

func authRateMax() int {
	s := strings.TrimSpace(os.Getenv(authRateMaxEnv))
	if s == "" {
		return defaultAuthRateMax
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultAuthRateMax
	}
	return n
}

func (l *failureLimiter) pruneLocked(key string, now time.Time) []time.Time {
	q := l.hits[key]
	i := 0
	for i < len(q) && now.Sub(q[i]) >= l.window {
		i++
	}
	if i > 0 {
		q = append([]time.Time(nil), q[i:]...)
	}
	if len(q) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = q
	return q
}

// AuthBlocked reports whether key has exceeded the failure budget.
// SPEAKEASY_AUTH_RATE_MAX<=0 disables the limiter (tests).
func AuthBlocked(key string) bool {
	max := authRateMax()
	if max <= 0 || key == "" {
		return false
	}
	now := time.Now()
	authFailures.mu.Lock()
	defer authFailures.mu.Unlock()
	q := authFailures.pruneLocked(key, now)
	return len(q) >= max
}

// AuthFailure records a failed auth attempt for key.
func AuthFailure(key string) {
	max := authRateMax()
	if max <= 0 || key == "" {
		return
	}
	now := time.Now()
	authFailures.mu.Lock()
	defer authFailures.mu.Unlock()
	q := authFailures.pruneLocked(key, now)
	authFailures.hits[key] = append(q, now)
}

func WriteAuthError(w http.ResponseWriter, code int) {
	if w == nil {
		return
	}
	if code == http.StatusTooManyRequests {
		http.Error(w, "too many requests", code)
		return
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
