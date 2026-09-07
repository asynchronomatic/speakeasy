package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postModel(t *testing.T, p *Proxy, path, model string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	return rec
}

func TestChatCompletionsUnknownModelOpenAIError(t *testing.T) {
	p := testProxy(t)
	rec := postModel(t, p, "/v1/chat/completions", "nope-model")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	var got openaiAPIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v body %s", err, rec.Body.String())
	}
	if got.Error.Type != "invalid_request_error" {
		t.Fatalf("type %q", got.Error.Type)
	}
	if got.Error.Code != "model_not_found" {
		t.Fatalf("code %q", got.Error.Code)
	}
	if got.Error.Param != nil {
		t.Fatalf("param %+v want null", got.Error.Param)
	}
	if !strings.Contains(got.Error.Message, "`nope-model`") {
		t.Fatalf("message %q", got.Error.Message)
	}
}

func TestResponsesUnknownModelOpenAIError(t *testing.T) {
	p := testProxy(t)
	body := `{"model":"missing","input":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d want 404", rec.Code)
	}
	var got openaiAPIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v body %s", err, rec.Body.String())
	}
	if got.Error.Code != "model_not_found" || got.Error.Type != "invalid_request_error" {
		t.Fatalf("error %+v", got.Error)
	}
	if !strings.Contains(got.Error.Message, "`missing`") {
		t.Fatalf("message %q", got.Error.Message)
	}
}

func TestOllamaChatUnknownModelJSON(t *testing.T) {
	p := testProxy(t)
	rec := postModel(t, p, "/api/chat", "nope-model")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v body %s", err, rec.Body.String())
	}
	if got["error"] != "model 'nope-model' not found" {
		t.Fatalf("error %q", got["error"])
	}
}

func TestMeshUnknownModelOpenAIError(t *testing.T) {
	p := testProxy(t)
	body := `{"model":"ghost","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	p.MeshServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d want 404", rec.Code)
	}
	var got openaiAPIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v body %s", err, rec.Body.String())
	}
	if got.Error.Code != "model_not_found" {
		t.Fatalf("error %+v", got.Error)
	}
}
