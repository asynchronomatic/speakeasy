package api

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// IsJSONContentType reports whether ct is application/json, ignoring parameters
// such as charset.
func IsJSONContentType(ct string) bool {
	media, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return strings.EqualFold(media, "application/json")
}

// RequireJSONContentType rejects requests whose Content-Type is not JSON.
// Browser "simple" POSTs (text/plain, form-urlencoded) cannot satisfy this,
// so they never skip CORS preflight into JSON mutating APIs.
func RequireJSONContentType(r *http.Request) error {
	if !IsJSONContentType(r.Header.Get("Content-Type")) {
		return NewError(http.StatusUnsupportedMediaType, "content-type must be application/json")
	}
	return nil
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
		return NewError(http.StatusForbidden, "invalid origin")
	}
	return NewError(http.StatusForbidden, "origin mismatch")
}

func RejectSameOrigin(w http.ResponseWriter, err error) {
	if ce, ok := err.(*Error); ok {
		http.Error(w, ce.Message(), ce.Code())
		return
	}
	http.Error(w, err.Error(), http.StatusForbidden)
}
