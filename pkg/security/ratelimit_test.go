package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthRateLimit(t *testing.T) {
	t.Setenv(authRateMaxEnv, "2")
	key := t.Name()
	if AuthBlocked(key) {
		t.Fatal("blocked before failures")
	}
	AuthFailure(key)
	AuthFailure(key)
	if !AuthBlocked(key) {
		t.Fatal("expected block after 2 failures")
	}
}

func TestAuthRateLimitDisabled(t *testing.T) {
	t.Setenv(authRateMaxEnv, "0")
	key := t.Name()
	for i := 0; i < 50; i++ {
		AuthFailure(key)
	}
	if AuthBlocked(key) {
		t.Fatal("disabled limiter should not block")
	}
}

func TestWriteAuthError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAuthError(rec, http.StatusTooManyRequests)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("code %d", rec.Code)
	}
}
