package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port string

	RedisURL string
	ESURL    string
	ESIndex  string

	JWTSecret string
	JWTIssuer string

	RateLimitPerMinute int

	CBFailureThreshold int
	CBCooldown         time.Duration
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func Load() Config {
	return Config{
		Port:               getEnv("QUERY_API_PORT", "8080"),
		RedisURL:           getEnv("REDIS_URL", "redis://localhost:6379/0"),
		ESURL:              getEnv("ES_URL", "http://localhost:9200"),
		ESIndex:            getEnv("ES_INDEX", "customer-signals"),
		JWTSecret:          getEnv("JWT_SECRET", "change-me-in-production"),
		JWTIssuer:          getEnv("JWT_ISSUER", "customer-signal-pipeline"),
		RateLimitPerMinute: getEnvInt("RATE_LIMIT_PER_MINUTE", 60),
		CBFailureThreshold: getEnvInt("CB_FAILURE_THRESHOLD", 5),
		CBCooldown:         time.Duration(getEnvInt("CB_COOLDOWN_SECONDS", 30)) * time.Second,
	}
}
