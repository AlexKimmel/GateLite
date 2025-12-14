package ratelimit

import (
	"context"
	"time"
)

type Policy struct {
	RPM   int // requests per minute
	Burst int // bucket capacity
}

type Decision struct {
	Allowed      bool
	Limit        int   // limit per minute
	Remaining    int   // tokens after this request (min 0)
	ResetUnixSec int64 // when tokens would be full if no more traffic
}

type Limiter interface {
	Allow(ctx context.Context, key string, p Policy, now time.Time) (Decision, error)
	Close() error
}