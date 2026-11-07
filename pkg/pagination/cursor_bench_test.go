package pagination_test

import (
	"task-manager-api/pkg/pagination"
	"testing"
	"time"

	"github.com/google/uuid"
)

func BenchmarkEncodeCursor(b *testing.B) {
	now := time.Now().UTC()
	id := uuid.New()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = pagination.EncodeCursor(now, id)
	}
}

func BenchmarkDecodeCursor(b *testing.B) {
	now := time.Now().UTC()
	id := uuid.New()
	encoded := pagination.EncodeCursor(now, id)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = pagination.DecodeCursor(encoded)
	}
}
