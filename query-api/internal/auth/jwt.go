package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const subjectContextKey contextKey = "auth_subject"

var (
	ErrMissingToken = errors.New("missing bearer token")
	ErrInvalidToken = errors.New("invalid or expired token")
)

type Claims struct {
	jwt.RegisteredClaims
}

// GenerateToken issues a JWT for subject, signed with secret and stamped
// with issuer. Used by the standalone gen-token CLI for local testing since
// this service has no user store of its own.
func GenerateToken(subject, secret, issuer string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseToken(tokenString, secret, issuer string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	}, jwt.WithIssuer(issuer))
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// Middleware validates a Bearer JWT on every request, writing 401 on
// failure and otherwise storing the token subject in the request context
// (used downstream to key the per-client rate limiter).
func Middleware(secret, issuer string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, ErrMissingToken.Error(), http.StatusUnauthorized)
				return
			}
			tokenString := strings.TrimPrefix(header, "Bearer ")
			claims, err := ParseToken(tokenString, secret, issuer)
			if err != nil {
				http.Error(w, ErrInvalidToken.Error(), http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), subjectContextKey, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SubjectFromContext returns the authenticated client identifier, falling
// back to "anonymous" if the middleware wasn't applied (shouldn't happen on
// protected routes).
func SubjectFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(subjectContextKey).(string); ok && v != "" {
		return v
	}
	return "anonymous"
}
