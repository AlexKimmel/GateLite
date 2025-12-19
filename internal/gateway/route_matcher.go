package gateway

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexKimmel/GateLite/internal/routing"
)

func RouteMatcher(rr *routing.Router, skip map[string]struct{}) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := skip[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}

			method := r.Method
			path := r.URL.Path

			rt, ok := rr.Match(method, path)
			if !ok {
				// TEMP DEBUG: show why
				var prefixes []string
				for _, x := range rr.Routes() {
					prefixes = append(prefixes, strconv.Quote(x.Prefix))
				}
				log.Printf("RouteMatcher: NO MATCH method=%s path=%q known_prefixes=%s",
					method, path, strings.Join(prefixes, ","),
				)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"code":"no_route","message":"no matching route"}}`))
				return
			}

			next.ServeHTTP(w, routing.WithRoute(r, rt))
		})
	}
}
