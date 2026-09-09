package api

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

func parseInviteURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "https://"):
	case strings.HasPrefix(raw, "http://"):
	case strings.HasPrefix(raw, "https:/"):
		raw = "https://" + strings.TrimPrefix(raw, "https:/")
	case strings.HasPrefix(raw, "http:/"):
		raw = "http://" + strings.TrimPrefix(raw, "http:/")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invite url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" || u.Path == "" {
		return nil, fmt.Errorf("invalid invite url")
	}
	if u.User != nil {
		return nil, core.ErrProviderURLUserinfo
	}
	return u, nil
}

// RedeemInvite posts this node's identity to an invite URL and returns mesh
// join settings. The endpoint is public; the invite token in the URL is the
// credential.
func RedeemInvite(inviteURL string, node Node) (*RedeemInviteResponse, error) {
	if node.ID == "" {
		return nil, fmt.Errorf("node peer id is required")
	}

	u, err := parseInviteURL(inviteURL)
	if err != nil {
		return nil, err
	}

	base := u.Scheme + "://" + u.Host
	if _, err := core.ParseProviderURL(base, true); err != nil {
		return nil, err
	}

	path := u.EscapedPath()
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	var resp RedeemInviteResponse
	c := jsonrpc.NewClient(base, "").WithDoer(core.NewProviderHTTPClient(true))
	if err := c.Post(path, RedeemInviteRequest{Node: node}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
