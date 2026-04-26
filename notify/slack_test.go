package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"email-triage-agent/ai"
	"email-triage-agent/config"
	"email-triage-agent/gmail"
	"email-triage-agent/notify"
)

// mockSlackServer spins up a local HTTP server that acts as Slack
func mockSlackServer(t *testing.T, handler func(body map[string]interface{})) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		if handler != nil {
			handler(body)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
}

// ── SendUrgentAlert tests ────────────────────────────────────

func TestSendUrgentAlert_SendsCorrectPayload(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockSlackServer(t, func(body map[string]interface{}) {
		receivedBody = body
	})
	defer server.Close()

	cfg := config.Config{
		SlackWebhookURL: server.URL,
		SlackChannel:    "#email-urgent",
	}

	slack := notify.NewSlack(cfg)

	email := gmail.Email{
		ID:      "email-001",
		From:    "client@bigcompany.com",
		Subject: "Payment overdue — urgent",
		Body:    "Our invoice has not been paid.",
		Date:    time.Now(),
	}

	clf := ai.Classification{
		Urgency:     "HIGH",
		Topic:       "billing",
		Sentiment:   "negative",
		Confidence:  0.95,
		IsVIP:       false,
		ActionItems: []string{"check invoice", "reply within 1h"},
		ReplyDraft:  "Thank you for reaching out. We will resolve this immediately.",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := slack.SendUrgentAlert(ctx, email, clf)
	require.NoError(t, err)

	// Verify payload was sent
	assert.NotNil(t, receivedBody)
	blocks, ok := receivedBody["blocks"].([]interface{})
	assert.True(t, ok, "blocks should be an array")
	assert.Greater(t, len(blocks), 0, "should have at least one block")
}

func TestSendUrgentAlert_VIPBadge(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockSlackServer(t, func(body map[string]interface{}) {
		receivedBody = body
	})
	defer server.Close()

	cfg := config.Config{SlackWebhookURL: server.URL}
	slack := notify.NewSlack(cfg)

	email := gmail.Email{
		ID:      "vip-email-001",
		From:    "cto@company.com",
		Subject: "Critical issue",
		Date:    time.Now(),
	}

	clf := ai.Classification{
		Urgency:    "HIGH",
		Topic:      "vip",
		Confidence: 1.0,
		IsVIP:      true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := slack.SendUrgentAlert(ctx, email, clf)
	require.NoError(t, err)

	// Header block should contain VIP badge
	blocks := receivedBody["blocks"].([]interface{})
	header := blocks[0].(map[string]interface{})
	headerText := header["text"].(map[string]interface{})
	assert.Contains(t, headerText["text"].(string), "VIP")
}

func TestSendUrgentAlert_NoActionItems(t *testing.T) {
	server := mockSlackServer(t, nil)
	defer server.Close()

	cfg := config.Config{SlackWebhookURL: server.URL}
	slack := notify.NewSlack(cfg)

	email := gmail.Email{
		ID:      "no-actions-001",
		From:    "someone@example.com",
		Subject: "Hello",
		Date:    time.Now(),
	}

	clf := ai.Classification{
		Urgency:     "HIGH",
		Topic:       "support",
		Confidence:  0.85,
		ActionItems: []string{}, // empty
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should not error even with empty action items
	err := slack.SendUrgentAlert(ctx, email, clf)
	assert.NoError(t, err)
}

// ── SendDigest tests ─────────────────────────────────────────

func TestSendDigest_SendsCorrectCounts(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockSlackServer(t, func(body map[string]interface{}) {
		receivedBody = body
	})
	defer server.Close()

	cfg := config.Config{SlackWebhookURL: server.URL}
	slack := notify.NewSlack(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	topTopics := map[string]int{
		"billing": 5,
		"support": 3,
		"spam":    2,
	}

	err := slack.SendDigest(ctx, 10, 3, 4, 3, topTopics)
	require.NoError(t, err)

	assert.NotNil(t, receivedBody)
	blocks := receivedBody["blocks"].([]interface{})
	assert.Greater(t, len(blocks), 0)
}

func TestSendDigest_EmptyTopics(t *testing.T) {
	server := mockSlackServer(t, nil)
	defer server.Close()

	cfg := config.Config{SlackWebhookURL: server.URL}
	slack := notify.NewSlack(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should not error with empty topics map
	err := slack.SendDigest(ctx, 0, 0, 0, 0, map[string]int{})
	assert.NoError(t, err)
}

// ── Error handling tests ─────────────────────────────────────

func TestSendUrgentAlert_ServerError(t *testing.T) {
	// Server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := config.Config{SlackWebhookURL: server.URL}
	slack := notify.NewSlack(cfg)

	email := gmail.Email{ID: "err-001", From: "x@x.com", Subject: "test", Date: time.Now()}
	clf := ai.Classification{Urgency: "HIGH", Confidence: 0.9}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := slack.SendUrgentAlert(ctx, email, clf)
	assert.Error(t, err, "should return error on 500 response")
}

func TestSendUrgentAlert_InvalidURL(t *testing.T) {
	cfg := config.Config{SlackWebhookURL: "http://localhost:0/invalid"}
	slack := notify.NewSlack(cfg)

	email := gmail.Email{ID: "err-002", From: "x@x.com", Subject: "test", Date: time.Now()}
	clf := ai.Classification{Urgency: "HIGH", Confidence: 0.9}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := slack.SendUrgentAlert(ctx, email, clf)
	assert.Error(t, err, "should return error when server unreachable")
}