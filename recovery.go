package sloghttp

import (
	"fmt"
	"net/http"
	"runtime"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				buf := make([]byte, 2048)
				n := runtime.Stack(buf, false)
				buf = buf[:n]

				fmt.Printf("panic recovered: %v\n %s", err, buf)
				w.WriteHeader(500)
				_, _ = w.Write([]byte{})
			}
		}()

		next.ServeHTTP(w, r)
	})
}
