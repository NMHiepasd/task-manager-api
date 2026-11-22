package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"time"

	"task-manager-api/pkg/webhook"
)

// DeliveryRecord stores received webhook payload and verification outcome.
type DeliveryRecord struct {
	ID          string    `json:"id"`
	Event       string    `json:"event"`
	Timestamp   string    `json:"timestamp"`
	Signature   string    `json:"signature"`
	Body        string    `json:"body"`
	Verified    bool      `json:"verified"`
	Error       string    `json:"error,omitempty"`
	ReceivedAt  time.Time `json:"received_at"`
}

type ReceiverServer struct {
	mu           sync.RWMutex
	secret       string
	tolerance    time.Duration
	deliveries   []DeliveryRecord
	lastDelivery *DeliveryRecord
}

func NewReceiverServer(secret string, tolerance time.Duration) *ReceiverServer {
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	return &ReceiverServer{
		secret:    secret,
		tolerance: tolerance,
	}
}

func (s *ReceiverServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	id := r.Header.Get("X-Webhook-Id")
	ts := r.Header.Get("X-Webhook-Timestamp")
	sig := r.Header.Get("X-Webhook-Signature")
	event := r.Header.Get("X-Webhook-Event")

	record := DeliveryRecord{
		ID:         id,
		Event:      event,
		Timestamp:  ts,
		Signature:  sig,
		Body:       string(body),
		ReceivedAt: time.Now().UTC(),
	}

	// Verify using Webhook V2 specification with constant-time comparison & tolerance window
	verifyErr := webhook.VerifyV2(s.secret, ts, id, body, sig, time.Now().UTC(), s.tolerance)
	if verifyErr != nil {
		record.Verified = false
		record.Error = verifyErr.Error()
		fmt.Printf("[REJECTED] Webhook ID: %s, Event: %s, Error: %v\n", id, event, verifyErr)

		s.mu.Lock()
		s.deliveries = append(s.deliveries, record)
		s.lastDelivery = &record
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "rejected",
			"error":  verifyErr.Error(),
		})
		return
	}

	record.Verified = true
	fmt.Printf("[VERIFIED] Webhook ID: %s, Event: %s, Timestamp: %s\n", id, event, ts)

	s.mu.Lock()
	s.deliveries = append(s.deliveries, record)
	s.lastDelivery = &record
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"id":     id,
	})
}

func (s *ReceiverServer) handleLastDelivery(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.lastDelivery == nil {
		http.Error(w, "No deliveries recorded", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.lastDelivery)
}

func (s *ReceiverServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

func (s *ReceiverServer) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", s.handleWebhook)
	mux.HandleFunc("/last-delivery", s.handleLastDelivery)
	mux.HandleFunc("/healthz", s.handleHealthz)
	return mux
}

// runSelfTest executes a comprehensive end-to-end verification of Webhook V2 delivery,
// signature verification, and replay attack tolerance rejection.
func runSelfTest(secret string) error {
	server := NewReceiverServer(secret, 5*time.Minute)
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	// 1. Send valid V2 webhook request
	eventID := "01934567-89ab-7cde-8f01-234567890abc"
	now := time.Now().UTC()
	tsStr := strconv.FormatInt(now.Unix(), 10)
	payload := []byte(`{"event":"task.created","task":{"id":"01934567-89ab-7cde-8f01-234567890abc","title":"Live Webhook Task"}}`)
	sig := webhook.SignV2(secret, tsStr, eventID, payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/webhook", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Id", eventID)
	req.Header.Set("X-Webhook-Timestamp", tsStr)
	req.Header.Set("X-Webhook-Signature", "v1="+sig)
	req.Header.Set("X-Webhook-Event", "task.created")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("delivery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("expected HTTP 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	// 2. Query last delivery from receiver
	lastResp, err := http.Get(ts.URL + "/last-delivery")
	if err != nil {
		return fmt.Errorf("failed to fetch last delivery: %w", err)
	}
	defer lastResp.Body.Close()

	var record DeliveryRecord
	if err := json.NewDecoder(lastResp.Body).Decode(&record); err != nil {
		return fmt.Errorf("failed to decode last delivery: %w", err)
	}
	if !record.Verified || record.ID != eventID {
		return fmt.Errorf("delivery not verified correctly in receiver: %+v", record)
	}

	// 3. Test Replay Attack: Re-sending with timestamp older than tolerance (>5 minutes)
	expiredTs := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	expiredSig := webhook.SignV2(secret, expiredTs, eventID, payload)

	replayReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/webhook", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create replay request: %w", err)
	}
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.Header.Set("X-Webhook-Id", eventID)
	replayReq.Header.Set("X-Webhook-Timestamp", expiredTs)
	replayReq.Header.Set("X-Webhook-Signature", "v1="+expiredSig)
	replayReq.Header.Set("X-Webhook-Event", "task.created")

	replayResp, err := http.DefaultClient.Do(replayReq)
	if err != nil {
		return fmt.Errorf("replay request failed: %w", err)
	}
	defer replayResp.Body.Close()

	if replayResp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("expected HTTP 401 for expired replay request, got %d", replayResp.StatusCode)
	}

	// 4. Test Tampered Payload Rejection
	tamperedBody := []byte(`{"event":"task.created","task":{"id":"01934567-89ab-7cde-8f01-234567890abc","title":"TAMPERED"}}`)
	tamperReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/webhook", bytes.NewReader(tamperedBody))
	if err != nil {
		return fmt.Errorf("failed to create tamper request: %w", err)
	}
	tamperReq.Header.Set("Content-Type", "application/json")
	tamperReq.Header.Set("X-Webhook-Id", eventID)
	tamperReq.Header.Set("X-Webhook-Timestamp", tsStr)
	tamperReq.Header.Set("X-Webhook-Signature", "v1="+sig) // Signature of original untampered body
	tamperReq.Header.Set("X-Webhook-Event", "task.created")

	tamperResp, err := http.DefaultClient.Do(tamperReq)
	if err != nil {
		return fmt.Errorf("tamper request failed: %w", err)
	}
	defer tamperResp.Body.Close()

	if tamperResp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("expected HTTP 401 for tampered payload, got %d", tamperResp.StatusCode)
	}

	// 5. Direct verification with webhook.VerifyV2 against errors.Is
	directErr := webhook.VerifyV2(secret, expiredTs, eventID, payload, "v1="+expiredSig, time.Now().UTC(), 5*time.Minute)
	if !errors.Is(directErr, webhook.ErrTimestampExpired) {
		return fmt.Errorf("expected ErrTimestampExpired, got %v", directErr)
	}

	return nil
}

func main() {
	port := flag.Int("port", 9999, "Port to listen on")
	secret := flag.String("secret", "smoke-secret-key-minimum-32-characters-long!!", "HMAC signing secret")
	isTest := flag.Bool("test", false, "Run self-contained end-to-end verification and exit")
	flag.Parse()

	if envSecret := os.Getenv("WEBHOOK_SECRET"); envSecret != "" {
		*secret = envSecret
	}
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}

	if *isTest {
		fmt.Println("[TEST] Running Webhook V2 delivery & verification self-test...")
		if err := runSelfTest(*secret); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("PASSED: Webhook V2 delivery, signature verification, and replay tolerance rejection verified.")
		return
	}

	server := NewReceiverServer(*secret, 5*time.Minute)
	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("Webhook Receiver listening on %s (secret length: %d bytes)\n", addr, len(*secret))
	fmt.Printf("Endpoints:\n  POST %s/webhook\n  GET  %s/last-delivery\n  GET  %s/healthz\n", addr, addr, addr)

	if err := http.ListenAndServe(addr, server.Routes()); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Receiver server failed: %v", err)
	}
}
