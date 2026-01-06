package sloghttp

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

type customAttributesCtxKeyType struct{}
type requestIDCtxKeyType struct{}

var customAttributesCtxKey = customAttributesCtxKeyType{}
var requestIDCtxKey = requestIDCtxKeyType{}

var (
	TraceIDKey   = "trace_id"
	SpanIDKey    = "span_id"
	RequestIDKey = "id"

	RequestBodyMaxSize  = 64 * 1024 // 64KB
	ResponseBodyMaxSize = 64 * 1024 // 64KB

	HiddenRequestHeaders = map[string]struct{}{
		"authorization": {},
		"cookie":        {},
		"set-cookie":    {},
		"x-auth-token":  {},
		"x-csrf-token":  {},
		"x-xsrf-token":  {},
	}
	HiddenResponseHeaders = map[string]struct{}{
		"set-cookie": {},
	}

	// Formatted with http.CanonicalHeaderKey
	RequestIDHeaderKey = "X-Request-Id"
)

type Config struct {
	DefaultLevel     slog.Level
	ClientErrorLevel slog.Level
	ServerErrorLevel slog.Level

	WithUserAgent      bool
	WithRequestID      bool
	WithRequestBody    bool
	WithRequestHeader  bool
	WithResponseBody   bool
	WithResponseHeader bool
	WithSpanID         bool
	WithTraceID        bool
	WithClientIP       bool

	Filters []Filter
}

// New returns a `func(http.Handler) http.Handler` (middleware) that logs requests using slog.
//
// Requests with errors are logged using slog.Error().
// Requests without errors are logged using slog.Info().
func New(logger *slog.Logger) func(http.Handler) http.Handler {
	return NewWithConfig(logger, DefaultConfig())
}

// NewWithFilters returns a `func(http.Handler) http.Handler` (middleware) that logs requests using slog.
//
// Requests with errors are logged using slog.Error().
// Requests without errors are logged using slog.Info().
func NewWithFilters(logger *slog.Logger, filters ...Filter) func(http.Handler) http.Handler {
	config := DefaultConfig()
	config.Filters = filters
	return NewWithConfig(logger, config)
}

// DefaultConfig returns the default configuration for the request logger.
func DefaultConfig() Config {
	return Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,

		WithUserAgent:      false,
		WithRequestID:      true,
		WithRequestBody:    false,
		WithRequestHeader:  false,
		WithResponseBody:   false,
		WithResponseHeader: false,
		WithSpanID:         false,
		WithTraceID:        false,
		WithClientIP:       true,

		Filters: []Filter{},
	}
}

// NewWithConfig returns a `func(http.Handler) http.Handler` (middleware) that logs requests using slog.
func NewWithConfig(logger *slog.Logger, config Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			path := r.URL.Path
			query := r.URL.RawQuery

			requestID := r.Header.Get(RequestIDHeaderKey)
			if config.WithRequestID {
				if requestID == "" {
					requestID = uuid.New().String()
					r.Header.Set(RequestIDHeaderKey, requestID)
				}
				r = r.WithContext(context.WithValue(r.Context(), requestIDCtxKey, requestID))
			}

			// dump request body
			br := newBodyReader(r.Body, RequestBodyMaxSize, config.WithRequestBody)
			r.Body = br

			// dump response body
			bw := newBodyWriter(w, ResponseBodyMaxSize, config.WithResponseBody)

			// Make sure we create a map only once per request (in case we have multiple middleware instances)
			if v := r.Context().Value(customAttributesCtxKey); v == nil {
				r = r.WithContext(context.WithValue(r.Context(), customAttributesCtxKey, &sync.Map{}))
			}

			defer func() {
				// Pass thru filters and skip early the code below, to prevent unnecessary processing.
				for _, filter := range config.Filters {
					if !filter(bw, r) {
						return
					}
				}

				status := bw.Status()
				method := r.Method
				host := r.Host
				end := time.Now()
				latency := end.Sub(start)
				userAgent := r.UserAgent()
				referer := r.Referer()

				baseAttributes := make([]slog.Attr, 0, 3)
				requestAttributes := make([]slog.Attr, 0, 11)
				responseAttributes := make([]slog.Attr, 0, 6)

				requestAttributes = append(requestAttributes,
					slog.Time("time", start.UTC()),
					slog.String("method", method),
					slog.String("host", host),
					slog.String("path", path),
					slog.String("query", query),
					slog.String("referer", referer),
				)

				if config.WithClientIP {
					requestAttributes = append(requestAttributes,
						slog.String("ip", r.RemoteAddr),
					)
				}

				responseAttributes = append(responseAttributes,
					slog.Time("time", end.UTC()),
					slog.Duration("latency", latency),
					slog.Int("status", status),
				)

				if config.WithRequestID {
					baseAttributes = append(baseAttributes, slog.String(RequestIDKey, requestID))
				}

				// otel
				baseAttributes = append(baseAttributes, extractTraceSpanID(r.Context(), config.WithTraceID, config.WithSpanID)...)

				// request body
				requestAttributes = append(requestAttributes, slog.Int("length", br.bytes))
				if config.WithRequestBody {
					requestAttributes = append(requestAttributes, slog.String("body", br.body.String()))
				}

				// request headers
				if config.WithRequestHeader {
					kv := []any{}

					for k, v := range r.Header {
						if _, found := HiddenRequestHeaders[strings.ToLower(k)]; found {
							continue
						}
						kv = append(kv, slog.Any(k, v))
					}

					requestAttributes = append(requestAttributes, slog.Group("header", kv...))
				}

				if config.WithUserAgent {
					requestAttributes = append(requestAttributes, slog.String("user-agent", userAgent))
				}

				// response body
				responseAttributes = append(responseAttributes, slog.Int("length", bw.bytes))
				if config.WithResponseBody {
					responseAttributes = append(responseAttributes, slog.String("body", bw.body.String()))
				}

				// response headers
				if config.WithResponseHeader {
					kv := []any{}

					for k, v := range w.Header() {
						if _, found := HiddenResponseHeaders[strings.ToLower(k)]; found {
							continue
						}
						kv = append(kv, slog.Any(k, v))
					}

					responseAttributes = append(responseAttributes, slog.Group("header", kv...))
				}

				attributes := append(
					[]slog.Attr{
						{
							Key:   "request",
							Value: slog.GroupValue(requestAttributes...),
						},
						{
							Key:   "response",
							Value: slog.GroupValue(responseAttributes...),
						},
					},
					baseAttributes...,
				)

				// custom context values
				if v := r.Context().Value(customAttributesCtxKey); v != nil {
					if m, ok := v.(*sync.Map); ok {
						m.Range(func(key, value any) bool {
							attributes = append(attributes, slog.Attr{Key: key.(string), Value: value.(slog.Value)})
							return true
						})
					}
				}

				level := config.DefaultLevel
				if status >= http.StatusInternalServerError {
					level = config.ServerErrorLevel
				} else if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
					level = config.ClientErrorLevel
				}

				logger.LogAttrs(r.Context(), level, strconv.Itoa(status)+": "+http.StatusText(status), attributes...)
			}()

			next.ServeHTTP(bw, r)
		})
	}
}

// GetRequestID returns the request identifier.
func GetRequestID(r *http.Request) string {
	return GetRequestIDFromContext(r.Context())
}

// GetRequestIDFromContext returns the request identifier from the context.
func GetRequestIDFromContext(ctx context.Context) string {
	requestID := ctx.Value(requestIDCtxKey)
	if id, ok := requestID.(string); ok {
		return id
	}

	return ""
}

// AddCustomAttributes adds custom attributes to the request context. This func can be called from any handler or middleware, as long as the slog-http middleware is already mounted.
func AddCustomAttributes(r *http.Request, attrs ...slog.Attr) {
	AddContextAttributes(r.Context(), attrs...)
}

// AddContextAttributes is the same as AddCustomAttributes, but it doesn't need access to the request struct.
func AddContextAttributes(ctx context.Context, attrs ...slog.Attr) {
	if v := ctx.Value(customAttributesCtxKey); v != nil {
		if m, ok := v.(*sync.Map); ok {
			for _, attr := range attrs {
				m.Store(attr.Key, attr.Value)
			}
		}
	}
}

func extractTraceSpanID(ctx context.Context, withTraceID bool, withSpanID bool) []slog.Attr {
	if !(withTraceID || withSpanID) {
		return []slog.Attr{}
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return []slog.Attr{}
	}

	attrs := make([]slog.Attr, 0, 2)
	spanCtx := span.SpanContext()

	if withTraceID && spanCtx.HasTraceID() {
		traceID := trace.SpanFromContext(ctx).SpanContext().TraceID().String()
		attrs = append(attrs, slog.String(TraceIDKey, traceID))
	}

	if withSpanID && spanCtx.HasSpanID() {
		spanID := spanCtx.SpanID().String()
		attrs = append(attrs, slog.String(SpanIDKey, spanID))
	}

	return attrs
}
