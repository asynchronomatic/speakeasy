# Speakeasy security audit

**Original review:** 2026-09-07  
**Re-evaluated:** 2026-09-07 against the current working tree (including uncommitted changes in `pkg/admin`, `pkg/proxy`, `pkg/mesh`, `pkg/jsonclient`).  
**Scope:** Proxy UI, admin controller, mesh/libp2p, invite/join, provider config, static web UI.  
**Method:** Manual review vs. the previous `AUDIT.md`. No pentest, no dependency CVE scan, **no code changes** in this pass.

The intended model is unchanged: proxy `:4080` is a LAN front door for trusted apps; admin `:4002` is the control plane; libp2p peers are admitted via invites.

---

## Delta since the last audit

### Closed

| ID | Issue | What changed |
|----|--------|----------------|
| **C2** | Mesh stream handler served the full UI mux | `MeshServeHTTP` now uses `p.meshMux` with only `GET /.mesh/status\|members\|models`. Inference paths are still handled separately. Peers can no longer hit `/api/mesh/*`, `/api/admin/*`, or `/ui/`. |
| **C3** | `GET /api/mesh/config` dumped `config.yaml` | Handler and route removed. |
| **C4** (partial) | Mesh sessions could register / refresh / unregister **other** nodes | Register and unregister require `ctx.User() == id` unless `Group == admin`. Refresh requires `id == ctx.User()`. |
| **H2** (partial) | One-time invite TOCTOU | `adminRedeemInviteLink` now holds `s.lock` for the get/put/delete sequence (in-process race closed). |
| **L1** | Double `Unlock` in `adminEnableHandler` | Extra unlock removed. |
| **L2** | `jsonclient` had no timeout / leaked keep-alives | `NewClient` now uses a cloned transport with `DisableKeepAlives` and a 15s timeout. |
| **L4** | Public reachability advertised TCP/UDP **4001** | Advertises `mc.Port` instead. |
| **L5** | Discovery goroutine `Fatalf` on exit | Now `Eventf`. |
| Default debug | `pkg/log.Default` used `LogAll` in production | `cmd/mesh/main.go` sets `LogNormal` unless `config.Debug` is true. |

### New or worsened

| ID | Issue |
|----|--------|
| **C6** | Token redaction is applied in `loadProvidersConfig()`, which is also used by add/update/delete. Saving then writes `token: "*"` over every existing provider secret. |

### Still open (narrowed)

**C4 remainder:** `POST /api/v1/authorize` is still `authenticated` only (any mesh session can `acl.Add` an arbitrary peer ID).

---

## Executive summary

The mesh-vs-UI mux split and the config-dump removal are real improvements. The proxy is still an unauthenticated LAN (and possibly WAN) control plane. Provider-token hiding was implemented on the **load** path, which will **wipe live API keys** on the next provider add/edit/delete.

| Severity | Count | Headline |
|----------|------:|----------|
| Critical | 4 | Unauthenticated proxy APIs; `authorize` still mesh-callable; plaintext secrets on disk; provider save path clobbers tokens |
| High | 6 | Never-expiring sessions; admin bearer in a map; unauthenticated debug/theme/provider mutation; CSRF via `text/plain`; reverse-proxy SSRF; websocket `CheckOrigin: true` |
| Medium | 8 | Timing oracles, token logging, no rate limits, no TLS, no security headers, unbounded bodies / infinite peer retries, gatekeeper accept-all, spoofable `X-Mesh` |
| Low / info | 3 | Cwd-relative allow-list, mesh ID forced to `default`, redeem holds global lock across bcrypt |

**Do this first**

1. Authenticate the proxy HTTP API (or bind it to loopback). Treat inference as a separate listener if local apps must stay keyless.
2. Wrap `POST /api/v1/authorize` in `asAdmin`.
3. Redact tokens only in the **list** response. On update, treat `token` empty or `"*"` as “keep existing.”
4. Rotate any secret that lived in a working-tree `config.yaml`.
5. Issue mesh sessions with a TTL and a revocation path.

---

## Architecture (trust boundaries)

```
[Chat app / browser] --HTTP :4080--> [Proxy] --libp2p--> [Peer proxies]
                         |                    \
                         |                     \-> [Admin :4002]  (invites, ACL, node directory)
                         v
                   config.yaml, node.key, provider tokens
```

- **Proxy (`pkg/proxy`)** — OpenAI-compatible front door, settings UI, provider CRUD, optional admin UI. `ListenAndServe`, no TLS, **no caller authentication**. Local mux and mesh mux are now separate.
- **Admin (`pkg/admin`)** — Invite create/redeem, node login, allow-list, relay addresses. Bearer admin secret *or* `mesh-` session token.
- **Mesh (`pkg/mesh`)** — libp2p host, circuit relay, MDNS, HTTP/1.1 over `/ollama/0.0.1`. Gater checks the allow-list after Noise; `InterceptAccept` is still open.

---

## Critical

### C1. Proxy HTTP API has no authentication — **open**

**Where:** `pkg/proxy/proxy.go` (`NewProxy` local mux), `ServeHTTP`  
**What:** Anyone who can open TCP to `proxy.listen` (default `:4080`, all interfaces) can call:

- `GET/POST /api/mesh/providers` and `POST/DELETE /api/mesh/providers/{id}`
- `GET/POST /api/admin/*` — `withAdmin` only checks that **this process** has an admin client, not who the HTTP caller is
- `GET/POST /api/mesh/debug` and `/api/mesh/theme`
- `GET /api/mesh/members`, `/api/mesh/models`, `/v1/models`
- `/v1/chat/completions` (and other inference paths)
- `/api/v.1/refresh/websocket`

The UI “admin token” form only configures the **outbound** admin client.

**Impact:** On a LAN or a forwarded port: drain paid inference, rewrite `config.yaml`, kick members, mint invites. Combined with C6, a single unauthenticated POST can also destroy stored provider tokens.

**Fix:** Bind to `127.0.0.1` by default; add a proxy-local credential (or mTLS) for UI/API; keep inference on a separate listener if product requires an open OpenAI port.

---

### C2. Mesh stream handler served the full proxy mux — **closed**

`meshMux` is limited to `/.mesh/status|members|models`. Unknown mesh paths 404. Inference is still accepted on the documented OpenAI/Anthropic paths with `isFromMesh=true` (private providers skipped via `GetLocalRouteProtected`).

Residual: a peer can still call `/.mesh/members` on another node and trigger that node to dial the rest of the mesh (enumeration / modest amplification). Acceptable for a mesh directory; do not put secrets on those handlers.

---

### C3. `GET /api/mesh/config` dumped `config.yaml` — **closed**

Route and `uiConfigHandler` are gone.

---

### C4. Mesh sessions can still expand the allow-list — **narrowed, still critical**

**Where:** `pkg/admin/server.go` `POST /api/v1/authorize`; `pkg/admin/api_node.go` `apiNodeAuthorize`

| Route | Auth now | Status |
|-------|----------|--------|
| `POST /api/v1/authorize` | `authenticated` only; **no** `id == User()` check | **Open** — any `mesh-` session can `acl.Add` any peer ID |
| `POST /api/v1/nodes` | Caller must be that node, or admin | Addressed |
| `DELETE /api/v1/nodes/{id}` | Self or admin | Addressed (self-leave still `acl.Remove`s that id, which is reasonable) |
| `POST /api/v1/nodes/{id}` | `id == ctx.User()` | Addressed |
| `GET /api/v1/nodes`, `GET /api/v1/relay` | Any authenticated mesh/admin | Directory/relay info — expected for members |

**Impact:** One invited or stolen node login can admit arbitrary libp2p peer IDs. The gatekeeper then allows those peers. This is the remaining privilege-escalation path from C4.

**Fix:** `asAdmin` on authorize. Do not let mesh sessions persist ACL additions.

---

### C5. Long-term secrets still plaintext on disk — **open** (API list redacted)

**Where:** `pkg/core/config.go` `SaveConfig`; working-tree `config.yaml`  
**What:** `admin.secret`, `mesh.secret`, and `providers[].token` remain plaintext in `config.yaml` (`0600` on write). `.gitignore` lists `config.yaml` / keys; that does not help a file that was committed earlier, backups, or this working tree.

`GET /api/mesh/providers` no longer returns real tokens (they are replaced with `"*"`). That part of the old C5 is addressed, but see **C6**.

**Impact:** Disk theft and VCS history still yield usable credentials. Rotate anything that lived in a local `config.yaml`.

**Fix:** OS keychain or encrypted secret file. Confirm git history is clean.

---

### C6. Provider add/update/delete persist redacted tokens — **new**

**Where:** `pkg/proxy/handlers_providers.go` `loadProvidersConfig`  
**What:** Every load does:

```go
for i := range cfg.Providers {
    cfg.Providers[i].Token = "*"
}
```

`providerAddHandler`, `providerUpdateHandler`, and `providerDeleteHandler` all load via that helper, then `SaveConfig`. Existing provider tokens on disk become `*`.

The settings UI fills the token field from the list payload, so an Edit → Save without re-typing the key also writes `"*"`.

Add/update still **echo** the request token in the JSON response (the client-supplied secret, not other providers’ secrets).

**Impact:** The next provider change on a node that already has API keys (e.g. xAI) **destroys those keys**. Unauthenticated callers (C1) can trigger this.

**Fix:** Redact only in the list handler (copy, don’t mutate the save candidate). On update, if `token` is empty or `"*"`, keep the stored value.

---

## High

### H1. Mesh session tokens never expire — **open**

**Where:** `pkg/admin/api_node.go` `apiNodeLogin` still calls `issueSessionToken(req.NodeID, 0)`. Tests still assert `Expires == 0`.  
AES-256-GCM key is process-lifetime (`magiclink.GenerateKey` in `NewServer`). No denylist.

**Impact:** Stolen `mesh-` token works until admin restart. Restart is the only revocation (and it drops everyone).

**Fix:** Use `SessionTokenTTL` (already 10 minutes). Refresh via re-login. Server-side session table or a persisted key plus revocation list.

---

### H2. One-time invite race — **mostly closed**

In-process concurrent redeems are serialized by `s.lock`. Remaining:

- Node ID in the body is still caller-chosen (not a signed libp2p key).
- Invite URL remains a bearer capability (phishing / leaked screenshot).
- The lock is held across bcrypt (~100ms), blocking other admin operations (availability, not a bypass).

**Fix:** Compare-and-swap in KV if you ever run multiple admin processes. Bind redeem to a peer public key + signature. Rate-limit by IP.

---

### H3. Admin bearer stored and compared as a map key — **open**

**Where:** `pkg/admin/auth/token.go` `AddUser` / `DoAuth`  
Still `a.users[password]`. No hash, no constant-time compare. `DeleteUser` is still a no-op.

**Fix:** bcrypt/argon2 of the admin secret. `subtle.ConstantTimeCompare` (or bcrypt’s compare). Implement delete.

---

### H4. Unauthenticated debug and theme mutation — **open**

**Where:** `POST /api/mesh/debug`, `POST /api/mesh/theme`  
Still no caller auth. CLI default log level is now `LogNormal` unless `proxy.debug` (improvement). `pkg/log.Default` still starts at `LogAll` until `main` runs.

**Fix:** Same auth as the rest of settings. Keep production default at `LogNormal`.

---

### H5. CSRF against JSON APIs — **open**

`json.Decoder` does not require `Content-Type: application/json`. Browser “simple” `text/plain` POSTs skip CORS preflight and can still mutate providers, debug, theme, admin enable, invites, kicks.

**Fix:** Require JSON content-type. Authenticate (C1). Optional Origin/Host checks.

---

### H6. Reverse-proxy SSRF via provider `base_url` — **open**

**Where:** `validateProvider` only checks non-empty; `httputil.NewSingleHostReverseProxy`  
No scheme/host allow-list. Incoming `Authorization` is forwarded when the provider has no token.

**Fix:** Allow `http`/`https` only; block link-local / loopback / metadata unless opted in. Strip incoming `Authorization` unless configured to forward. Disable backend redirects.

---

### H7. WebSocket accepts any Origin — **open**

**Where:** `pkg/proxy/socket/notifier.go` `CheckOrigin: return true`  
Upgrade is unauthenticated. Payload is only `wakeup`, so impact is load + waking a local UI.

**Fix:** Same-origin (or an allow-list). Authenticate the upgrade.

---

## Medium

### M1. Login timing oracle and no rate limits — **open**

Unknown node ID returns 401 before bcrypt; bad password runs bcrypt. `POST /api/admin/enabled` is still an admin-secret oracle (400 / 412 / 200). No throttling on login, redeem, or enable.

**Fix:** Dummy bcrypt on missing records. Uniform errors. Rate-limit per IP and per id.

---

### M2. Secrets in logs — **open**

`pkg/admin/api_node.go` still logs `token mismatch %s %s` (expected and provided registration tokens). Discovery still dumps libp2p events at Event level.

**Fix:** Never log tokens. Redact `Authorization`.

---

### M3. No TLS on proxy or admin HTTP — **open**

`ListenAndServe` only. Empty `TLSConfig` on the proxy is unused. `jsonclient` now has a 15s timeout (L2 closed) but still depends on the system CA bundle and whatever URL is in config (`http://` is allowed).

**Fix:** TLS or a documented terminator. Disallow `http://` admin URLs in production.

---

### M4. Non-constant-time refresh token compare; weak `rand` fallback — **open**

`ref.Token != req.Token`. If `rand.Read` fails, `newNodeToken` is `hex(time.RFC3339Nano)`.

**Fix:** `subtle.ConstantTimeCompare`. Fail closed on `rand` errors.

---

### M5. Slowloris / unbounded JSON / infinite peer retry — **open**

Proxy read/write timeouts are 600s; no `MaxHeaderBytes`. Admin JSON has no `MaxBytesReader`. `OnPeerUpdate` still `retry.WithMaxRetries(0, …)` on `context.Background()`.

**Fix:** Tighter header timeouts, 1 MiB body cap, bounded retries with a deadline.

---

### M6. No browser security headers — **open**

No CSP, `frame-ancestors`/`X-Frame-Options`, `nosniff`, or `Referrer-Policy`. Admin UI is frameable (clickjacking + H5). Chat markdown still HTML-escapes first; table cells generally use `escapeHTML`.

**Fix:** `Content-Security-Policy: default-src 'self'`, `frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`.

---

### M7. Gatekeeper accepts all pre-handshake connections — **open**

`InterceptAccept` / `InterceptAddrDial` return true; ACL is applied in `InterceptSecured`. MDNS still optional via config.

**Fix:** Connection-manager limits; MDNS off by default on untrusted LANs.

---

### M8. `X-Mesh` is a spoofable client header — **open**

`streamHandler` still sets `X-Mesh: true` on the parsed request. Local clients can send the same header (`IsSource` / `GetLocalRouteProtected` then skip private providers). Mesh origin should be a Go context value from `MeshServeHTTP` only — that function already knows `isFromMesh`.

**Fix:** Stop trusting the header for access control.

---

## Low / informational

### L1. Double unlock — **closed**

### L2. jsonclient timeout / keep-alives — **closed**

### L3. Allow-list path is cwd-relative — **open**

`NewAllowList("allow.list")` still depends on process cwd.

### L4. Advertised public port 4001 — **closed** (uses `mc.Port`; `0` still advertises `/tcp/0`, which is odd but not the old wrong port)

### L5. Discovery `Fatalf` — **closed**

### L6. Invite mesh ID forced to `"default"` — **open**

Isolation between meshes is still not real.

---

## What is in reasonably good shape

- Mesh HTTP is no longer a backdoor into the UI mux (**C2**).
- Config dump endpoint is gone (**C3**).
- Node register / refresh / unregister are bound to the session user (or admin) (**C4** partial).
- Invite public IDs are SHA-256 of a UUID.
- Node login secrets are bcrypt (`DefaultCost`).
- Session blobs are AES-256-GCM with a random nonce.
- `SaveConfig` / `node.key` / `allow.list` use mode `0600`.
- `LoadOrCreateKey` uses Ed25519.
- Inference body read is capped around 1 MiB (still not explicitly rejected when over the cap).
- Private providers are skipped for mesh-origin inference (`GetLocalRouteProtected`).
- Unexported `ModelRoute.providers` (tokens) are not JSON-encoded on `/.mesh/models`.
- UI generally escapes HTML for names and IDs.
- `/api/v1/admin/*` remains wrapped in `asAdmin`.
- Production CLI log level is `LogNormal` unless `debug` is set.

---

## Suggested order of work

1. **C6** — stop persisting redacted tokens (this will brick provider keys as soon as someone uses Settings).  
2. **C4** — `authorize` is admin-only.  
3. **C1** — authenticate or loopback-bind the proxy.  
4. **C5** — rotate working-tree secrets; keep tokens off the wire.  
5. **H1** — session TTL + revocation.  
6. **H3 / M4** — hash admin secret; constant-time compares.  
7. **H6** — provider URL allow-list.  
8. **H5 / H7 / M6** — content-type, Origin, CSP.  
9. **M1 / M3 / M5** — rate limits, TLS, body/timeout caps.

---

## Out of scope / not verified

- Dependency CVEs (`go.mod` / libp2p / gorilla / badger).  
- Whether `config.yaml` or keys exist in **git history** on the remote.  
- Production TLS terminator configuration.  
- Load testing of hole punching and relay ACL.

This document is a review of the current tree, not a guarantee that every defect was found.
)
