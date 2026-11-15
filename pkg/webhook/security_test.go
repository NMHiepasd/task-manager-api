package webhook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEvents(t *testing.T) {
	t.Run("ValidEvents", func(t *testing.T) {
		err := ValidateEvents([]string{"task.created", "task.deleted", "comment.created"})
		assert.NoError(t, err)
	})

	t.Run("WildcardEvent", func(t *testing.T) {
		err := ValidateEvents([]string{"*"})
		assert.NoError(t, err)
	})

	t.Run("EmptyEventsList", func(t *testing.T) {
		err := ValidateEvents([]string{})
		assert.Error(t, err)
	})

	t.Run("InvalidEventName", func(t *testing.T) {
		err := ValidateEvents([]string{"task.created", "user.hacked"})
		assert.Error(t, err)
	})
}

func TestValidateURL(t *testing.T) {
	t.Run("ValidPublicHTTPS", func(t *testing.T) {
		// When allowLocal is false, example.com resolves to public IP
		err := ValidateURL("https://example.com/webhook", false)
		assert.NoError(t, err)
	})

	t.Run("InvalidScheme", func(t *testing.T) {
		err := ValidateURL("ftp://example.com/webhook", false)
		assert.Error(t, err)
	})

	t.Run("LocalhostBlockedWhenAllowLocalFalse", func(t *testing.T) {
		err := ValidateURL("http://localhost:8080/hook", false)
		assert.Error(t, err)
	})

	t.Run("LocalhostAllowedWhenAllowLocalTrue", func(t *testing.T) {
		err := ValidateURL("http://localhost:8080/hook", true)
		assert.NoError(t, err)
	})

	t.Run("LoopbackIPBlocked", func(t *testing.T) {
		err := ValidateURL("http://127.0.0.1:3000/hook", false)
		assert.Error(t, err)
	})
}

func TestHMACSignature(t *testing.T) {
	secret := "super-secret-key-12345"
	payload := []byte(`{"event":"task.created","task_id":"123"}`)

	sig := ComputeSignature(secret, payload)
	require.NotEmpty(t, sig)

	assert.True(t, VerifySignature(secret, payload, sig))
	assert.False(t, VerifySignature("wrong-secret", payload, sig))
	assert.False(t, VerifySignature(secret, []byte(`{"tampered":true}`), sig))
	assert.False(t, VerifySignature(secret, payload, "invalid-hex-non-hex!"))
}
