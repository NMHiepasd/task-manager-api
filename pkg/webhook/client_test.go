package webhook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSafeHTTPClient_BlocksSSRF(t *testing.T) {
	// Local test server on 127.0.0.1
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("internal secret data"))
	}))
	defer ts.Close()

	// Client with allowLocal = false must block access to 127.0.0.1
	client := NewSafeHTTPClient(false, 2*time.Second)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	assert.Error(t, err, "expected client.Do to fail with SSRF blocked error")
	assert.Contains(t, err.Error(), "SSRF prevention")
}

func TestNewSafeHTTPClient_AllowsLocalWhenConfigured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := NewSafeHTTPClient(true, 2*time.Second)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(body))
}

func TestNewSafeHTTPClient_DoesNotFollowRedirects(t *testing.T) {
	redirectTargetCalled := false
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectTargetCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	redirectingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetServer.URL, http.StatusFound)
	}))
	defer redirectingServer.Close()

	client := NewSafeHTTPClient(true, 2*time.Second)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, redirectingServer.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// The client must return the 302 response and NOT follow to targetServer
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.False(t, redirectTargetCalled, "redirect target should NOT have been called")
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "http"))
}
