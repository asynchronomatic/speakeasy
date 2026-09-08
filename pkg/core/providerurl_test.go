package core

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseProviderURL(t *testing.T) {
	cases := []struct {
		raw          string
		allowPrivate bool
		want         error
	}{
		{raw: "https://api.example/v1", want: nil},
		{raw: "http://example.com", want: nil},
		{raw: "http://127.0.0.1:11434", allowPrivate: true, want: nil},
		{raw: "http://localhost:11434", allowPrivate: true, want: nil},
		{raw: "http://[::1]:11434", allowPrivate: true, want: nil},
		{raw: "http://10.0.0.5:11434", allowPrivate: true, want: nil},
		{raw: "", want: ErrProviderURLHost},
		{raw: "http://", want: ErrProviderURLHost},
		{raw: "file:///etc/passwd", want: ErrProviderURLScheme},
		{raw: "gopher://example.com", want: ErrProviderURLScheme},
		{raw: "http://user:pass@example.com", want: ErrProviderURLUserinfo},
		{raw: "http://127.0.0.1:11434", want: ErrProviderURLPrivate},
		{raw: "http://localhost:11434", want: ErrProviderURLPrivate},
		{raw: "http://localhost.localdomain", want: ErrProviderURLPrivate},
		{raw: "http://foo.localhost", want: ErrProviderURLPrivate},
		{raw: "http://10.0.0.1", want: ErrProviderURLPrivate},
		{raw: "http://192.168.1.1", want: ErrProviderURLPrivate},
		{raw: "http://[::1]", want: ErrProviderURLPrivate},
		{raw: "http://[fe80::1]", want: ErrProviderURLPrivate},
		{raw: "http://0.0.0.0", allowPrivate: true, want: ErrProviderURLPrivate},
		{raw: "http://169.254.169.254/", want: ErrProviderURLMetadata},
		{raw: "http://169.254.169.254/", allowPrivate: true, want: ErrProviderURLMetadata},
		{raw: "http://metadata.google.internal/", allowPrivate: true, want: ErrProviderURLMetadata},
		{raw: "http://METADATA.GOOG", allowPrivate: true, want: ErrProviderURLMetadata},
		{raw: "http://[fd00:ec2::254]/", allowPrivate: true, want: ErrProviderURLMetadata},
		{raw: "http://100.100.100.200/", allowPrivate: true, want: ErrProviderURLMetadata},
	}
	for _, tc := range cases {
		_, err := ParseProviderURL(tc.raw, tc.allowPrivate)
		if tc.want == nil {
			if err != nil {
				t.Errorf("ParseProviderURL(%q, %v)=%v want nil", tc.raw, tc.allowPrivate, err)
			}
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("ParseProviderURL(%q, %v)=%v want %v", tc.raw, tc.allowPrivate, err, tc.want)
		}
	}
}

type lookupFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

func (f lookupFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

func TestProviderDialBlocksResolvedMetadata(t *testing.T) {
	rt := newProviderTransport(true, lookupFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "evil.example" {
			t.Fatalf("host %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	}))
	_, err := rt.DialContext(context.Background(), "tcp", "evil.example:80")
	if !errors.Is(err, ErrProviderURLMetadata) {
		t.Fatalf("err=%v want metadata", err)
	}
}

func TestProviderDialBlocksResolvedPrivate(t *testing.T) {
	rt := newProviderTransport(false, lookupFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}))
	_, err := rt.DialContext(context.Background(), "tcp", "rebind.example:80")
	if !errors.Is(err, ErrProviderURLPrivate) {
		t.Fatalf("err=%v want private", err)
	}
}

func TestProviderDialBlocksMixedPrivateRecord(t *testing.T) {
	rt := newProviderTransport(false, lookupFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("8.8.8.8")},
			{IP: net.ParseIP("10.0.0.1")},
		}, nil
	}))
	_, err := rt.DialContext(context.Background(), "tcp", "mixed.example:80")
	if !errors.Is(err, ErrProviderURLPrivate) {
		t.Fatalf("err=%v want private", err)
	}
}

func TestProviderHTTPClientNoRedirect(t *testing.T) {
	hit := false
	dest := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	t.Cleanup(dest.Close)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusFound)
	}))
	t.Cleanup(src.Close)

	c := NewProviderHTTPClient(true)
	resp, err := c.Get(src.URL)
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if !errors.Is(err, ErrProviderRedirect) {
		t.Fatalf("err=%v want redirect", err)
	}
	if hit {
		t.Fatal("followed redirect")
	}
}

func TestPrivateBackendsAllowed(t *testing.T) {
	cfg := &Config{}
	if cfg.PrivateBackendsAllowed() {
		t.Fatal("default should be false")
	}
	cfg.Proxy.AllowPrivateBackends = true
	if !cfg.PrivateBackendsAllowed() {
		t.Fatal("config true")
	}
	t.Setenv("SPEAKEASY_ALLOW_PRIVATE_BACKENDS", "0")
	if cfg.PrivateBackendsAllowed() {
		t.Fatal("env should override to false")
	}
	t.Setenv("SPEAKEASY_ALLOW_PRIVATE_BACKENDS", "true")
	cfg.Proxy.AllowPrivateBackends = false
	if !cfg.PrivateBackendsAllowed() {
		t.Fatal("env should override to true")
	}
}
