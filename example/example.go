package main

import (
	"net/http"
	"os"
	"time"

	"log/slog"

	slogformatter "github.com/samber/slog-formatter"
	sloghttp "github.com/samber/slog-http"
)

func main() {
	// Create a slog logger, which:
	//   - Logs to stdout.
	//   - RFC3339 with UTC time format.
	logger := slog.New(
		slogformatter.NewFormatterHandler(
			slogformatter.TimezoneConverter(time.UTC),
			slogformatter.TimeFormatter(time.RFC3339, nil),
		)(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}),
		),
	)

	// Add an attribute to all log entries made through this logger.
	logger = logger.With("env", "production")

	// mux router
	mux := http.NewServeMux()

	// Routes
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, World!"))
	}))
	mux.Handle("/foobar/42", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sloghttp.AddCustomAttributes(r, slog.String("foo", "bar"))
		w.Write([]byte("Hello, World!"))
	}))
	mux.Handle("/error", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "A simulated error", http.StatusInternalServerError)
	}))
	mux.Handle("/panic", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("failed")
	}))

	// Middleware
	handler := sloghttp.Recovery(mux)
	// config := sloghttp.Config{WithRequestBody: true, WithResponseBody: true, WithRequestHeader: true, WithResponseHeader: true}
	// handler = sloghttp.NewWithConfig(logger, config)(handler)
	handler = sloghttp.New(logger.WithGroup("http"))(handler)

	// Start server
	http.ListenAndServe(":4242", handler)

	// output:
	// time=2023-04-10T14:00:00Z level=INFO msg="Success" env=production http.status=200 http.method=GET http.path=/ http.ip=::1 http.latency=25.958µs http.user-agent=curl/7.77.0 http.time=2023-04-10T14:00:00Z http.request-id=229c7fc8-64f5-4467-bc4a-940700503b0d
}
