package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AlexKimmel/GateLite/internal/auth"
	"github.com/AlexKimmel/GateLite/internal/config"
	"github.com/AlexKimmel/GateLite/internal/gateway"
	"github.com/AlexKimmel/GateLite/internal/obs"
	"github.com/AlexKimmel/GateLite/internal/ratelimit"
	"github.com/AlexKimmel/GateLite/internal/ratelimit/memory"
)

func main() {

	cfg, err := config.Load("./config.yaml")

	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger := obs.SetupLogger(cfg.Observability.LogLevel)
	logger.Info().Msg("Setup logger")

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("v.0.0.1"))
	})

	pairs := map[string]string{} // secret -> keyID
	for _, k := range cfg.Auth.Keys {
		if k.Secret != "" && k.ID != "" {
			pairs[k.Secret] = k.ID
		}
	}
	authStore := auth.NewStatic(cfg.Auth.Header, pairs)

	// limiter + policy
	memLimiter := memory.New()
	policy := ratelimit.Policy{
		RPM:   cfg.Limits.Default.RequestsPerMinute,
		Burst: cfg.Limits.Default.Burst,
	}
	skip := map[string]struct{}{
		"/health":  {},
		"/version": {},
	}

	handler := gateway.Chain(
		mux,
		obs.Logger(logger),
		gateway.BodyLimit(int(cfg.Server.MaxBody())),
		authStore.Middleware(skip),
		gateway.RateLimit(memLimiter, policy, skip),
	)

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
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
