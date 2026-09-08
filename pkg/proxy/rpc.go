package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/asynchronomatic/speakeasy/api"
)

type RPC struct {
	w http.ResponseWriter
	r *http.Request
}

func (rpc *RPC) Request() *http.Request {
	return rpc.r
}

func (rpc *RPC) PathVar(name string) string {
	return rpc.r.PathValue(name)
}

func (rpc *RPC) GetObject(obj any) error {
	defer rpc.r.Body.Close()
	if err := api.RequireJSONContentType(rpc.r); err != nil {
		return err
	}
	err := json.NewDecoder(rpc.r.Body).Decode(obj)
	if err != nil {
		return api.NewError(http.StatusBadRequest, "bad request")
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
