package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"customer-signal-pipeline/query-api/internal/auth"
	"customer-signal-pipeline/query-api/internal/circuitbreaker"
	"customer-signal-pipeline/query-api/internal/config"
	"customer-signal-pipeline/query-api/internal/handlers"
	"customer-signal-pipeline/query-api/internal/middleware"
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
	mux.Handle("/search", middleware.CORS(auth.Middleware(cfg.JWTSecret, cfg.JWTIssuer)(h.RateLimitMiddleware(http.HandlerFunc(h.SearchHandler)))))
	mux.Handle("/stats", middleware.CORS(auth.Middleware(cfg.JWTSecret, cfg.JWTIssuer)(h.RateLimitMiddleware(http.HandlerFunc(h.StatsHandler)))))
	mux.Handle("/health", middleware.CORS(http.HandlerFunc(h.HealthHandler)))

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf(`{"level":"info","service":"query-api","msg":"listening","addr":"%s","es_url":"%s","rate_limit_per_minute":%d}`,
			addr, cfg.ESURL, cfg.RateLimitPerMinute)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := <-shutdown
	log.Printf(`{"level":"info","service":"query-api","msg":"shutting down","signal":"%s"}`, sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
	log.Printf(`{"level":"info","service":"query-api","msg":"stopped"}`)
}
