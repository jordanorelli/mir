package main

import (
	"net/http"
)

type apiError int

func (e apiError) Error() string {
	return http.StatusText(int(e))
}

type errorNode struct {
	err    error
	parent error
}

func (e errorNode) Error() string {
	return e.err.Error()
}

func (e errorNode) Unwrap() error {
	return e.parent
}

func joinErrors(e, e2 error) error {
	if e == nil {
		return e2
	}
	if e2 == nil {
		return e
	}
	return errorNode{err: e, parent: e2}
}
