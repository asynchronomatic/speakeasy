package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ollama/ollama/types/model"

	"github.com/asynchronomatic/speakeasy/web"
)

func TestWebDirHasIndex(t *testing.T) {
	dir := webDir()
	index := filepath.Join(dir, "index.html")
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("expected %s: %v", index, err)
	}
}

func TestEmbeddedWebHasIndex(t *testing.T) {
	f, err := web.FS.Open("index.html")
	if err != nil {
		t.Fatalf("embedded index.html: %v", err)
	}
	f.Close()
}

func TestUIStaticAssets(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /ui/", http.StripPrefix("/ui/", http.FileServer(uiFileSystem())))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, path := range []string{"/ui/", "/ui/css/styles.css", "/ui/js/app.js", "/ui/js/theme-boot.js", "/ui/favicon.ico", "/ui/favicon.svg"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d", path, res.StatusCode)
		}
		res.Body.Close()
	}

	res, err := http.Get(srv.URL + "/ui/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, needle := range []string{"Speakeasy", "Welcome", "OpenAI endpoint", "/v1", "Open WebUI", "Mesh", "Models", "view-admin", "Enter admin token", "admin-enable-form", "admin-locked", "New Invite", "admin-invite-modal", "admin-invite-revoke-modal", "admin-node-kick-modal", "admin-nodes-body", "Peer ID", "theme-select", "theme-error", "login-overlay", "login-form", "login-username", `value="admin"`, "readonly", "Sign in", `id="app" class="dashboard hidden"`, "data-theme", "Deco", "Cyber", "Clean", `value="cyber"`, `value="clean"`, "/ui/favicon.ico", "New Provider", "providers-body", "provider-modal", "provider-delete-modal", "provider-models-body", "provider-model-add", "modal-card-provider", "Add model", "Model whitelist (only the listed models will be exported)", "<th>Owner</th>", "<th>Context</th>", "<th>Visibility</th>", "<th>Capabilities</th>", "sk-speakeasy", `value="86400" selected`, `id="admin-invite-once" checked`, "Never expires", "<th>Uses</th>", "Inference Tokens", "inference-tokens-body", "inference-token-modal", "inference-secret-modal", "inference-token-delete-modal", "inference-insecure", `role="switch"`, "switch-track", "inference-insecure-notice", "New Token", "Allow inference without a token", "Inference is public", "<th>Token</th>"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("index missing %s", needle)
		}
	}
	for _, old := range []string{"ModelMesh", "sk-modelmesh", "settings-yaml", "config.yaml", "debug-toggle", "debug-error"} {
		if strings.Contains(body, old) {
			t.Fatalf("index still has old branding %s", old)
		}
	}
	for _, old := range []string{"<th>Size</th>", "<th>Family</th>", "<th>Parameters</th>"} {
		if strings.Contains(body, old) {
			t.Fatalf("index still has old model column %s", old)
		}
	}
}

func TestUIModelsJSONShape(t *testing.T) {
	resp := UIModelsResponse{
		Models: []UIModel{{
			Name:          "llama3.2:latest",
			Model:         "llama3.2:latest",
			Private:       true,
			Owner:         "alice",
			ContextLength: 8192,
			Capabilities:  []string{"completion", "tools", "vision"},
			Providers: []UIModelProvider{{
				Name: "local",
				ID:   "12D3KooWtest",
			}},
		}},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, needle := range []string{
		`"models"`, `"providers"`, `"ID":"12D3KooWtest"`, `"Name":"local"`,
		`"capabilities"`, `"completion"`, `"vision"`,
		`"private":true`, `"owner":"alice"`, `"context_length":8192`,
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("missing %s in %s", needle, s)
		}
	}
	for _, old := range []string{`"digest"`, `"parameter_size"`, `"quantization"`, `"size_vram"`, `"loaded"`, `"peer_id"`, `"self"`} {
		if strings.Contains(s, old) {
			t.Fatalf("unexpected legacy field %s in %s", old, s)
		}
	}
}

func TestAppJSUsesNewUIModelFields(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(webDir(), "js", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, needle := range []string{"m.private", "m.owner", "m.context_length", "badge-private", "badge-shared", "providerID", "providerName", "providerIsSelf"} {
		if !strings.Contains(s, needle) {
			t.Fatalf("app.js missing %s", needle)
		}
	}
	for _, old := range []string{"m.loaded", "m.parameter_size", "m.size_vram", "m.digest", "formatBytes", "m.family", "m.quantization", "p.peer_id", "p.self"} {
		if strings.Contains(s, old) {
			t.Fatalf("app.js still uses legacy UIModel field %s", old)
		}
	}
}

func TestAppJSUsesRefreshWebsocket(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(webDir(), "js", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/api/v.1/refresh/websocket") {
		t.Fatal("app.js should connect to /api/v.1/refresh/websocket")
	}
	if !strings.Contains(s, "new WebSocket") {
		t.Fatal("app.js should open a WebSocket for refresh")
	}
	if !strings.Contains(s, "/api/mesh/refresh/ticket") {
		t.Fatal("app.js should fetch a websocket ticket before connecting")
	}
	for _, old := range []string{"setInterval(refresh", "POLL_MS", "access_token"} {
		if strings.Contains(s, old) {
			t.Fatalf("app.js still polls with %s", old)
		}
	}
}

func TestAppJSAdminPanel(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(webDir(), "js", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, needle := range []string{
		"/api/admin/enabled",
		"/api/admin/invite",
		"/api/admin/node",
		"checkAdmin",
		`state.view !== "admin"`,
		"enableAdmin",
		"setAdminEnabled",
		"createInvite",
		"Reusable",
		"1 remaining",
		"This invite will never expire",
		"openInviteModal",
		"revokeInvite",
		"openRevokeInviteModal",
		"confirmRevokeInvite",
		"kickNode",
		"openKickNodeModal",
		"confirmKickNode",
		"data.enabled || data.Enabled",
		"slice(-16)",
		"data-copy-link",
		"execCommand",
		"fallbackCopy",
		"speakeasy-theme",
		"applyTheme",
		"normalizeTheme",
		"data-theme",
		"themeSelect",
		"theme-select",
		"cyber",
		"clean",
		"/api/mesh/theme",
		"saveTheme",
		"loadTheme",
		"bootAuth",
		"requireLogin",
		"verifyProxyToken",
		"/api/mesh/login",
		`user: "admin"`,
		"submitLogin",
		"AUTH_KEY",
		"/api/mesh/providers",
		"openProviderModal",
		"saveProvider",
		"removeProvider",
		"openProviderDeleteModal",
		"confirmProviderDelete",
		"data-edit-provider",
		"collectProviderModels",
		"addProviderModelRow",
		"data-remove-provider-model",
		"/api/proxy/inference/tokens",
		"loadInferenceTokens",
		"createInferenceToken",
		"removeInferenceToken",
		"openInferenceTokenDeleteModal",
		"confirmInferenceTokenDelete",
		"saveInferenceInsecure",
		"updateInferenceInsecureNotice",
		"openInferenceSecretModal",
		"inferenceTokenValue",
		"data-remove-inference-token",
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("app.js missing %s", needle)
		}
	}
	for _, old := range []string{"/api/mesh/debug", "saveDebug", "loadDebug", "debug-toggle"} {
		if strings.Contains(s, old) {
			t.Fatalf("app.js still has debug control %s", old)
		}
	}
}

func TestCapabilityStrings(t *testing.T) {
	got := capabilityStrings([]model.Capability{
		"completion",
		" tools ",
		"",
		"completion",
		"vision",
	})
	want := []string{"completion", "tools", "vision"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
