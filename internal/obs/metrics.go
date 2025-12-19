package obs

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AlexKimmel/GateLite/internal/gateway"
	"github.com/AlexKimmel/GateLite/internal/routing"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	RateLimited     *prometheus.CounterVec
	LimiterErrors   *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gatelite_requests_total",
				Help: "Total HTTP requests processed by the gateway",
			},
			[]string{"route", "method", "code"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gatelite_request_duration_seconds",
				Help:    "Request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"route", "method"},
		),
		RateLimited: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gatelite_rate_limited_total",
				Help: "Total requests rejected due to rate limiting",
			},
			[]string{"route"},
		),
		LimiterErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gatelite_limiter_errors_total",
				Help: "Total rate limiter errors",
			},
			[]string{"route"},
		),
	}

	reg.MustRegister(m.RequestsTotal, m.RequestDuration, m.RateLimited, m.LimiterErrors)
	return m
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Middleware records per-request metrics.
// It uses the route stored by RouteMatcher (gateway.RouteFrom).
func (m *Metrics) Middleware(skip map[string]struct{}) gateway.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := skip[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			route := "unknown"
			if rt, ok := routing.RouteFrom(r); ok && rt != nil && rt.ID != "" {
				route = rt.ID
			}

			method := r.Method
			code := rec.status
			if code == 0 {
				code = http.StatusOK
			}

			m.RequestDuration.WithLabelValues(route, method).Observe(time.Since(start).Seconds())
			m.RequestsTotal.WithLabelValues(route, method, strconv.Itoa(code)).Inc()
		})
	}
}
