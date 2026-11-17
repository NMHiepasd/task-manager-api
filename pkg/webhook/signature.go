package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrSignatureMismatch indicates that the provided HMAC signature does not match the computed digest.
	ErrSignatureMismatch = errors.New("webhook signature mismatch")

	// ErrTimestampExpired indicates that the webhook timestamp is expired or outside the allowed tolerance window.
	ErrTimestampExpired = errors.New("webhook timestamp is expired or outside tolerance window")

	// ErrInvalidSignatureFormat indicates that the signature string could not be decoded as hex.
	ErrInvalidSignatureFormat = fmt.Errorf("%w: invalid hex signature format", ErrSignatureMismatch)

	// ErrInvalidTimestampFormat indicates that the timestamp string could not be parsed.
	ErrInvalidTimestampFormat = errors.New("invalid webhook timestamp format")
)

// SignV2 calculates the HMAC-SHA256 hex signature for Webhook V2 (replay-resistant).
//
// Payload Format:
//
//	The HMAC-SHA256 is computed over the string concatenation:
//	  "<timestamp>.<id>.<body_bytes>"
//
// Parameters:
//   - secret: The shared pre-shared secret key.
//   - ts: The timestamp string (e.g. Unix seconds "1700000000" or RFC3339).
//   - id: The unique delivery ID (e.g. UUID).
//   - body: The raw JSON/binary HTTP request body payload.
//
// Returns:
//   - The lower-case hexadecimal representation of the HMAC-SHA256 digest.
func SignV2(secret, ts, id string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write([]byte(id))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyV2 validates a Webhook V2 signature and timestamp against replay attacks and tampering.
//
// Validation Steps:
//  1. Timestamp Verification:
//     Parses ts (as integer Unix epoch seconds, or RFC3339 as fallback).
//     Ensures the timestamp is within [-tolerance, +tolerance] of `now`.
//     If the timestamp is older than `now - tolerance` (replay attack) or further ahead than
//     `now + tolerance` (future clock drift / attack), ErrTimestampExpired is returned.
//  2. Signature Format:
//     Strips optional "v1=" prefix from sigHeader.
//     Decodes the hexadecimal digest. Returns ErrInvalidSignatureFormat if decoding fails.
//  3. Constant-Time HMAC Comparison:
//     Re-computes HMAC-SHA256 using SignV2 and compares with the incoming signature using
//     crypto/hmac.Equal (preventing timing side-channel attacks). Returns ErrSignatureMismatch on failure.
func VerifyV2(secret, ts, id string, body []byte, sigHeader string, now time.Time, tolerance time.Duration) error {
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}

	// 1. Parse and validate timestamp
	var tsTime time.Time
	if sec, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64); err == nil {
		tsTime = time.Unix(sec, 0)
	} else if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(ts)); err == nil {
		tsTime = parsed
	} else {
		return ErrInvalidTimestampFormat
	}

	// Check for past expiration
	if now.Sub(tsTime) > tolerance {
		return ErrTimestampExpired
	}
	// Check for future timestamp (clock drift or replay attack)
	if tsTime.Sub(now) > tolerance {
		return ErrTimestampExpired
	}

	// 2. Parse and decode signature
	cleanedSig := strings.TrimSpace(sigHeader)
	cleanedSig = strings.TrimPrefix(cleanedSig, "v1=")

	providedBytes, err := hex.DecodeString(cleanedSig)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidSignatureFormat, err.Error())
	}

	// 3. Compute expected signature and perform constant-time comparison
	expectedHex := SignV2(secret, ts, id, body)
	expectedBytes, err := hex.DecodeString(expectedHex)
	if err != nil {
		return ErrSignatureMismatch
	}

	if !hmac.Equal(providedBytes, expectedBytes) {
		return ErrSignatureMismatch
	}

	return nil
}
