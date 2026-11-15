package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"
	"task-manager-api/internal/domain"
)

// SupportedEventTypes enumerates all system events eligible for webhook subscription.
var SupportedEventTypes = map[string]bool{
	"task.created":         true,
	"task.updated":         true,
	"task.deleted":         true,
	"comment.created":      true,
	"attachment.uploaded":  true,
	"project.member_added": true,
	"*":                    true,
	"ping":                 true,
}

// ValidateEvents verifies that all subscribed events are supported and non-empty.
func ValidateEvents(events []string) error {
	if len(events) == 0 {
		return domain.ErrInvalidWebhookEvents
	}
	for _, e := range events {
		trimmed := strings.TrimSpace(e)
		if trimmed == "" || !SupportedEventTypes[trimmed] {
			return fmt.Errorf("%w: unsupported event '%s'", domain.ErrInvalidWebhookEvents, e)
		}
	}
	return nil
}

// ValidateURL inspects a webhook target URL for protocol validity and SSRF prevention.
func ValidateURL(targetURL string, allowLocal bool) error {
	parsed, err := url.ParseRequestURI(targetURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return domain.ErrInvalidWebhookURL
	}

	hostname := parsed.Hostname()
	if allowLocal {
		return nil
	}

	// Guard against localhost directly
	if strings.EqualFold(hostname, "localhost") {
		return domain.ErrWebhookSSRF
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If DNS resolution fails and it's not an IP literal, it cannot be safely validated
		return fmt.Errorf("%w: failed to resolve host '%s'", domain.ErrInvalidWebhookURL, hostname)
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return domain.ErrWebhookSSRF
		}
	}

	return nil
}

// ComputeSignature calculates the HMAC-SHA256 hex digest of a payload using a secret.
func ComputeSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature validates a signature against a payload using constant-time comparison.
func VerifySignature(secret string, payload []byte, expectedHexSignature string) bool {
	expectedSig, err := hex.DecodeString(expectedHexSignature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualSig := mac.Sum(nil)
	return hmac.Equal(actualSig, expectedSig)
}
