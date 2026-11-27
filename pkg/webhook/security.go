package webhook

import (
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

// blockedCIDRs contains all private, reserved, CGNAT, loopback, and documentation CIDR blocks
// that must be blocked to prevent Server-Side Request Forgery (SSRF).
var blockedCIDRs []*net.IPNet

func init() {
	ranges := []string{
		"0.0.0.0/8",          // Current network (RFC 1122)
		"10.0.0.0/8",         // Private IPv4 (RFC 1918)
		"100.64.0.0/10",      // Shared Address Space / CGNAT (RFC 6598)
		"127.0.0.0/8",        // Loopback IPv4 (RFC 1122)
		"169.254.0.0/16",     // Link-Local IPv4 (RFC 3927)
		"172.16.0.0/12",      // Private IPv4 (RFC 1918)
		"192.0.0.0/24",       // IETF Protocol Assignments (RFC 6890)
		"192.0.2.0/24",       // Documentation TEST-NET-1 (RFC 5737)
		"192.88.99.0/24",     // 6to4 Relay Anycast (RFC 7526)
		"192.168.0.0/16",     // Private IPv4 (RFC 1918)
		"198.18.0.0/15",      // Benchmarking (RFC 2544)
		"198.51.100.0/24",    // Documentation TEST-NET-2 (RFC 5737)
		"203.0.113.0/24",     // Documentation TEST-NET-3 (RFC 5737)
		"224.0.0.0/4",        // Multicast (RFC 5771)
		"240.0.0.0/4",        // Reserved for future use (RFC 1112)
		"255.255.255.255/32", // Limited Broadcast (RFC 919)
		"::/128",             // Unspecified IPv6 (RFC 4291)
		"::1/128",            // Loopback IPv6 (RFC 4291)
		"100::/64",           // Discard-Only Address Block (RFC 6666)
		"2001:db8::/32",      // Documentation IPv6 (RFC 3849)
		"fc00::/7",           // Unique Local IPv6 (RFC 4193)
		"fe80::/10",          // Link-Local Unicast IPv6 (RFC 4291)
		"ff00::/8",           // Multicast IPv6 (RFC 4291)
	}

	for _, cidr := range ranges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			blockedCIDRs = append(blockedCIDRs, ipNet)
		}
	}
}

// IsBlockedIP returns true if the given IP address is private, loopback, link-local,
// multicast, CGNAT (100.64.0.0/10), or reserved.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Normalize IPv4-mapped IPv6 addresses (e.g., ::ffff:127.0.0.1 -> 127.0.0.1)
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
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

	// If hostname is directly an IP address
	if ip := net.ParseIP(hostname); ip != nil {
		if IsBlockedIP(ip) {
			return domain.ErrWebhookSSRF
		}
		return nil
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If DNS resolution fails and it's not an IP literal, it cannot be safely validated
		return fmt.Errorf("%w: failed to resolve host '%s'", domain.ErrInvalidWebhookURL, hostname)
	}

	for _, ip := range ips {
		if IsBlockedIP(ip) {
			return domain.ErrWebhookSSRF
		}
	}

	return nil
}
