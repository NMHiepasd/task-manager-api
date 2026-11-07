package pagination_test

import (
	"task-manager-api/pkg/pagination"
	"testing"
	"time"

	"github.com/google/uuid"
)

func FuzzDecodeCursor(f *testing.F) {
	// Seed initial corpus with realistic and malformed samples
	f.Add("")
	f.Add("invalid")
	f.Add("bm90X2Vub3VnaF9wYXJ0cw==")
	f.Add("invalid_date|not_a_uuid")
	f.Add("MjAyNi0xMS0wNlQxNjo1Mzo1My40NTAwMjVafDAxYTFiMWM3LWYyYWEtNzk5Zi1hY2FmLWMwNDM5OTZlOGE4OQ")
	f.Add(pagination.EncodeCursor(time.Now().UTC(), uuid.New()))

	f.Fuzz(func(t *testing.T, cursorStr string) {
		ts, id, err := pagination.DecodeCursor(cursorStr)
		if err == nil {
			// Invariant: If decoding succeeded without error, re-encoding must succeed and be non-empty
			reEncoded := pagination.EncodeCursor(ts, id)
			if reEncoded == "" && !ts.IsZero() && id != uuid.Nil {
				t.Fatalf("expected non-empty re-encoded cursor, got empty for %v, %v", ts, id)
			}
		}
	})
}
