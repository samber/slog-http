package sloghttp

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyWriter_ReadFrom(t *testing.T) {
	content := "<!DOCTYPE html><html lang=\"en\">"

	tests := []struct {
		name       string
		recordBody bool
	}{
		{"with record body", true},
		{"without record body", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			bw := newBodyWriter(w, 16, tt.recordBody)
			// wrap in simpleReader to use io.ReadFrom instead of io.WriteTo to check for infinite recursion
			reader := simpleReader{strings.NewReader(content)}

			n, err := bw.ReadFrom(reader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expectedN := len(content)
			if n != int64(expectedN) {
				t.Errorf("expected n to be %d, got %d", expectedN, n)
			}
			if bw.bytes != expectedN {
				t.Errorf("expected bytes to be %d, got %d", expectedN, bw.bytes)
			}
			if w.Body.String() != content {
				t.Errorf("expected response body to be %q, got %q", content, w.Body.String())
			}
			if tt.recordBody && bw.body.String() != content[:16] {
				t.Errorf("expected captured body to be truncated to %q, got %q", content[:16], bw.body.String())
			}
		})
	}
}

type simpleReader struct {
	io.Reader
}
