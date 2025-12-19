package gateway

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AlexKimmel/GateLite/internal/auth"
	"github.com/AlexKimmel/GateLite/internal/ratelimit"
	"github.com/AlexKimmel/GateLite/internal/routing"
)

func RateLimit(
	lim ratelimit.Limiter,
	policy ratelimit.Policy,
	skipPaths map[string]struct{},
	onLimited func(routeID string),
	onError func(routeID string),
) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// allow ops endpoints without limits
			if _, ok := skipPaths[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}

			// auth key id
			keyID, ok := auth.KeyIDFrom(r.Context())
			if !ok || keyID == "" {
				keyID = "anon"
			}

			now := time.Now()

			// route (from gateway context)
			rt, _ := routing.RouteFrom(r)

			routeID := "unknown"
			if rt != nil && rt.ID != "" {
				routeID = rt.ID
			}

			// limiter key = routeID:keyID (per-route per-key)
			limKey := keyID
			if rt != nil && rt.ID != "" {
				limKey = rt.ID + ":" + keyID
			}

			// choose policy: start with global fallback
			p := policy

			// override from route default if present (>0)
			if rt != nil && rt.LimitDefaultRPM > 0 && rt.LimitDefaultBurst > 0 {
				p = ratelimit.Policy{RPM: rt.LimitDefaultRPM, Burst: rt.LimitDefaultBurst}
			}

			// optional per-key override on this route
			if rt != nil && rt.LimitOverrides != nil {
				if o, ok := rt.LimitOverrides[keyID]; ok && o.RPM > 0 && o.Burst > 0 {
					p = ratelimit.Policy{RPM: o.RPM, Burst: o.Burst}
				}
			}

			dec, err := lim.Allow(r.Context(), limKey, p, now)
			if err != nil {
				if onError != nil {
					onError(routeID)
				}
				writeJSON(w, http.StatusInternalServerError, "rate_limiter_error", "internal rate limiter error")
				return
			}

			// headers for good DX
			if dec.Limit > 0 {
				w.Header().Set("X-RateLimit-Limit", itoa(dec.Limit))
				w.Header().Set("X-RateLimit-Remaining", itoa(max(dec.Remaining, 0)))
				w.Header().Set("X-RateLimit-Reset", itoa64(dec.ResetUnixSec))
			}

			if !dec.Allowed {
				if onLimited != nil {
					onLimited(routeID)
				}
				writeJSON(w, http.StatusTooManyRequests, "rate_limited", "Too many requests")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func itoa(i int) string     { return fmtInt(int64(i)) }
func itoa64(i int64) string { return fmtInt(i) }

func fmtInt(i int64) string {
	var buf [32]byte
	return string(strconv.AppendInt(buf[:0], i, 10))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// local tiny JSON helper to avoid coupling to auth package
func writeJSON(w http.ResponseWriter, code int, errCode, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"error":{"code":"` + errCode + `","message":"` + msg + `"}}`))
}
