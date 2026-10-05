package sloghttp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
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

func TestBodyWriter_CapturedPrefix(t *testing.T) {
	tests := []struct {
		name    string
		maxSize int
		chunks  []string
		want    string
	}{
		{"empty", 4, nil, ""},
		{"empty writes", 4, []string{"", "abc", ""}, "abc"},
		{"below limit", 4, []string{"ab", "c"}, "abc"},
		{"single overflow", 4, []string{"abcdef"}, "abcd"},
		{"exact limit", 4, []string{"ab", "cd"}, "abcd"},
		{"short crossing chunk", 4, []string{"abc", "de"}, "abcd"},
		{"long crossing chunk", 4, []string{"ab", "cdef"}, "abcd"},
		{"short write after limit", 4, []string{"abcd", "e"}, "abcd"},
		{"long write after limit", 4, []string{"abcd", "efghij"}, "abcd"},
		{"empty write after limit", 4, []string{"abcd", "", "e"}, "abcd"},
		{"repeated short writes", 4, []string{"a", "b", "c", "d", "e", "f"}, "abcd"},
		{"short write after overflow", 4, []string{"abcdef", "g"}, "abcd"},
		{"zero limit", 0, []string{"ab", "c", ""}, ""},
	}

	for _, tt := range tests {
		for _, readFrom := range []bool{false, true} {
			method := "Write"
			if readFrom {
				method = "ReadFrom"
			}
			t.Run(tt.name+"/"+method, func(t *testing.T) {
				response := httptest.NewRecorder()
				writer := newBodyWriter(response, tt.maxSize, true)
				content := strings.Join(tt.chunks, "")
				if readFrom {
					reader := &bodyChunksReader{chunks: append([]string(nil), tt.chunks...)}
					n, err := writer.ReadFrom(reader)
					if err != nil || n != int64(len(content)) {
						t.Fatalf("ReadFrom() = %d, %v, want %d, nil", n, err, len(content))
					}
				} else {
					for _, chunk := range tt.chunks {
						n, err := writer.Write([]byte(chunk))
						if err != nil || n != len(chunk) {
							t.Fatalf("Write() = %d, %v, want %d, nil", n, err, len(chunk))
						}
					}
				}
				if got := string(writer.Body()); got != tt.want {
					t.Errorf("captured body = %q, want %q", got, tt.want)
				}
				if got := response.Body.String(); got != content {
					t.Errorf("response body = %q, want %q", got, content)
				}
				if got := writer.BytesWritten(); got != len(content) {
					t.Errorf("bytes written = %d, want %d", got, len(content))
				}
			})
		}
	}
}

type bodyChunksReader struct {
	chunks []string
}

func (r *bodyChunksReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	r.chunks[0] = r.chunks[0][n:]
	if r.chunks[0] == "" {
		r.chunks = r.chunks[1:]
	}
	return n, nil
}

func TestMiddleware_CapturedResponsePrefix(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	config := DefaultConfig()
	config.WithResponseBody = true
	content := strings.Repeat("a", ResponseBodyMaxSize) + "bc"
	handler := NewWithConfig(logger, config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, chunk := range []string{content[:ResponseBodyMaxSize], "b", "c"} {
			if _, err := io.WriteString(w, chunk); err != nil {
				t.Fatalf("write response: %v", err)
			}
		}
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := response.Body.String(); got != content {
		t.Fatalf("response length = %d, want %d", len(got), len(content))
	}
	var entry struct {
		Response struct {
			Body   string
			Length int
		}
	}
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if got := entry.Response.Body; got != content[:ResponseBodyMaxSize] {
		t.Errorf("logged body length = %d, want prefix length %d", len(got), ResponseBodyMaxSize)
	}
	if got := entry.Response.Length; got != len(content) {
		t.Errorf("logged response length = %d, want %d", got, len(content))
	}
}
