package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"task-manager-api/pkg/webhook"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignV2(t *testing.T) {
	//nolint:gosec // G101: Test dummy secret key
	secret := "whsec_test_secret_32_characters_minimum_len!"
	ts := "1700000000"
	id := "del_12345678-1234-1234-1234-1234567890ab"
	body := []byte(`{"event":"task.created","task_id":"abc"}`)

	// Manual HMAC computation for verification
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + id + "."))
	mac.Write(body)
	expectedHex := hex.EncodeToString(mac.Sum(nil))

	sig := webhook.SignV2(secret, ts, id, body)
	assert.Equal(t, expectedHex, sig)
}

func TestVerifyV2(t *testing.T) {
	//nolint:gosec // G101: Test dummy secret key
	secret := "whsec_test_secret_32_characters_minimum_len!"
	id := "del_12345678-1234-1234-1234-1234567890ab"
	body := []byte(`{"event":"task.created","task_id":"abc"}`)
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	ts := strconv.FormatInt(now.Unix(), 10)
	tolerance := 5 * time.Minute

	sigHex := webhook.SignV2(secret, ts, id, body)

	t.Run("valid signature with raw hex", func(t *testing.T) {
		err := webhook.VerifyV2(secret, ts, id, body, sigHex, now, tolerance)
		require.NoError(t, err)
	})

	t.Run("valid signature with v1= prefix", func(t *testing.T) {
		err := webhook.VerifyV2(secret, ts, id, body, "v1="+sigHex, now, tolerance)
		require.NoError(t, err)
	})

	t.Run("valid signature with RFC3339 timestamp", func(t *testing.T) {
		rfcTs := now.Format(time.RFC3339)
		rfcSig := webhook.SignV2(secret, rfcTs, id, body)
		err := webhook.VerifyV2(secret, rfcTs, id, body, "v1="+rfcSig, now, tolerance)
		require.NoError(t, err)
	})

	t.Run("body tampered", func(t *testing.T) {
		tamperedBody := []byte(`{"event":"task.created","task_id":"hacked"}`)
		err := webhook.VerifyV2(secret, ts, id, tamperedBody, "v1="+sigHex, now, tolerance)
		require.Error(t, err)
		assert.True(t, errors.Is(err, webhook.ErrSignatureMismatch))
	})

	t.Run("id changed", func(t *testing.T) {
		otherID := "del_different_id_here"
		err := webhook.VerifyV2(secret, ts, otherID, body, "v1="+sigHex, now, tolerance)
		require.Error(t, err)
		assert.True(t, errors.Is(err, webhook.ErrSignatureMismatch))
	})

	t.Run("secret wrong", func(t *testing.T) {
		//nolint:gosec // G101: Test dummy secret key
		wrongSecret := "whsec_wrong_secret_32_characters_minimum_len"
		err := webhook.VerifyV2(wrongSecret, ts, id, body, "v1="+sigHex, now, tolerance)
		require.Error(t, err)
		assert.True(t, errors.Is(err, webhook.ErrSignatureMismatch))
	})

	t.Run("timestamp expired in past", func(t *testing.T) {
		pastTime := now.Add(-10 * time.Minute)
		pastTs := strconv.FormatInt(pastTime.Unix(), 10)
		pastSig := webhook.SignV2(secret, pastTs, id, body)

		err := webhook.VerifyV2(secret, pastTs, id, body, "v1="+pastSig, now, tolerance)
		require.Error(t, err)
		assert.True(t, errors.Is(err, webhook.ErrTimestampExpired))
	})

	t.Run("timestamp in future beyond tolerance", func(t *testing.T) {
		futureTime := now.Add(10 * time.Minute)
		futureTs := strconv.FormatInt(futureTime.Unix(), 10)
		futureSig := webhook.SignV2(secret, futureTs, id, body)

		err := webhook.VerifyV2(secret, futureTs, id, body, "v1="+futureSig, now, tolerance)
		require.Error(t, err)
		assert.True(t, errors.Is(err, webhook.ErrTimestampExpired))
	})

	t.Run("signature invalid hex format", func(t *testing.T) {
		err := webhook.VerifyV2(secret, ts, id, body, "v1=not-valid-hex-at-all!!", now, tolerance)
		require.Error(t, err)
		assert.True(t, errors.Is(err, webhook.ErrInvalidSignatureFormat) || errors.Is(err, webhook.ErrSignatureMismatch))
	})

	t.Run("invalid timestamp string", func(t *testing.T) {
		err := webhook.VerifyV2(secret, "invalid-timestamp-string", id, body, "v1="+sigHex, now, tolerance)
		require.Error(t, err)
	})
}
