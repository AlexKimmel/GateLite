package memory

import (
	"context"
	"sync"
	"time"

	"github.com/AlexKimmel/GateLite/internal/ratelimit"
)

type bucket struct {
	mu         sync.Mutex
	token      float64
	lastRefill time.Time
}

type Limiter struct {
	now    func() time.Time
	bucket sync.Map
}

func New() *Limiter {
	return &Limiter{
		now: time.Now,
	}
}

func (l *Limiter) Close() error { return nil }

func (l *Limiter) Allow(_ context.Context, key string, p ratelimit.Policy, now time.Time) (ratelimit.Decision, error) {
	if p.RPM <= 0 || p.Burst <= 0 {
		return ratelimit.Decision{Allowed: true, Limit: 60, Remaining: 60, ResetUnixSec: 0}, nil
	}

	refillPerSec := float64(p.RPM) / 60
	capacity := float64(p.Burst)

	// create bucket
	v, _ := l.bucket.LoadOrStore(key, &bucket{
		token:      capacity,
		lastRefill: now,
	})

	b := v.(*bucket)

	b.mu.Lock()
	defer b.mu.Unlock()

	// refill tokens
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.token += elapsed * refillPerSec
	if b.token > capacity {
		b.token = capacity
	}
	b.lastRefill = now

	// decide
	allow := b.token >= 1.0
	if allow {
		b.token -= 1.0
	}

	// estimate reset time (to full)
	var resetSec int64
	if b.token >= capacity {
		resetSec = now.Unix()
	} else {
		need := capacity - b.token
		sec := need / refillPerSec
		resetSec = now.Add(time.Duration(sec * float64(time.Second))).Unix()
	}

	return ratelimit.Decision{
		Allowed:      allow,
		Limit:        p.RPM,
		Remaining:    int(b.token),
		ResetUnixSec: resetSec,
	}, nil
}
