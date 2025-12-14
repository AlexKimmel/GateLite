package gateway

import "net/http"

func BodyLimit(maxBytes int) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes > 0 && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
			}
			next.ServeHTTP(w, r)
		})
	}
}
