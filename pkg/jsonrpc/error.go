package jsonrpc

import (
	"errors"
	"fmt"
)

type Error struct {
	code int
	err  error
}

func (re *Error) Code() int {
	return re.code
}

func (re *Error) Message() string {
	return re.err.Error()
}

func (re *Error) Error() string {
	return fmt.Sprintf("%d:%s", re.code, re.err)
}

func NewError(code int, err string) *Error {
	return &Error{
		code: code,
		err:  errors.New(err),
	}
}

/*
type RequestStatus struct {
	Code   int
	Status string
}*/
