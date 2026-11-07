package token_test

import (
	"task-manager-api/pkg/token"
	"testing"
	"time"

	"github.com/google/uuid"
)

func BenchmarkJWTMaker_CreateToken(b *testing.B) {
	maker, err := token.NewJWTMaker("12345678901234567890123456789012")
	if err != nil {
		b.Fatalf("failed to create jwt maker: %v", err)
	}

	userID := uuid.New()
	email := "bench@example.com"
	role := "user"
	duration := 15 * time.Minute

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = maker.CreateAccessToken(userID, email, role, duration)
	}
}

func BenchmarkJWTMaker_VerifyToken(b *testing.B) {
	maker, err := token.NewJWTMaker("12345678901234567890123456789012")
	if err != nil {
		b.Fatalf("failed to create jwt maker: %v", err)
	}

	userID := uuid.New()
	email := "bench@example.com"
	role := "user"
	duration := 15 * time.Minute

	tokenStr, _, err := maker.CreateAccessToken(userID, email, role, duration)
	if err != nil {
		b.Fatalf("failed to generate token: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = maker.VerifyToken(tokenStr)
	}
}
