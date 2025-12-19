package proxy

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/AlexKimmel/GateLite/internal/routing"
)

func NewHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 60 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// Handler returns a handler that proxies to the upstream specified by the matched route.
func Handler(tr *http.Transport) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt, ok := routing.RouteFrom(r)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"no_route_ctx","message":"route not in context"}}`))
			return
		}

		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = rt.UpUrl.Scheme
				req.URL.Host = rt.UpUrl.Host
				// Forwarded headers
				req.Header.Set("X-Forwarded-Host", req.Host)
				req.Header.Set("X-Forwarded-Proto", "http")
			},
			Transport: tr,
		}
		// per-route timeout
		ctx, cancel := context.WithTimeout(r.Context(), rt.Timeout)
		defer cancel()
		proxy.ServeHTTP(w, r.WithContext(ctx))
	})
}
