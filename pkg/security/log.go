package security

import (
	"net/http"
	"os"
	"strings"
	"unicode"
)

const trustForwardedEnv = "SPEAKEASY_TRUST_FORWARDED"

// SanitizeLog removes ASCII/Unicode control characters so CR/LF cannot forge log lines.
func SanitizeLog(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\u007f' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func trustForwarded() bool {
	v, ok := os.LookupEnv(trustForwardedEnv)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func forwardedClient(r *http.Request) string {
	raw := SanitizeLog(r.Header.Get("X-Forwarded-For"))
	if raw == "" {
		return ""
	}
	if i := strings.IndexByte(raw, ','); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}

// ClientAddr is the address to log for r. X-Forwarded-For is ignored unless
// SPEAKEASY_TRUST_FORWARDED is set (1/true/yes/on).
func ClientAddr(r *http.Request) string {
	if r == nil {
		return ""
	}
	if trustForwarded() {
		if xff := forwardedClient(r); xff != "" {
			return xff
		}
	}
	return SanitizeLog(r.RemoteAddr)
}

// RequestPath is r.URL.Path with control characters stripped.
func RequestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return SanitizeLog(r.URL.Path)
}

// RequestMethod is r.Method with control characters stripped.
func RequestMethod(r *http.Request) string {
	if r == nil {
		return ""
	}
	return SanitizeLog(r.Method)
}
