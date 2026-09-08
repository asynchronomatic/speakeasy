package security

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

var DefaultAllowedHeaders = map[string]bool{
	"Content-Type": true,
	"X-Request-Id": true,
}

func ScrubHeaders(r *http.Request, allowedHeaders map[string]bool) {
	for k := range r.Header {
		// only allowed Headers are passed on
		if !allowedHeaders[k] {
			r.Header.Del(k)
		}
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// OriginOK reports whether r is same-origin. A missing Origin is allowed
// (CLI and tests). Unlike RequireSameOrigin this applies to GET as well,
// which is required for WebSocket upgrades.
func OriginOK(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return host != "" && strings.EqualFold(u.Host, host)
}

// RequireSameOrigin rejects unsafe methods that carry a browser Origin which
// does not match the request Host. Requests with no Origin (CLI, jsonclient)
// are allowed. X-Forwarded-Host is ignored so clients cannot spoof it.
func RequireSameOrigin(r *http.Request) error {
	if r == nil || isSafeMethod(r.Method) {
		return nil
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}
	if OriginOK(r) {
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return jsonrpc.NewError(http.StatusForbidden, "invalid origin")
	}
	return jsonrpc.NewError(http.StatusForbidden, "origin mismatch")
}

func RejectSameOrigin(w http.ResponseWriter, err error) {
	if ce, ok := err.(*jsonrpc.Error); ok {
		http.Error(w, ce.Message(), ce.Code())
		return
	}
	http.Error(w, err.Error(), http.StatusForbidden)
}
