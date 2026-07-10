package main

import (
	"log"
	"net/http"

	"github.com/redis/go-redis/v9"

	"customer-signal-pipeline/query-api/internal/auth"
	"customer-signal-pipeline/query-api/internal/circuitbreaker"
	"customer-signal-pipeline/query-api/internal/config"
	"customer-signal-pipeline/query-api/internal/handlers"
	"customer-signal-pipeline/query-api/internal/ratelimit"
	"customer-signal-pipeline/query-api/internal/search"
	"customer-signal-pipeline/query-api/internal/stats"
)

func main() {
	cfg := config.Load()

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("invalid REDIS_URL: %v", err)
	}
	rdb := redis.NewClient(redisOpts)

	esClient, err := search.NewClient(cfg.ESURL, cfg.ESIndex)
	if err != nil {
		log.Fatalf("failed to build elasticsearch client: %v", err)
	}

	h := &handlers.Handler{
		Search:       esClient,
		Stats:        stats.NewClient(rdb),
		Limiter:      ratelimit.New(rdb, cfg.RateLimitPerMinute),
		ESBreaker:    circuitbreaker.New(cfg.CBFailureThreshold, cfg.CBCooldown),
		RedisBreaker: circuitbreaker.New(cfg.CBFailureThreshold, cfg.CBCooldown),
	}

	mux := http.NewServeMux()
	mux.Handle("/search", auth.Middleware(cfg.JWTSecret, cfg.JWTIssuer)(h.RateLimitMiddleware(http.HandlerFunc(h.SearchHandler))))
	mux.Handle("/stats", auth.Middleware(cfg.JWTSecret, cfg.JWTIssuer)(h.RateLimitMiddleware(http.HandlerFunc(h.StatsHandler))))
	mux.HandleFunc("/health", h.HealthHandler)

	addr := ":" + cfg.Port
	log.Printf(`{"level":"info","service":"query-api","msg":"listening","addr":"%s","es_url":"%s","rate_limit_per_minute":%d}`,
		addr, cfg.ESURL, cfg.RateLimitPerMinute)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
