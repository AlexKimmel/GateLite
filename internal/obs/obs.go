package obs

import (
	"context"

	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

type ctxKey int

const keyRID ctxKey = 0

func SetupLogger(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339Nano

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger().Level(lvl)

	return logger
}

// RequestID middleware: uses X-Request-ID if present, else generates one.
func RequestID() func(http.Handler) http.Handler {
	return hlog.RequestIDHandler("req_id", "X-Request-ID")
}

// Logger returns a middleware that logs per-request with duration and status.
func Logger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		h := hlog.NewHandler(logger)(
			hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
				hlog.FromRequest(r).Info().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("remote", r.RemoteAddr).
					Int("status", status).
					Int("size", size).
					Dur("dur", duration).
					Msg("req")
			})(
				hlog.UserAgentHandler("ua")(
					hlog.RefererHandler("referer")(
						hlog.RequestIDHandler("req_id", "X-Request-ID")(next),
					),
				),
			),
		)
		return h
	}
}
func WithReqID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRID, id)
}
func ReqIDFrom(ctx context.Context) (string, bool) {
	v := ctx.Value(keyRID)
	if v == nil {

		return "", false
	}

	s, ok := v.(string)
	return s, ok
}
