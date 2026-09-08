package security

import (
	"net/http"
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
