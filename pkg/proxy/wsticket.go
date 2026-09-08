package proxy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

const (
	wsTicketCookie  = "speakeasy_ws"
	wsTicketTTL     = 30 * time.Second
	wsTicketByteLen = 32
)

func hashWSTicket(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (p *Proxy) pruneWSTicketsLocked(now time.Time) {
	for h, exp := range p.wsTickets {
		if now.After(exp) {
			delete(p.wsTickets, h)
		}
	}
}

func (p *Proxy) issueWSTicket() (string, error) {
	b := make([]byte, wsTicketByteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := hex.EncodeToString(b)
	now := time.Now()
	p.wsTicketMu.Lock()
	p.pruneWSTicketsLocked(now)
	p.wsTickets[hashWSTicket(raw)] = now.Add(wsTicketTTL)
	p.wsTicketMu.Unlock()
	return raw, nil
}

func (p *Proxy) consumeWSTicket(r *http.Request) bool {
	c, err := r.Cookie(wsTicketCookie)
	if err != nil || c.Value == "" {
		return false
	}
	h := hashWSTicket(c.Value)
	now := time.Now()
	p.wsTicketMu.Lock()
	defer p.wsTicketMu.Unlock()
	p.pruneWSTicketsLocked(now)
	exp, ok := p.wsTickets[h]
	if !ok {
		return false
	}
	delete(p.wsTickets, h)
	return !now.After(exp)
}

func (p *Proxy) refreshTicketHandler(rpc *jsonrpc.RPC) error {
	if p.auth != nil {
		raw, err := p.issueWSTicket()
		if err != nil {
			return err
		}

		rpc.SetCookie(&http.Cookie{
			Name:     wsTicketCookie,
			Value:    raw,
			Path:     "/api/v.1/refresh/websocket",
			MaxAge:   int(wsTicketTTL.Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
	}
	return rpc.ReplyObject(map[string]bool{"ok": true})
}
