package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"customer-signal-pipeline/query-api/internal/auth"
	"customer-signal-pipeline/query-api/internal/circuitbreaker"
	"customer-signal-pipeline/query-api/internal/ratelimit"
	"customer-signal-pipeline/query-api/internal/search"
	"customer-signal-pipeline/query-api/internal/stats"
)

type Handler struct {
	Search       *search.Client
	Stats        *stats.Client
	Limiter      *ratelimit.Limiter
	ESBreaker    *circuitbreaker.Breaker
	RedisBreaker *circuitbreaker.Breaker
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func logRequest(r *http.Request, status int, start time.Time) {
	log.Printf(`{"level":"info","service":"query-api","method":"%s","path":"%s","status":%d,"duration_ms":%d}`,
		r.Method, r.URL.Path, status, time.Since(start).Milliseconds())
}

// RateLimitMiddleware enforces the per-client token bucket. It runs after
// auth so the bucket is keyed by JWT subject rather than raw IP.
func (h *Handler) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := auth.SubjectFromContext(r.Context())
		allowed, err := h.Limiter.Allow(r.Context(), client)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rate limiter unavailable"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) SearchHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	q := search.Query{
		Text:      r.URL.Query().Get("q"),
		Brand:     r.URL.Query().Get("brand"),
		Sentiment: r.URL.Query().Get("sentiment"),
		Urgency:   r.URL.Query().Get("urgency"),
	}

	var hits []search.Hit
	err := h.ESBreaker.Execute(func() error {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		res, err := h.Search.Search(ctx, q)
		if err != nil {
			return err
		}
		hits = res
		return nil
	})

	if err == circuitbreaker.ErrOpen {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "search temporarily unavailable (circuit open)"})
		logRequest(r, http.StatusServiceUnavailable, start)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "search backend error"})
		logRequest(r, http.StatusBadGateway, start)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"results": hits, "count": len(hits)})
	logRequest(r, http.StatusOK, start)
}

func (h *Handler) StatsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	brand := r.URL.Query().Get("brand")
	if brand == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "brand query parameter is required"})
		logRequest(r, http.StatusBadRequest, start)
		return
	}

	var breakdown *stats.Breakdown
	err := h.RedisBreaker.Execute(func() error {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		res, err := h.Stats.GetBreakdown(ctx, brand)
		if err != nil {
			return err
		}
		breakdown = res
		return nil
	})

	if err == circuitbreaker.ErrOpen {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "stats temporarily unavailable (circuit open)"})
		logRequest(r, http.StatusServiceUnavailable, start)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stats backend error"})
		logRequest(r, http.StatusBadGateway, start)
		return
	}

	writeJSON(w, http.StatusOK, breakdown)
	logRequest(r, http.StatusOK, start)
}

func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	esErr := h.ESBreaker.Execute(func() error { return h.Search.Ping(ctx) })
	redisErr := h.RedisBreaker.Execute(func() error { return h.Stats.Ping(ctx) })

	status := http.StatusOK
	body := map[string]interface{}{
		"elasticsearch": esErr == nil,
		"redis":         redisErr == nil,
		"es_breaker":    h.ESBreaker.State().String(),
		"redis_breaker": h.RedisBreaker.State().String(),
	}
	if esErr != nil || redisErr != nil {
		status = http.StatusServiceUnavailable
		body["status"] = "degraded"
	} else {
		body["status"] = "ok"
	}

	writeJSON(w, status, body)
	logRequest(r, status, start)
}
