package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	// KeySize is the expected key size in bytes for AES-256 (32 bytes = 256 bits).
	KeySize = 32

	// NonceSize is the standard GCM nonce size in bytes (12 bytes = 96 bits).
	NonceSize = 12
)

var (
	// ErrInvalidKeySize indicates that the provided AES key is not 32 bytes.
	ErrInvalidKeySize = errors.New("crypto: key size must be exactly 32 bytes for AES-256")

	// ErrCiphertextTooShort indicates that the ciphertext payload is shorter than the minimum nonce + tag length.
	ErrCiphertextTooShort = errors.New("crypto: ciphertext payload is too short")

	// ErrDecryptionFailed indicates that authentication failed due to corrupted data or incorrect key.
	ErrDecryptionFailed = errors.New("crypto: authentication tag mismatch or corrupted ciphertext")
)

// Encrypt encrypts plaintext using AES-256-GCM with a randomly generated 12-byte nonce.
// The returned string is Base64-encoded with the structure: [12-byte nonce][ciphertext][16-byte authentication tag].
func Encrypt(plaintext []byte, key []byte) (string, error) {
	if len(key) != KeySize {
		return "", ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to initialize cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to initialize GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: failed to generate random nonce: %w", err)
	}

	// Seal appends ciphertext + authentication tag to the provided nonce buffer
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decodes a Base64-encoded payload, extracts the 12-byte nonce, and decrypts/verifies
// the ciphertext using AES-256-GCM. Returns ErrDecryptionFailed if the tag does not verify.
func Decrypt(ciphertextBase64 string, key []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	raw, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid base64 encoding: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to initialize cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to initialize GCM: %w", err)
	}

	minLen := gcm.NonceSize() + gcm.Overhead()
	if len(raw) < minLen {
		return nil, ErrCiphertextTooShort
	}

	nonce := raw[:gcm.NonceSize()]
	cipherAndTag := raw[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, cipherAndTag, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	if plaintext == nil {
		plaintext = []byte{}
	}

	return plaintext, nil
}

// EncryptString is a helper that encrypts a UTF-8 string and returns the Base64 ciphertext.
func EncryptString(plaintext string, key []byte) (string, error) {
	return Encrypt([]byte(plaintext), key)
}

// DecryptString is a helper that decrypts a Base64 ciphertext and returns the UTF-8 plaintext string.
func DecryptString(ciphertextBase64 string, key []byte) (string, error) {
	bytes, err := Decrypt(ciphertextBase64, key)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// GenerateKey generates a cryptographically secure 32-byte key for AES-256.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("crypto: failed to generate secure random key: %w", err)
	}
	return key, nil
}

// ParseKey parses an encryption key provided as:
// 1. 64-character hexadecimal string
// 2. 44-character Base64-encoded string
// 3. Raw 32-byte string
// Returns a validated 32-byte key slice.
func ParseKey(encoded string) ([]byte, error) {
	// Hex-encoded 64 hex characters (32 bytes = 256 bits)
	if len(encoded) == 64 {
		decoded, err := hex.DecodeString(encoded)
		if err == nil && len(decoded) == KeySize {
			return decoded, nil
		}
	}

	// Base64-encoded string (typically 44 chars for 32 bytes with padding)
	if len(encoded) >= 42 && len(encoded) <= 45 {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil && len(decoded) == KeySize {
			return decoded, nil
		}
	}

	// Direct 32-byte raw string
	raw := []byte(encoded)
	if len(raw) == KeySize {
		return raw, nil
	}

	return nil, ErrInvalidKeySize
}
