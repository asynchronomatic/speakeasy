package admin

import (
	"encoding/json"
	"net/http"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/admin/auth"
)

type JsonRPC struct {
	w    http.ResponseWriter
	r    *http.Request
	user *auth.Properties
}

func (c *JsonRPC) Request() *http.Request {
	return c.r
}

func (c *JsonRPC) User() string {
	return c.user.User
}

func (c *JsonRPC) Group() string {
	return c.user.Group
}

func (c *JsonRPC) PathVar(name string) string {
	return c.r.PathValue(name)
}

func (c *JsonRPC) GetObject(obj any) error {
	defer c.r.Body.Close()
	if err := api.RequireJSONContentType(c.r); err != nil {
		return err
	}
	err := json.NewDecoder(c.r.Body).Decode(obj)
	if err != nil {
		return api.NewError(http.StatusBadRequest, "bad request")
	}
	return nil
}

func (c *JsonRPC) ReplyObject(obj any) error {
	c.w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(c.w).Encode(obj)
}

func (c *JsonRPC) Error(code int, msg string) {
	http.Error(c.w, msg, code)
}
