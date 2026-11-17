package sloghttp

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestFieldToggles(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		present map[string]bool
	}{
		{"defaults", func(*Config) {}, map[string]bool{"path": true, "query": true, "referer": true}},
		{"no path", func(c *Config) { c.WithPath = false }, map[string]bool{"path": false, "query": true, "referer": true}},
		{"no query", func(c *Config) { c.WithQuery = false }, map[string]bool{"path": true, "query": false, "referer": true}},
		{"no referer", func(c *Config) { c.WithReferer = false }, map[string]bool{"path": true, "query": true, "referer": false}},
		{"none", func(c *Config) { c.WithPath, c.WithQuery, c.WithReferer = false, false, false }, map[string]bool{"path": false, "query": false, "referer": false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))

			cfg := DefaultConfig()
			tt.mutate(&cfg)

			h := NewWithConfig(logger, cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/foo?a=b", nil)
			req.Header.Set("Referer", "https://example.com/")
			h.ServeHTTP(httptest.NewRecorder(), req)

			var out struct {
				Request map[string]any `json:"request"`
			}
			if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
				t.Fatalf("invalid log output %q: %v", buf.String(), err)
			}

			for key, want := range tt.present {
				if _, got := out.Request[key]; got != want {
					t.Errorf("field %q present = %v, want %v", key, got, want)
				}
			}
		})
	}
}
