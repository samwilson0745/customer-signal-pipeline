// gen-token is a small local dev/test helper that issues a JWT bearer token
// signed with JWT_SECRET, since this project intentionally has no user
// store or login flow -- the Query API only validates bearer tokens.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"customer-signal-pipeline/query-api/internal/auth"
)

func main() {
	subject := flag.String("sub", "demo-client", "token subject / client id")
	secret := flag.String("secret", os.Getenv("JWT_SECRET"), "HMAC signing secret (defaults to $JWT_SECRET)")
	issuer := flag.String("issuer", envOrDefault("JWT_ISSUER", "customer-signal-pipeline"), "token issuer")
	ttl := flag.Duration("ttl", 24*time.Hour, "token lifetime")
	flag.Parse()

	if *secret == "" {
		log.Fatal("a signing secret is required: pass -secret or set JWT_SECRET")
	}

	token, err := auth.GenerateToken(*subject, *secret, *issuer, *ttl)
	if err != nil {
		log.Fatalf("failed to generate token: %v", err)
	}
	fmt.Println(token)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
