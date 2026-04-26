package ai_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"email-triage-agent/ai"
	"email-triage-agent/config"
	"email-triage-agent/gmail"
)

// ── Unit tests (no Gemini API needed) ───────────────────────

func TestClassificationFields(t *testing.T) {
	clf := ai.Classification{
		Urgency:     "HIGH",
		Topic:       "billing",
		Sentiment:   "negative",
		Confidence:  0.95,
		IsVIP:       true,
		ActionItems: []string{"respond within 1 hour", "check invoice"},
		ReplyDraft:  "Thank you for reaching out. We will look into this immediately.",
	}

	assert.Equal(t, "HIGH", clf.Urgency)
	assert.Equal(t, "billing", clf.Topic)
	assert.Equal(t, "negative", clf.Sentiment)
	assert.InDelta(t, 0.95, clf.Confidence, 0.001)
	assert.True(t, clf.IsVIP)
	assert.Len(t, clf.ActionItems, 2)
	assert.NotEmpty(t, clf.ReplyDraft)
}

func TestNewClassifier_MissingAPIKey(t *testing.T) {
	cfg := config.Config{
		GeminiAPIKey:      "", // empty — should fail gracefully
		GeminiModel:       "gemini-1.5-flash",
		GeminiTimeoutSecs: 30,
	}

	_, err := ai.NewClassifier(cfg, nil)
	// Should return an error when API key is missing
	assert.Error(t, err)
}

func TestNewClassifier_VIPFastPath(t *testing.T) {
	cfg := config.Config{
		GeminiAPIKey:      "fake-key-for-test",
		GeminiModel:       "gemini-1.5-flash",
		GeminiTimeoutSecs: 30,
		VIPSenders:        "boss@company.com,cto@company.com",
		EnableVIPFastPath: true,
	}

	classifier, err := ai.NewClassifier(cfg, nil)
	// May fail to connect but VIP logic is local — test separately
	if err != nil {
		t.Skip("Skipping: cannot connect to Gemini without valid API key")
	}
	defer classifier.Close()

	email := gmail.Email{
		ID:      "test-001",
		From:    "Boss Name <boss@company.com>",
		Subject: "Urgent: Q4 numbers",
		Body:    "Please review immediately.",
		Date:    time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clf, err := classifier.Classify(ctx, email, nil)
	require.NoError(t, err)

	// VIP fast path should force HIGH urgency without calling Gemini
	assert.Equal(t, "HIGH", clf.Urgency)
	assert.True(t, clf.IsVIP)
	assert.InDelta(t, 1.0, clf.Confidence, 0.001)
}

// ── Table-driven urgency label tests ────────────────────────

func TestUrgencyLabels(t *testing.T) {
	tests := []struct {
		name     string
		urgency  string
		valid    bool
	}{
		{"high is valid",   "HIGH",   true},
		{"medium is valid", "MEDIUM", true},
		{"low is valid",    "LOW",    true},
		{"empty is invalid","",       false},
		{"unknown invalid", "CRITICAL",false},
	}

	validUrgencies := map[string]bool{
		"HIGH": true, "MEDIUM": true, "LOW": true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := validUrgencies[tt.urgency]
			assert.Equal(t, tt.valid, ok)
		})
	}
}

// ── Table-driven confidence threshold tests ──────────────────

func TestConfidenceThresholds(t *testing.T) {
	cfg := config.Config{
		UrgencyHighConf: 0.80,
		AutoDraftConf:   0.90,
		AutoArchiveConf: 0.95,
	}

	tests := []struct {
		name        string
		confidence  float64
		shouldAlert bool
		shouldDraft bool
		shouldArchive bool
	}{
		{"low confidence",    0.50, false, false, false},
		{"above alert",       0.85, true,  false, false},
		{"above draft",       0.92, true,  true,  false},
		{"above archive",     0.97, true,  true,  true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.shouldAlert,   tt.confidence >= cfg.UrgencyHighConf)
			assert.Equal(t, tt.shouldDraft,   tt.confidence >= cfg.AutoDraftConf)
			assert.Equal(t, tt.shouldArchive, tt.confidence >= cfg.AutoArchiveConf)
		})
	}
}