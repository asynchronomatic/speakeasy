# Speakeasy security audit

**Original review:** 2026-09-07  
**Re-evaluated:** 2026-09-08 against `main` at `8e4e053` (password UI, origin/CSRF guards, provider token handling, hashed admin tokens).  
**Scope:** Proxy UI, admin controller, mesh/libp2p, invite/join, provider config, static web UI.  
**Method:** Manual review vs. the previous `AUDIT.md`. No pentest, no dependency CVE scan, **no code changes** in this pass.

Intended model: proxy is a local front door (default `127.0.0.1:4080`); admin `:4002` is the control plane; libp2p peers join via invites.

---

## Closed since last review

| ID | What changed |
|----|----------------|
| **C2** | Mesh HTTP uses `meshMux` (`GET /.mesh/status`, `GET /.mesh/models`) only. |
| **C3** | `GET /api/mesh/config` removed. |
| **C4** | Register / refresh / unregister bound to session user (or admin). `authorize` may only add **the caller’s** peer ID (or admin). |
| **C6** | Tokens are stripped only on list/add/update **responses**. Updates keep the stored token when the body sends `""` or `"*"`. |
| **H2** | Redeem holds `s.lock` for get/put/delete (in-process one-time race closed). |
| **H3** | Admin (and proxy) bearer secrets are SHA-256 hashed; lookup is by hash, not the plaintext password. |
| **H4** | Debug and theme routes go through `authenticated()` (enforced when `proxy.password` is set). |
| **H5** | `GetObject` requires `Content-Type: application/json`. Unsafe methods with a browser `Origin` must match `Host`. |
| **H7** | WebSocket `CheckOrigin` is same-origin; `Sec-Fetch-Site: cross-site` is rejected; upgrade requires the proxy password when configured. |
| **L1** | Double `Unlock` in `adminEnableHandler` removed. |
| **L2** | `jsonclient` uses a 15s timeout and `DisableKeepAlives`. |
| **L4** | Public addrs use `mc.Port`. |
| **L5** | Discovery exit logs `Eventf`, not `Fatalf`. |
| **M8** | `X-Mesh` / `IsSource` are gone. Mesh vs local is the `MeshServeHTTP` / `isFromMesh` flag. |
| Default debug | `pkg/log.Default` is `LogNormal`; CLI sets `LogAll` only if `debug: true`. |

---

## Executive summary

Hardening landed: mux split, no config dump, CSRF content-type/origin, hashed bearers, provider-token redaction that does not wipe disk, optional proxy password + UI login, same-origin websocket.

What still matters:

- **Inference (`/v1/chat/completions` and siblings) never consults `p.auth`.** A password-protected UI does not protect the OpenAI port.
- **`proxy.password` is optional.** If unset, UI/admin APIs on the listen address are still open.
- **Mesh sessions never expire and survive kick.** A kicked node can `authorize` itself back onto the allow-list with the old `mesh-` token.
- Provider `base_url` is still an SSRF primitive. Secrets remain plaintext in `config.yaml`.

| Severity | Count | Headline |
|----------|------:|----------|
| Critical | 2 | Inference (and optional-password UI APIs) unauthenticated; kicked node can re-ACL itself via immortal session |
| High | 3 | Never-expiring mesh sessions; reverse-proxy SSRF; plaintext secrets on disk |
| Medium | 6 | Timing/rate limits; TLS; refresh-token compare; slowloris/unbounded JSON/infinite retry; no security headers; gatekeeper accept-all |
| Low / info | 4 | Cwd allow-list; mesh id `"default"`; WS `access_token` query; SHA-256 not bcrypt |

**Do this first**

1. Apply `p.auth` to local inference (`proxyHandleURLS` + `GET /v1/models`) or put inference on a separate loopback-only listener.  
2. On kick/delete, invalidate that node’s sessions (or check ACL/KV in `authenticateSessionToken`). Make `authorize` admin-only if self-rejoin is not desired.  
3. Give mesh sessions a TTL (`SessionTokenTTL` already exists).  
4. Allow-list provider `base_url` schemes/hosts.  
5. Rotate any secret that lived in a working-tree `config.yaml`.

---

## Architecture

```
[Chat app / browser] --HTTP 127.0.0.1:4080--> [Proxy] --libp2p--> [Peer proxies]
                         |                      \
                         |                       \-> [Admin :4002]
                         v
                   config.yaml (proxy.password, admin.secret, provider tokens)
```

- **Proxy:** OpenAI front door + `/ui`. `authenticated()` wraps mesh/admin JSON when `WithAuthToken` ran. Inference is dispatched in `ServeHTTP` **before** the mux.  
- **Admin:** Invites, node login, allow-list. Bearer is hashed SHA-256 or a `mesh-` AES-GCM session.  
- **Mesh:** libp2p; gater ACL after Noise; `InterceptAccept` still open.

---

## Critical

### C1. Local inference (and password-optional UI APIs) skip proxy auth

**Where:** `pkg/proxy/proxy.go` `ServeHTTP`; `cmd/mesh/app_proxy.go` `WithAuthToken` only if `config.Proxy.Password != ""`

When a password **is** set, `/api/mesh/*` and `/api/admin/*` require `Authorization: Bearer`. The UI prompts and stores it in `sessionStorage`.

Still unauthenticated even with a password:

- `/v1/chat/completions`, `/v1/responses`, `/v1/embeddings`, `/v1/messages` (handled before the mux)
- `GET /v1/models`
- `GET /.mesh/status`, `GET /.mesh/models` on the **local** mux
- Static `/ui/` (intentional so login can load)
- `GET /api/mesh/auth` (required-flag only)

When a password is **not** set, `authenticated()` is a no-op: all JSON APIs are open on the listen address. Join now prompts for `proxy.password`, so new nodes should have it; old configs may not. Default listen is `127.0.0.1:4080` (loopback), which reduces LAN exposure if operators keep the default.

**Impact:** Anyone who can reach the inference paths can spend provider quota and read prompts/responses. Binding `0.0.0.0:4080` without a password restores the old “open control plane” issue for UI APIs.

**Fix:** Honor `p.auth` on `proxyHandleURLS` and `GET /v1/models` (OpenAI clients would use the proxy password as the API key). Keep inference on loopback if it must stay keyless. Fail closed if listen is non-loopback and password is empty.

---

### C4. Kicked node can re-admit itself — **residual**

`POST /api/v1/authorize` is no longer “any mesh session, any peer ID.” It requires `req.Node.ID == ctx.User()` or admin.

`authenticateSessionToken` still does **not** check the allow-list or KV. Login issues a `mesh-` token with `Expires == 0`. Kick/delete removes ACL + KV + the live map but not the session.

A kicked node can `POST /api/v1/authorize` with its own ID and get back on the allow-list until the admin process restarts.

**Fix:** Reject sessions for IDs not on the ACL (or not in KV). Revoke on kick. Prefer `asAdmin` for authorize if nodes should not re-add themselves.

---

## High

### H1. Mesh session tokens never expire

**Where:** `pkg/admin/api_node.go` `apiNodeLogin` → `issueSessionToken(req.NodeID, 0)`  
AES-256-GCM key is process-lifetime. No denylist. Tests still expect `Expires == 0`.

**Impact:** Stolen `mesh-` token works until admin restart. Combined with C4 residual, kick is not sticky.

**Fix:** Use `SessionTokenTTL` (10 minutes). Re-login with the bcrypt mesh secret. Persist revocation or bind the session to ACL membership.

---

### H6. Reverse-proxy SSRF via provider `base_url`

**Where:** `validateProvider` (non-empty only); `httputil.NewSingleHostReverseProxy`  
No scheme/host allow-list. If the provider has no token, the client `Authorization` is forwarded to the backend.

Adding a provider still requires proxy auth **when a password is set**; without a password it does not.

**Fix:** Allow `http`/`https` only; block link-local / loopback / metadata unless opted in. Strip incoming `Authorization` unless configured to forward. Disable backend redirects.

---

### C5. Long-term secrets still plaintext on disk

**Where:** `config.yaml` via `SaveConfig` (`0600`)  
`admin.secret`, `mesh.secret`, `proxy.password`, `providers[].token` are plaintext. List/add/update APIs no longer return provider tokens. `.gitignore` covers `config.yaml`; history/backups may not.

**Fix:** OS keychain or encrypted file. Rotate anything that lived in a working-tree config.

---

## Medium

### M1. Login timing oracle and no rate limits

Unknown node ID returns 401 before bcrypt; bad password runs bcrypt. `POST /api/admin/enabled` still distinguishes missing / invalid / valid admin tokens. Proxy login is a 401 on `/api/mesh/models` with no throttling.

**Fix:** Dummy bcrypt on missing records. Uniform errors. Rate-limit per IP and id.

---

### M2. Verbose event logs

Registration-token mismatches no longer log the secret (only the node id). Discovery still dumps libp2p events (`%T: %+v`). Websocket `access_token` query will show up in proxy request logs (`r.RequestURI`).

**Fix:** Never log query tokens. Redact `Authorization` and URLs.

---

### M3. No TLS on proxy or admin HTTP

`ListenAndServe` only. `jsonclient` allows `http://` admin URLs.

**Fix:** TLS or a documented terminator. Prefer `https://` admin addresses.

---

### M4. Non-constant-time node refresh token; weak `rand` fallback

`ref.Token != req.Token`. If `rand.Read` fails, `newNodeToken` is `hex(time.RFC3339Nano)`.

Admin/proxy **login** passwords are SHA-256 then map lookup (not `subtle.ConstantTimeCompare` on the hash). Fine for high-entropy secrets; weaker than bcrypt for human passwords.

**Fix:** `subtle.ConstantTimeCompare` on registration tokens. Fail closed on `rand` errors. Consider bcrypt/argon2 for `proxy.password`.

---

### M5. Slowloris / unbounded JSON / infinite peer retry

Proxy read/write timeouts 600s; no `MaxHeaderBytes`. Admin JSON has no `MaxBytesReader`. `OnPeerUpdate` uses `retry.WithMaxRetries(0, …)` on `context.Background()`.

**Fix:** Tighter header timeouts, 1 MiB body cap, bounded retries with a deadline.

---

### M6. No browser security headers

No CSP, `frame-ancestors`, `nosniff`. Login overlay and admin UI are frameable. Chat markdown still escapes first.

**Fix:** `Content-Security-Policy: default-src 'self'`, `frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`.

---

### M7. Gatekeeper accepts all pre-handshake connections

`InterceptAccept` / `InterceptAddrDial` return true; ACL is in `InterceptSecured`. MDNS still config-gated.

**Fix:** Connection-manager limits; MDNS off by default on untrusted LANs.

---

## Low / informational

### L3. Allow-list path is cwd-relative

`NewAllowList("allow.list")` still depends on process cwd.

### L6. Invite mesh ID forced to `"default"`

`adminCreateInviteLink` still overwrites `MeshId`. Isolation between meshes is not real.

### N1. Websocket password in the query string

UI connects with `?access_token=`. That lands in logs, Referer, and process lists. Browsers cannot set `Authorization` on `WebSocket()`.

**Mitigation:** short-lived upgrade ticket in a cookie (`SameSite=Strict`) instead of the long-term password in the URL.

### N2. `sessionStorage` holds the proxy password

Clears on tab close; any XSS in `/ui/` can read it. Keep CSP tight (M6).

---

## What is in reasonably good shape

- Mesh HTTP is not a backdoor into the UI mux.  
- Config dump is gone; provider tokens are not returned to the UI.  
- Node register/refresh/unregister (and authorize-other) are bound to the session user.  
- JSON mutating APIs reject non-JSON content-types and cross-origin `Origin`.  
- Websocket origin + optional password on upgrade.  
- Optional `proxy.password` with UI sign-in; default listen is loopback.  
- Invite IDs are SHA-256 of a UUID; node mesh secrets are bcrypt.  
- Session blobs are AES-256-GCM. File modes `0600` on config/keys/allow-list. Ed25519 node keys.  
- Private providers skipped for mesh-origin inference.  
- Production log level `LogNormal` unless `debug`.

---

## Suggested order of work

1. **C1** — authenticate local inference (or isolate it).  
2. **C4 / H1** — bind sessions to ACL; expire/revoke on kick.  
3. **H6** — provider URL allow-list.  
4. **C5** — rotate working-tree secrets.  
5. **N1** — stop putting the password in the WS query.  
6. **M1 / M3 / M5 / M6** — rate limits, TLS, body/timeouts, CSP.

---

## Out of scope / not verified

- Dependency CVEs (`go.mod` / libp2p / gorilla / badger).  
- Whether `config.yaml` or keys exist in **git history** on the remote.  
- Production TLS terminator configuration.  
- Load testing of hole punching and relay ACL.

This document is a review of the current tree, not a guarantee that every defect was found.
)
