package jsonrpc

import (
	"encoding/json"
	"mime"
	"net/http"
	"strings"
)

type Properties struct {
	User       string
	Group      string
	Roles      map[string]bool // for now just check if has role
	Properties map[string]any
}

// IsJSONContentType reports whether ct is application/json, ignoring parameters
// such as charset.
func IsJSONContentType(ct string) bool {
	media, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return strings.EqualFold(media, "application/json")
}

// RequireJSONContentType rejects requests whose Content-Type is not JSON.
// Browser "simple" POSTs (text/plain, form-urlencoded) cannot satisfy this,
// so they never skip CORS preflight into JSON mutating APIs.
func RequireJSONContentType(r *http.Request) error {
	if !IsJSONContentType(r.Header.Get("Content-Type")) {
		return NewError(http.StatusUnsupportedMediaType, "content-type must be application/json")
	}
	return nil
}

type RPC struct {
	w     http.ResponseWriter
	r     *http.Request
	props Properties
}

func (rpc *RPC) User() string {
	return rpc.props.User
}

func (rpc *RPC) Group() string {
	return rpc.props.Group
}

func (rpc *RPC) WithProps(props Properties) *RPC {
	rpc.props = props
	return rpc
}

func (rpc *RPC) Request() *http.Request {
	return rpc.r
}

func (rpc *RPC) PathVar(name string) string {
	return rpc.r.PathValue(name)
}

func (rpc *RPC) GetObject(obj any) error {
	defer rpc.r.Body.Close()
	if err := RequireJSONContentType(rpc.r); err != nil {
		return err
	}
	err := json.NewDecoder(rpc.r.Body).Decode(obj)
	if err != nil {
		return NewError(http.StatusBadRequest, "bad request")
	}
	return nil
}

func (rpc *RPC) ReplyObject(obj any) error {
	rpc.w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(rpc.w).Encode(obj)
}

func (rpc *RPC) Error(code int, msg string) error {
	http.Error(rpc.w, msg, code)
	return nil
}

func (rpc *RPC) SetCookie(cookie *http.Cookie) {
	cookie.Secure = rpc.r.TLS != nil
	http.SetCookie(rpc.w, cookie)
}

func NewRPC(w http.ResponseWriter, r *http.Request) *RPC {
	return &RPC{w: w, r: r}
}
