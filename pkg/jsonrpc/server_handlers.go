package jsonrpc

import (
	"net/http"
)

const (
	AdminGroup = "admin"
	AdminUser  = "admin"
)

func AsGroup(group string, fn func(*RPC) error) func(*RPC) error {
	return func(rpc *RPC) error {
		if rpc.Group() != group {
			return NewError(http.StatusUnauthorized, "unauthorized")
		}
		return fn(rpc)
	}
}

func AsAdmin(fn func(*RPC) error) func(*RPC) error {
	return AsGroup(AdminGroup, fn)
}
