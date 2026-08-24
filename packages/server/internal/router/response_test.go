package router

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, map[string]string{"ok": "true"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	if body := rec.Body.String(); body != "{\"ok\":\"true\"}\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"sentinel", ErrNotFound, http.StatusNotFound, `{"error":"not found"}`},
		{"wrapped", fmt.Errorf("get pkg: %w", ErrNotFound), http.StatusNotFound, `{"error":"not found"}`},
		{"custom", NewError(418, "teapot"), http.StatusTeapot, `{"error":"teapot"}`},
		{"unknown", errors.New("boom"), http.StatusInternalServerError, `{"error":"internal server error"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if body := rec.Body.String(); body != tt.wantBody+"\n" {
				t.Fatalf("body = %q, want %q", body, tt.wantBody+"\n")
			}
		})
	}
}
