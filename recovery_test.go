package sloghttp

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Both mounting orders must behave the same way.
var orders = []struct {
	name          string
	recoveryOuter bool
}{
	{"Recovery inside logger", false},
	{"Recovery outside logger", true},
}

// serve runs a request through the logger and Recovery, in the requested order.
// It returns the recorder, the logs, and any panic that escaped both middlewares.
func serve(t *testing.T, cfg Config, recoveryOuter bool, handler http.HandlerFunc, path string) (rec *httptest.ResponseRecorder, logs string, escaped any) {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	var h http.Handler
	if recoveryOuter {
		h = Recovery(NewWithConfig(logger, cfg)(handler))
	} else {
		h = NewWithConfig(logger, cfg)(Recovery(handler))
	}

	rec = httptest.NewRecorder()
	func() {
		defer func() { escaped = recover() }()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	}()

	return rec, buf.String(), escaped
}

func panicking(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { panic(v) }
}

func logLine(t *testing.T, logs string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(logs), &m); err != nil {
		t.Fatalf("invalid log %q: %v", logs, err)
	}
	return m
}

func status(m map[string]any) int {
	return int(m["response"].(map[string]any)["status"].(float64))
}

func TestPanic_Logging(t *testing.T) {
	flags := []struct {
		name      string
		withPanic bool
		withStack bool
	}{
		{"default logs type only", false, false},
		{"WithPanic", true, false},
		{"WithStack", false, true},
		{"WithPanic and WithStack", true, true},
	}

	for _, o := range orders {
		for _, f := range flags {
			t.Run(o.name+"/"+f.name, func(t *testing.T) {
				cfg := DefaultConfig()
				cfg.WithPanic, cfg.WithStack = f.withPanic, f.withStack

				rec, logs, escaped := serve(t, cfg, o.recoveryOuter, panicking("member 123 secret"), "/")

				if escaped != nil {
					t.Fatalf("panic escaped: %v", escaped)
				}
				if rec.Code != http.StatusInternalServerError {
					t.Errorf("response code = %d, want 500", rec.Code)
				}
				if strings.Contains(rec.Body.String(), "secret") {
					t.Errorf("panic value leaked in response: %q", rec.Body.String())
				}

				m := logLine(t, logs)
				if m["panic_type"] != "string" {
					t.Errorf("panic_type = %v, want string", m["panic_type"])
				}
				if got := status(m); got != 500 {
					t.Errorf("logged status = %d, want 500", got)
				}
				if m["level"] != "ERROR" {
					t.Errorf("level = %v, want ERROR", m["level"])
				}
				if v, ok := m["panic"]; ok != f.withPanic || (ok && v != "member 123 secret") {
					t.Errorf("panic = %v (present=%v), want present=%v", v, ok, f.withPanic)
				}
				if v, ok := m["stack"]; ok != f.withStack || (ok && !strings.Contains(v.(string), "goroutine")) {
					t.Errorf("stack present=%v, want %v", ok, f.withStack)
				}
				if !f.withPanic && strings.Contains(logs, "secret") {
					t.Errorf("panic value leaked into logs: %s", logs)
				}
			})
		}
	}
}

func TestPanic_PropagatesWithoutRecovery(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := NewWithConfig(logger, DefaultConfig())(panicking("boom"))

	var escaped any
	func() {
		defer func() { escaped = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()

	if escaped != "boom" {
		t.Errorf("escaped = %v, want boom", escaped)
	}
	if logLine(t, buf.String())["panic_type"] != "string" {
		t.Errorf("panic should be logged before propagating: %s", buf.String())
	}
}

func TestPanic_AlreadyStartedKeepsStatus(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		panic("late")
	}

	for _, o := range orders {
		t.Run(o.name, func(t *testing.T) {
			_, logs, _ := serve(t, DefaultConfig(), o.recoveryOuter, h, "/")

			if got := status(logLine(t, logs)); got != http.StatusAccepted {
				t.Errorf("logged status = %d, want 202", got)
			}
		})
	}
}

func TestPanic_ErrAbortHandler(t *testing.T) {
	for _, o := range orders {
		t.Run(o.name, func(t *testing.T) {
			rec, logs, escaped := serve(t, DefaultConfig(), o.recoveryOuter, panicking(http.ErrAbortHandler), "/")

			if escaped != nil {
				t.Errorf("Recovery should swallow ErrAbortHandler, got %v", escaped)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("Recovery must not write a body for ErrAbortHandler, got %q", rec.Body.String())
			}
			if logLine(t, logs)["panic_type"] == nil {
				t.Error("abort should still be logged")
			}
		})
	}
}

func TestPanic_FilteredStillRecovered(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filters = []Filter{IgnorePath("/health")}

	for _, o := range orders {
		t.Run(o.name, func(t *testing.T) {
			rec, logs, escaped := serve(t, cfg, o.recoveryOuter, panicking("boom"), "/health")

			if logs != "" {
				t.Errorf("filtered request must not be logged, got %q", logs)
			}
			if escaped != nil || rec.Code != http.StatusInternalServerError {
				t.Errorf("panic must still be recovered with a 500 (escaped=%v, code=%d)", escaped, rec.Code)
			}
		})
	}
}

func TestNoPanic_NoPanicAttributes(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

	for _, o := range orders {
		t.Run(o.name, func(t *testing.T) {
			_, logs, escaped := serve(t, DefaultConfig(), o.recoveryOuter, ok, "/")

			if escaped != nil {
				t.Fatalf("unexpected panic: %v", escaped)
			}
			if strings.Contains(logs, "panic") {
				t.Errorf("unexpected panic attributes: %s", logs)
			}
		})
	}
}

func TestGetPanic(t *testing.T) {
	var got *Panic
	observe := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			got = GetPanic(r)
		})
	}

	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	run := func(h http.Handler) {
		got = nil
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}

	// logger > observe > Recovery > handler
	run(NewWithConfig(logger, DefaultConfig())(observe(Recovery(panicking("boom")))))
	if got == nil || got.Value != "boom" || got.Type != "string" || !strings.Contains(got.Stack, "goroutine") {
		t.Errorf("GetPanic = %+v, want the recovered panic with its type and stack", got)
	}

	// no panic
	ok := func(w http.ResponseWriter, r *http.Request) {}
	run(NewWithConfig(logger, DefaultConfig())(observe(Recovery(http.HandlerFunc(ok)))))
	if got != nil {
		t.Errorf("GetPanic = %+v, want nil without a panic", got)
	}

	// no logging middleware outside: nothing is recorded, Recovery still recovers
	run(observe(Recovery(panicking("boom"))))
	if got != nil {
		t.Errorf("GetPanic = %+v, want nil without a logging middleware", got)
	}
}

func TestInformationalStatusDoesNotStick(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusCreated)
	}
	_, logs, _ := serve(t, DefaultConfig(), false, h, "/")

	if got := status(logLine(t, logs)); got != http.StatusCreated {
		t.Errorf("logged status = %d, want 201", got)
	}
}
