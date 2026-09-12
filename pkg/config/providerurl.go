package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	ErrProviderURLScheme   = errors.New("provider base_url must be http or https")
	ErrProviderURLUserinfo = errors.New("provider base_url must not include userinfo")
	ErrProviderURLHost     = errors.New("provider base_url host is required")
	ErrProviderURLMetadata = errors.New("provider base_url points at a cloud metadata endpoint")
	ErrProviderURLPrivate  = errors.New("provider base_url points at a private or loopback address")
	ErrProviderRedirect    = errors.New("provider redirects are disabled")
)

var metadataHosts = map[string]struct{}{
	"metadata.google.internal": {},
	"metadata.goog":            {},
}

var metadataIPs = []net.IP{
	net.ParseIP("169.254.169.254"),
	net.ParseIP("fd00:ec2::254"),
	net.ParseIP("100.100.100.200"),
}

type ipResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type providerDialer struct {
	allowPrivate bool
	resolver     ipResolver
	dialer       *net.Dialer
}

func envBool(name string) (bool, bool) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return false, false
	}
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, true
	default:
		return false, true
	}
}

// PrivateBackendsAllowed reports whether provider URLs may target loopback,
// RFC1918, ULA, or link-local addresses. SPEAKEASY_ALLOW_PRIVATE_BACKENDS
// overrides the config field when set.
func (c *Config) PrivateBackendsAllowed() bool {
	if v, ok := envBool("SPEAKEASY_ALLOW_PRIVATE_BACKENDS"); ok {
		return v
	}
	return c.Proxy.AllowPrivateBackends
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func isMetadataHost(host string) bool {
	_, ok := metadataHosts[normalizeHost(host)]
	return ok
}

func isMetadataIP(ip net.IP) bool {
	for _, meta := range metadataIPs {
		if meta != nil && meta.Equal(ip) {
			return true
		}
	}
	return false
}

func isLoopbackHost(host string) bool {
	h := normalizeHost(host)
	return h == "localhost" || h == "localhost.localdomain" || strings.HasSuffix(h, ".localhost")
}

// CheckProviderIP rejects metadata always, and private/loopback/link-local
// addresses unless allowPrivate is set.
func CheckProviderIP(ip net.IP, allowPrivate bool) error {
	if ip == nil || ip.To16() == nil {
		return ErrProviderURLHost
	}
	if isMetadataIP(ip) {
		return ErrProviderURLMetadata
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return ErrProviderURLPrivate
	}
	if allowPrivate {
		return nil
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return ErrProviderURLPrivate
	}
	return nil
}

// ParseProviderURL checks scheme, userinfo, host, metadata, and literal IPs.
// Hostnames that are not IPs are allowed through and re-checked at dial time.
func ParseProviderURL(raw string, allowPrivate bool) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrProviderURLHost
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderURLHost, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrProviderURLScheme
	}
	if u.User != nil {
		return nil, ErrProviderURLUserinfo
	}
	host := u.Hostname()
	if host == "" {
		return nil, ErrProviderURLHost
	}
	if isMetadataHost(host) {
		return nil, ErrProviderURLMetadata
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := CheckProviderIP(ip, allowPrivate); err != nil {
			return nil, err
		}
		return u, nil
	}
	if isLoopbackHost(host) && !allowPrivate {
		return nil, ErrProviderURLPrivate
	}
	return u, nil
}

func DenyProviderRedirect(*http.Request, []*http.Request) error {
	return ErrProviderRedirect
}

func (d *providerDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if isMetadataHost(host) {
		return nil, ErrProviderURLMetadata
	}

	var ips []net.IPAddr
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IPAddr{{IP: ip}}
	} else {
		resolver := d.resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		ips, err = resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("provider host %q resolved to no addresses", host)
	}
	for _, ip := range ips {
		if err := CheckProviderIP(ip.IP, d.allowPrivate); err != nil {
			return nil, err
		}
	}

	dialer := d.dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 30 * time.Second}
	}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

func newProviderTransport(allowPrivate bool, resolver ipResolver) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&providerDialer{
		allowPrivate: allowPrivate,
		resolver:     resolver,
		dialer:       &net.Dialer{Timeout: 30 * time.Second},
	}).DialContext
	return t
}

// NewProviderTransport returns a transport that refuses metadata and, unless
// allowPrivate is set, private/loopback/link-local destinations. It dials the
// first resolved address so DNS cannot rebind after the check.
func NewProviderTransport(allowPrivate bool) *http.Transport {
	return newProviderTransport(allowPrivate, net.DefaultResolver)
}

// NewProviderHTTPClient is for provider discovery. Redirects are disabled.
func NewProviderHTTPClient(allowPrivate bool) *http.Client {
	return NewProviderHTTPClientTransport(NewProviderTransport(allowPrivate))
}

// NewProviderHTTPClientTransport wraps an existing transport with a timeout
// and disabled redirects.
func NewProviderHTTPClientTransport(rt http.RoundTripper) *http.Client {
	return &http.Client{
		Timeout:       15 * time.Second,
		Transport:     rt,
		CheckRedirect: DenyProviderRedirect,
	}
}
