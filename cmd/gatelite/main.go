package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AlexKimmel/GateLite/internal/auth"
	"github.com/AlexKimmel/GateLite/internal/config"
	"github.com/AlexKimmel/GateLite/internal/gateway"
	"github.com/AlexKimmel/GateLite/internal/obs"
	"github.com/AlexKimmel/GateLite/internal/proxy"
	"github.com/AlexKimmel/GateLite/internal/ratelimit"
	"github.com/AlexKimmel/GateLite/internal/ratelimit/memory"
	"github.com/AlexKimmel/GateLite/internal/routing"
)

func main() {

	// Load config
	cfg, err := config.Load("./config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Logger
	logger := obs.SetupLogger(cfg.Observability.LogLevel)
	logger.Info().Msg("Setup logger")

	// Public endpoints
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("v.0.0.1"))
	})

	rr := routing.New()
	mux.HandleFunc("/debug/match", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		method := r.URL.Query().Get("method")
		if method == "" {
			method = "GET"
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			path = "/v1/echo/hello"
		}

		rt, ok := rr.Match(method, path)
		if ok {
			_, _ = w.Write([]byte("MATCHED\n"))
			_, _ = w.Write([]byte("route.id=" + rt.ID + "\n"))
			_, _ = w.Write([]byte("route.prefix=" + strconv.Quote(rt.Prefix) + "\n"))
			_, _ = w.Write([]byte("route.prefix_len=" + strconv.Itoa(len(rt.Prefix)) + "\n"))
			return
		}

		_, _ = w.Write([]byte("NO MATCH\n"))
		_, _ = w.Write([]byte("method=" + method + "\n"))
		_, _ = w.Write([]byte("path=" + strconv.Quote(path) + "\n"))
		_, _ = w.Write([]byte("path_len=" + strconv.Itoa(len(path)) + "\n"))
	})

	mux.HandleFunc("/debug/router", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		// IMPORTANT: you need a method on Router to expose routes (see below)
		routes := rr.Routes()

		_, _ = w.Write([]byte("router.routes_count=" + strconv.Itoa(len(routes)) + "\n"))
		for i, rt := range routes {
			_, _ = w.Write([]byte(
				"\n[" + strconv.Itoa(i) + "] id=" + rt.ID +
					"\n  prefix=" + strconv.Quote(rt.Prefix) +
					"\n  prefix_len=" + strconv.Itoa(len(rt.Prefix)) +
					"\n  methods_keys=" + keys(rt.Methods) +
					"\n",
			))
		}
	})

	// Build auth store (select -> KeyId)
	pairs := map[string]string{}
	for _, k := range cfg.Auth.Keys {
		if k.Secret != "" && k.ID != "" {
			pairs[k.Secret] = k.ID
		}
	}
	authStore := auth.NewStatic(cfg.Auth.Header, pairs)

	// Build router from cfg.Routers
	// rr := routing.New() moved upwards for debugging
	for _, rc := range cfg.Routes {
		u, err := url.Parse(rc.Upstream.URL)
		if err != nil {
			log.Fatalf("invalid upstream URL for route %s: %v", rc.ID, err)
		}
		methods := map[string]struct{}{}
		for _, m := range rc.Match.Methods {
			methods[strings.ToUpper(m)] = struct{}{}
		}

		timeout := time.Duration(rc.Upstream.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 3 * time.Second
		}
		prefix := strings.TrimSpace(rc.Match.PathPrefix)
		prefix = strings.TrimSuffix(prefix, "/")
		rr.Add(&routing.Route{
			ID:      rc.ID,
			Methods: methods,
			Prefix:  prefix,
			UpUrl:   u,
			Timeout: timeout,
		})
	}

	// Rate limiter + policy
	memLimiter := memory.New()
	policy := ratelimit.Policy{
		RPM:   cfg.Limits.Default.RequestsPerMinute,
		Burst: cfg.Limits.Default.Burst,
	}
	// Skip list for auth/ratelimit/router-matching
	skip := map[string]struct{}{
		"/health":  {},
		"/version": {},
		"/debug/match":   {},
		"/debug/router":  {},
	}

	// Reverse proxy final handler + middleware stack
	tr := proxy.NewHTTPTransport()
	finalProxy := proxy.Handler(tr)

	gatewayStack := gateway.Chain(
		finalProxy,
		obs.Logger(logger), // outermost
		gateway.BodyLimit(int(cfg.Server.MaxBody())),
		gateway.RouteMatcher(rr, skip),
		authStore.Middleware(skip),
		gateway.RateLimit(memLimiter, policy, skip),
	)

	mux.Handle("/", gatewayStack)

	// Server

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.Server.WriteTimeout(),
		IdleTimeout:       cfg.Server.IdleTimeout(),
		ReadTimeout:       cfg.Server.ReadTimeout(),
	}

	// start
	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	log.Printf("bye")
}

// debug helper
func toJSONSlice(xs []string) string {
	if len(xs) == 0 {
		return "[]"
	}
	// minimal JSON string array (no escaping needed for METHODS)
	var b strings.Builder
	b.WriteString("[")
	for i, s := range xs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"` + s + `"`)
	}
	b.WriteString("]")
	return b.String()
}

func keys(m map[string]struct{}) string {
	if len(m) == 0 {
		return "[]"
	}
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return "[" + strings.Join(out, ",") + "]"
}
