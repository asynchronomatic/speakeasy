package security

import "net/http"

// ContentSecurityPolicy is applied to the dashboard and JSON APIs.
const ContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

const permissionsPolicy = "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()"

// SetHeaders writes browser isolation headers. Safe on JSON APIs as well as /ui/.
func SetHeaders(w http.ResponseWriter) {
	if w == nil {
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", ContentSecurityPolicy)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Permissions-Policy", permissionsPolicy)
}

// Handler wraps next so every response gets SetHeaders.
func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetHeaders(w)
		next.ServeHTTP(w, r)
	})
}
