package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidCursor indicates the pagination cursor string is corrupt or invalid.
var ErrInvalidCursor = errors.New("invalid pagination cursor format")

// EncodeCursor converts a timestamp and UUID into an opaque, URL-safe base64 string.
func EncodeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s|%s", createdAt.UTC().Format(time.RFC3339Nano), id.String())
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor decodes an opaque cursor string back into its timestamp and UUID components.
func DecodeCursor(encoded string) (time.Time, uuid.UUID, error) {
	if encoded == "" {
		return time.Time{}, uuid.Nil, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	return createdAt, id, nil
}
