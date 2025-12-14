package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey int

const keyID ctxKey = 0

// Store is a static in-memory key store: secret -> keyID
type Store struct {
	header   string
	bySecret map[string]string
}

// NewStatic creates a new static key store.
// header: HTTP header to read the key from (e.g., "X-API-Key")
// pairs: map of secret -> keyID
func NewStatic(header string, pairs map[string]string) *Store {
	h := header
	if h == "" {
		h = "X-API-Key"
	}
	return &Store{header: h, bySecret: pairs}
}

func (s *Store) keyIDFor(secret string) (string, bool) {
	id, ok := s.bySecret[secret]
	return id, ok
}

// WithKeyID injects the key ID into context.
func WithKeyID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyID, id)
}

// KeyIDFrom extracts the key ID from context (if present).
func KeyIDFrom(ctx context.Context) (string, bool) {
	v := ctx.Value(keyID)
	if v == nil {
		return "", false
	}
	id, ok := v.(string)
	return id, ok
}

// Middleware validates the API key and writes JSON errors on failure.
// It skips authentication for any path in skipPaths.
func (s *Store) Middleware(skipPaths map[string]struct{}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		hname := s.header

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := skipPaths[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}

			secret := strings.TrimSpace(r.Header.Get(hname))
			if secret == "" {
				writeJSON(w, http.StatusUnauthorized, "missing_api_key", "Provide API key in "+hname)
				return
			}
			id, ok := s.keyIDFor(secret)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, "invalid_api_key", "API key not recognized")
				return
			}
			ctx := WithKeyID(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeJSON(w http.ResponseWriter, code int, errCode, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"error":{"code":"` + errCode + `","message":"` + msg + `"}}`))
}
