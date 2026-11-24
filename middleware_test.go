package sloghttp

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddleware_FilterCanReadBodyWithoutRecording(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	config := DefaultConfig()
	config.Filters = []Filter{
		func(w WrapResponseWriter, r *http.Request) bool {
			return len(w.Body()) == 0
		},
	}

	handler := NewWithConfig(logger, config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Body.String() != "hello" {
		t.Errorf("expected response body %q, got %q", "hello", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "status=200") {
		t.Errorf("expected the request to be logged, got %q", logs.String())
	}
}
