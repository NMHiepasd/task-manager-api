package webhook

import (
	"net"
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

	t.Run("CGNAT_IP_Blocked", func(t *testing.T) {
		err := ValidateURL("http://100.64.0.1:8080/hook", false)
		assert.Error(t, err)
		err2 := ValidateURL("http://100.127.255.254:8080/hook", false)
		assert.Error(t, err2)
	})

	t.Run("PrivateRFC1918Blocked", func(t *testing.T) {
		for _, urlStr := range []string{
			"http://10.0.0.1/hook",
			"http://172.16.0.1/hook",
			"http://192.168.1.1/hook",
		} {
			assert.Error(t, ValidateURL(urlStr, false))
		}
	})

	t.Run("CloudMetadataLinkLocalBlocked", func(t *testing.T) {
		err := ValidateURL("http://169.254.169.254/latest/meta-data", false)
		assert.Error(t, err)
	})

	t.Run("IPv6LoopbackBlocked", func(t *testing.T) {
		err := ValidateURL("http://[::1]:8080/hook", false)
		assert.Error(t, err)
	})
}

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"Nil IP", "", true},
		{"IPv4 Loopback", "127.0.0.1", true},
		{"IPv4 Private 10.x", "10.10.10.10", true},
		{"IPv4 Private 172.16.x", "172.20.0.1", true},
		{"IPv4 Private 192.168.x", "192.168.1.50", true},
		{"IPv4 CGNAT 100.64.x", "100.64.0.1", true},
		{"IPv4 CGNAT 100.127.x", "100.127.255.250", true},
		{"IPv4 Link-Local / Cloud Metadata", "169.254.169.254", true},
		{"IPv4 Broadcast", "255.255.255.255", true},
		{"IPv4 Multicast", "224.0.0.1", true},
		{"IPv4 Unspecified", "0.0.0.0", true},
		{"IPv4 Documentation", "192.0.2.1", true},
		{"IPv6 Loopback", "::1", true},
		{"IPv6 Unspecified", "::", true},
		{"IPv6 Unique Local", "fc00::1", true},
		{"IPv6 Link Local", "fe80::1", true},
		{"IPv6 Multicast", "ff02::1", true},
		{"IPv4-mapped IPv6 Loopback", "::ffff:127.0.0.1", true},
		{"IPv4-mapped IPv6 CGNAT", "::ffff:100.64.1.1", true},
		{"IPv4-mapped IPv6 Private", "::ffff:192.168.1.1", true},
		{"Public IPv4 Google DNS", "8.8.8.8", false},
		{"Public IPv4 Cloudflare DNS", "1.1.1.1", false},
		{"Public IPv6 Google DNS", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ip net.IP
			if tt.ip != "" {
				ip = net.ParseIP(tt.ip)
				require.NotNil(t, ip, "failed to parse test IP %s", tt.ip)
			}
			assert.Equal(t, tt.blocked, IsBlockedIP(ip), "IP %s blocked expectation mismatch", tt.ip)
		})
	}
}
