package api

import (
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

type Error = jsonrpc.Error

/*
func NewError(code int, msg string) *Error {
	return jsonclient.NewRequestError(code, msg)
}*/
