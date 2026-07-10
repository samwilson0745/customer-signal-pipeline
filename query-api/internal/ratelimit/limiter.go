// Package ratelimit implements a per-client token bucket rate limiter
// backed by Redis, so limits are shared correctly across multiple Query API
// replicas rather than kept in local process memory.
package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// tokenBucketScript atomically refills and (maybe) debits a bucket stored as
// a Redis hash: {tokens, ts}. Refill is lazy - computed from elapsed time
// since the last request rather than a background job.
//
// KEYS[1] = bucket key
// ARGV[1] = capacity (max tokens / burst size)
// ARGV[2] = refill rate (tokens per second)
// ARGV[3] = now (unix seconds, float)
// ARGV[4] = requested tokens (normally 1)
// ARGV[5] = key TTL in seconds (cleanup for idle clients)
//
// Returns 1 if the request was allowed, 0 if it was rejected.
const tokenBucketScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local ttl = tonumber(ARGV[5])

local bucket = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(bucket[1])
local ts = tonumber(bucket[2])

if tokens == nil then
  tokens = capacity
  ts = now
end

local elapsed = math.max(0, now - ts)
tokens = math.min(capacity, tokens + elapsed * rate)

local allowed = 0
if tokens >= requested then
  tokens = tokens - requested
  allowed = 1
end

redis.call('HMSET', key, 'tokens', tokens, 'ts', now)
redis.call('EXPIRE', key, ttl)

return allowed
`

type Limiter struct {
	client     *redis.Client
	capacity   float64
	ratePerSec float64
	script     *redis.Script
	now        func() time.Time
}

// New builds a limiter allowing `limitPerMinute` requests/minute per client,
// with a burst capacity equal to that same per-minute limit.
func New(client *redis.Client, limitPerMinute int) *Limiter {
	if limitPerMinute < 1 {
		limitPerMinute = 1
	}
	return &Limiter{
		client:     client,
		capacity:   float64(limitPerMinute),
		ratePerSec: float64(limitPerMinute) / 60.0,
		script:     redis.NewScript(tokenBucketScript),
		now:        time.Now,
	}
}

// Allow reports whether the client identified by key may make a request
// right now, consuming one token if so.
func (l *Limiter) Allow(ctx context.Context, clientKey string) (bool, error) {
	key := "ratelimit:" + clientKey
	nowSeconds := float64(l.now().UnixNano()) / 1e9
	res, err := l.script.Run(ctx, l.client, []string{key}, l.capacity, l.ratePerSec, nowSeconds, 1, 3600).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}
