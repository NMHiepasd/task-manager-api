package pagination_test

import (
	"task-manager-api/pkg/pagination"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursor_EncodeAndDecode(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	id := uuid.New()

	encoded := pagination.EncodeCursor(now, id)
	assert.NotEmpty(t, encoded)

	decodedTime, decodedID, err := pagination.DecodeCursor(encoded)
	require.NoError(t, err)
	assert.True(t, now.Equal(decodedTime))
	assert.Equal(t, id, decodedID)
}

func TestCursor_Empty(t *testing.T) {
	ts, id, err := pagination.DecodeCursor("")
	require.NoError(t, err)
	assert.True(t, ts.IsZero())
	assert.Equal(t, uuid.Nil, id)
}

func TestCursor_InvalidFormats(t *testing.T) {
	testCases := []string{
		"invalid-base64!@#",
		"bm90LWEtdmFsaWQtc3BsaXQ",                    // "not-a-valid-split"
		"MjAyNi0xMS0wNlQxNjoyMzo1N1p8bm90LWEtdXVpZA", // valid time | not-a-uuid
	}

	for _, tc := range testCases {
		_, _, err := pagination.DecodeCursor(tc)
		assert.ErrorIs(t, err, pagination.ErrInvalidCursor)
	}
}
