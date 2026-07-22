package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testSecret = "test-secret"
	testIssuer = "test-issuer"
)

func TestParseTokenAcceptsValidToken(t *testing.T) {
	tok, err := GenerateToken("client-a", testSecret, testIssuer, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	claims, err := ParseToken(tok, testSecret, testIssuer)
	if err != nil {
		t.Fatalf("expected valid token to parse, got: %v", err)
	}
	if claims.Subject != "client-a" {
		t.Fatalf("expected subject client-a, got %q", claims.Subject)
	}
}

func TestParseTokenRejectsExpiredToken(t *testing.T) {
	tok, err := GenerateToken("client-a", testSecret, testIssuer, -time.Minute)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	if _, err := ParseToken(tok, testSecret, testIssuer); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestParseTokenRejectsMissingExpiration(t *testing.T) {
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "client-a", Issuer: testIssuer}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("unexpected error signing token: %v", err)
	}
	if _, err := ParseToken(signed, testSecret, testIssuer); err == nil {
		t.Fatal("expected token without an exp claim to be rejected")
	}
}

func TestParseTokenRejectsWrongSecret(t *testing.T) {
	tok, err := GenerateToken("client-a", testSecret, testIssuer, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	if _, err := ParseToken(tok, "wrong-secret", testIssuer); err == nil {
		t.Fatal("expected token signed with a different secret to be rejected")
	}
}

func TestParseTokenRejectsWrongIssuer(t *testing.T) {
	tok, err := GenerateToken("client-a", testSecret, testIssuer, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	if _, err := ParseToken(tok, testSecret, "other-issuer"); err == nil {
		t.Fatal("expected token with mismatched issuer to be rejected")
	}
}

func TestSubjectFromContextDefaultsToAnonymous(t *testing.T) {
	if got := SubjectFromContext(context.Background()); got != "anonymous" {
		t.Fatalf("expected anonymous fallback, got %q", got)
	}
}
