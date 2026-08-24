package router

import (
	"encoding/json"
	"errors"
	"net/http"
)

type Error struct {
	status  int
	message string
}

func (e *Error) Status() int   { return e.status }
func (e *Error) Error() string { return e.message }

func NewError(status int, message string) *Error {
	return &Error{status: status, message: message}
}

var (
	ErrBadRequest       = NewError(http.StatusBadRequest, "bad request")
	ErrUnauthorized     = NewError(http.StatusUnauthorized, "unauthorized")
	ErrForbidden        = NewError(http.StatusForbidden, "forbidden")
	ErrNotFound         = NewError(http.StatusNotFound, "not found")
	ErrMethodNotAllowed = NewError(http.StatusMethodNotAllowed, "method not allowed")
	ErrInternal         = NewError(http.StatusInternalServerError, "internal server error")
)

type errorResponse struct {
	Error string `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = ErrInternal
	}
	WriteJSON(w, e.status, errorResponse{Error: e.message})
}
