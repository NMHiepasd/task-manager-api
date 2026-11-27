package crypto_test

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"task-manager-api/pkg/crypto"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	require.Len(t, key, crypto.KeySize)

	testCases := []struct {
		name      string
		plaintext string
	}{
		{name: "empty string", plaintext: ""},
		{name: "short string", plaintext: "secret-key-123"},
		{name: "webhook secret", plaintext: "whsec_9f83b2a7c41e05d6a89c2b1e4f7a0c3d9e8b7a6c5d4e3f2a1b0c9d8e7f6a5b4c"},
		{name: "long payload", plaintext: string(make([]byte, 8192))},
		{name: "utf8 characters", plaintext: "Khóa bí mật tiếng Việt có dấu: 🔐 Nguyễn Minh Hiệp"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, err := crypto.EncryptString(tc.plaintext, key)
			require.NoError(t, err)
			require.NotEmpty(t, ciphertext)

			// Decrypt string helper
			decrypted, err := crypto.DecryptString(ciphertext, key)
			require.NoError(t, err)
			assert.Equal(t, tc.plaintext, decrypted)

			// Decrypt bytes helper
			decryptedBytes, err := crypto.Decrypt(ciphertext, key)
			require.NoError(t, err)
			assert.Equal(t, []byte(tc.plaintext), decryptedBytes)
		})
	}
}

func TestEncrypt_NonceRandomness(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	plaintext := "constant-confidential-secret"

	// Encrypting the same plaintext multiple times must produce distinct ciphertexts due to fresh nonces
	ciphertexts := make(map[string]bool)
	for i := 0; i < 50; i++ {
		ct, err := crypto.EncryptString(plaintext, key)
		require.NoError(t, err)
		assert.False(t, ciphertexts[ct], "ciphertext collision detected; nonces must be uniquely generated")
		ciphertexts[ct] = true
	}
}

func TestDecrypt_CorruptedCiphertext(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	plaintext := "sensitive-information-must-verify"
	ciphertext, err := crypto.EncryptString(plaintext, key)
	require.NoError(t, err)

	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	require.NoError(t, err)

	// Corrupt a byte in the middle of ciphertext
	corrupted := make([]byte, len(raw))
	copy(corrupted, raw)
	corrupted[crypto.NonceSize+2] ^= 0xFF

	corruptedBase64 := base64.StdEncoding.EncodeToString(corrupted)
	_, err = crypto.Decrypt(corruptedBase64, key)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crypto.ErrDecryptionFailed))

	// Corrupt a byte in the tag (end of raw buffer)
	corruptedTag := make([]byte, len(raw))
	copy(corruptedTag, raw)
	corruptedTag[len(corruptedTag)-1] ^= 0xFF

	corruptedTagBase64 := base64.StdEncoding.EncodeToString(corruptedTag)
	_, err = crypto.Decrypt(corruptedTagBase64, key)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crypto.ErrDecryptionFailed))
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1, err := crypto.GenerateKey()
	require.NoError(t, err)

	key2, err := crypto.GenerateKey()
	require.NoError(t, err)

	plaintext := "top-secret-payload"
	ciphertext, err := crypto.EncryptString(plaintext, key1)
	require.NoError(t, err)

	_, err = crypto.DecryptString(ciphertext, key2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crypto.ErrDecryptionFailed))
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	// Short base64 payloads under minLen (12-byte nonce + 16-byte tag = 28 bytes)
	shortPayloads := []string{
		"",
		base64.StdEncoding.EncodeToString([]byte("short")),
		base64.StdEncoding.EncodeToString(make([]byte, 27)),
	}

	for _, sp := range shortPayloads {
		_, err := crypto.Decrypt(sp, key)
		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrCiphertextTooShort))
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	_, err = crypto.Decrypt("!!!not-valid-base64???", key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base64 encoding")
}

func TestKeyValidation_InvalidKeySize(t *testing.T) {
	invalidKeys := [][]byte{
		nil,
		{},
		make([]byte, 16), // AES-128 (not allowed, strictly 32 bytes required)
		make([]byte, 24), // AES-192 (not allowed)
		make([]byte, 31),
		make([]byte, 33),
		make([]byte, 64),
	}

	for _, k := range invalidKeys {
		_, err := crypto.Encrypt([]byte("data"), k)
		assert.True(t, errors.Is(err, crypto.ErrInvalidKeySize))

		_, err = crypto.Decrypt("dGVzdA==", k)
		assert.True(t, errors.Is(err, crypto.ErrInvalidKeySize))
	}
}

func TestGenerateKey(t *testing.T) {
	k1, err := crypto.GenerateKey()
	require.NoError(t, err)
	assert.Len(t, k1, crypto.KeySize)

	k2, err := crypto.GenerateKey()
	require.NoError(t, err)
	assert.Len(t, k2, crypto.KeySize)

	assert.NotEqual(t, k1, k2, "consecutive generated keys must be distinct")
}

func TestParseKey(t *testing.T) {
	rawKey := make([]byte, crypto.KeySize)
	_, err := rand.Read(rawKey)
	require.NoError(t, err)

	t.Run("raw 32-byte string", func(t *testing.T) {
		parsed, err := crypto.ParseKey(string(rawKey))
		require.NoError(t, err)
		assert.Equal(t, rawKey, parsed)
	})

	t.Run("hex-encoded 64 characters", func(t *testing.T) {
		hexStr := hex.EncodeToString(rawKey)
		assert.Len(t, hexStr, 64)

		parsed, err := crypto.ParseKey(hexStr)
		require.NoError(t, err)
		assert.Equal(t, rawKey, parsed)
	})

	t.Run("base64-encoded string", func(t *testing.T) {
		b64Str := base64.StdEncoding.EncodeToString(rawKey)

		parsed, err := crypto.ParseKey(b64Str)
		require.NoError(t, err)
		assert.Equal(t, rawKey, parsed)
	})

	t.Run("invalid key formats", func(t *testing.T) {
		invalidInputs := []string{
			"",
			"short",
			"invalid-hex-characters-that-are-64-characters-long-zzzzzzzzzzzz!",
			"short-base64",
			hex.EncodeToString(make([]byte, 15)), // 15 bytes hex (30 chars)
			hex.EncodeToString(make([]byte, 20)), // 20 bytes hex (40 chars)
			string(make([]byte, 31)),             // 31 bytes raw
			string(make([]byte, 33)),             // 33 bytes raw
		}

		for _, input := range invalidInputs {
			_, err := crypto.ParseKey(input)
			assert.True(t, errors.Is(err, crypto.ErrInvalidKeySize), "input of length %d should fail", len(input))
		}
	})
}

func TestHelperStringErrors(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	t.Run("EncryptString invalid key size", func(t *testing.T) {
		_, err := crypto.EncryptString("secret", []byte("short-key"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrInvalidKeySize))
	})

	t.Run("DecryptString invalid key size", func(t *testing.T) {
		_, err := crypto.DecryptString("dGVzdA==", []byte("short-key"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrInvalidKeySize))
	})

	t.Run("DecryptString invalid base64", func(t *testing.T) {
		_, err := crypto.DecryptString("!invalid-base64!", key)
		require.Error(t, err)
	})
}
