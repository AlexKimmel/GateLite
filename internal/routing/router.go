package routing

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Route struct {
	ID      string
	Methods map[string]struct{}
	Prefix  string
	UpUrl   *url.URL
	Timeout time.Duration
}

type Router struct {
	routes []*Route
}

func New() *Router {
	return &Router{}
}

func (r *Router) Add(rt *Route) {
	r.routes = append(r.routes, rt)
}

func (r *Router) Routes() []*Route {
	return r.routes
}
func (r *Router) Match( method string, path string) (*Route, bool) {
	log.Printf("Router.Match called method=%s path=%q routes=%d", method, path, len(r.routes))
	m := strings.ToUpper(method)
	for _, rt := range r.routes {
		if _, ok := rt.Methods[m]; !ok {
			continue
		}
		// Normalize the route prefix once per match attempt
		prefix := strings.TrimSpace(rt.Prefix)
		prefix = strings.TrimSuffix(rt.Prefix, "/")
		if prefix == "" {
			prefix = "/"
		}

		// Match path prefix
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return rt, true
		}
	}
	return nil, false
}

// --- context helpers ---
type ctxKey int

const keyRoute ctxKey = 0

func WithRoute(r *http.Request, rt *Route) *http.Request {
	ctx := context.WithValue(r.Context(), keyRoute, rt)
	return r.WithContext(ctx)
}

func RouteFrom(r *http.Request) (*Route, bool) {
	v := r.Context().Value(keyRoute)
	if v == nil {
		return nil, false
	}
	rt, ok := v.(*Route)
	return rt, ok
}
