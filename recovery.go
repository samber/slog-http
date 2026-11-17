package sloghttp

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
)

type panicCtxKeyType struct{}

var panicCtxKey = panicCtxKeyType{}

// Panic describes a panic recovered while handling a request.
type Panic struct {
	Value any
	Stack string
	Type  string
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			// Record the panic if a logging middleware is mounted outside of Recovery.
			if info, ok := r.Context().Value(panicCtxKey).(*Panic); ok {
				*info = Panic{Value: rec, Stack: string(debug.Stack()), Type: fmt.Sprintf("%T", rec)}
			}

			if rec != http.ErrAbortHandler {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// GetPanic returns the panic recovered by Recovery for this request, or nil.
func GetPanic(r *http.Request) *Panic {
	return GetPanicFromContext(r.Context())
}

// GetPanicFromContext returns the panic recovered by Recovery for this request, or nil.
func GetPanicFromContext(ctx context.Context) *Panic {
	if info, ok := ctx.Value(panicCtxKey).(*Panic); ok && info.Type != "" {
		return info
	}

	return nil
}

// withPanicHolder prepares the context so that Recovery can hand the panic it recovered
// to the middleware that mounted it. A context only flows down, hence the pointer.
func withPanicHolder(ctx context.Context) context.Context {
	if _, ok := ctx.Value(panicCtxKey).(*Panic); ok {
		return ctx
	}

	return context.WithValue(ctx, panicCtxKey, &Panic{})
}
