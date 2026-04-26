package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"email-triage-agent/ai"
	"email-triage-agent/config"
	"email-triage-agent/db"
	"email-triage-agent/gmail"
)

// getTestDB connects to a real DB only if POSTGRES_URL is set
// otherwise skips the test — safe for CI without a DB
func getTestDB(t *testing.T) *db.DB {
	t.Helper()
	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		t.Skip("Skipping: POSTGRES_URL not set")
	}
	cfg := config.Config{
		PostgresURL:      url,
		PostgresMaxConns: 2,
	}
	database, err := db.New(cfg)
	require.NoError(t, err, "DB connection failed")
	return database
}

// ── Migration test ───────────────────────────────────────────

func TestMigrate_CreatesTable(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := database.Migrate(ctx)
	assert.NoError(t, err, "migration should succeed")

	// Running twice should be idempotent
	err = database.Migrate(ctx)
	assert.NoError(t, err, "migration should be idempotent")
}

// ── LogTriage tests ──────────────────────────────────────────

func TestLogTriage_InsertsRow(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, database.Migrate(ctx))

	email := gmail.Email{
		ID:       "test-email-" + time.Now().Format("20060102150405"),
		ThreadID: "thread-001",
		From:     "sender@example.com",
		Subject:  "Test email for unit test",
		Body:     "This is a test body.",
		Date:     time.Now(),
	}

	clf := ai.Classification{
		Urgency:     "HIGH",
		Topic:       "support",
		Sentiment:   "negative",
		Confidence:  0.92,
		IsVIP:       false,
		ActionItems: []string{"reply within 1h", "check ticket system"},
		ReplyDraft:  "Thank you for reaching out. We are looking into this.",
	}

	err := database.LogTriage(ctx, email, clf)
	assert.NoError(t, err)
}

func TestLogTriage_DuplicateIsIgnored(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, database.Migrate(ctx))

	email := gmail.Email{
		ID:      "duplicate-test-" + time.Now().Format("20060102150405"),
		From:    "dup@example.com",
		Subject: "Duplicate test",
	}

	clf := ai.Classification{
		Urgency:    "LOW",
		Topic:      "newsletter",
		Confidence: 0.99,
	}

	// First insert
	err := database.LogTriage(ctx, email, clf)
	require.NoError(t, err)

	// Second insert with same ID — should be silently ignored
	err = database.LogTriage(ctx, email, clf)
	assert.NoError(t, err, "duplicate insert should not error")
}

func TestLogTriage_VIPFlag(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, database.Migrate(ctx))

	email := gmail.Email{
		ID:      "vip-test-" + time.Now().Format("20060102150405"),
		From:    "cto@company.com",
		Subject: "VIP email test",
	}

	clf := ai.Classification{
		Urgency:    "HIGH",
		Topic:      "vip",
		Confidence: 1.0,
		IsVIP:      true,
	}

	err := database.LogTriage(ctx, email, clf)
	assert.NoError(t, err)
}

// ── Stats tests ──────────────────────────────────────────────

func TestStats_ReturnsCorrectCounts(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	require.NoError(t, database.Migrate(ctx))

	base := time.Now().Format("20060102150405")

	// Insert known data
	testCases := []struct {
		id      string
		urgency string
	}{
		{base + "-h1", "HIGH"},
		{base + "-h2", "HIGH"},
		{base + "-m1", "MEDIUM"},
		{base + "-l1", "LOW"},
		{base + "-l2", "LOW"},
		{base + "-l3", "LOW"},
	}

	for _, tc := range testCases {
		email := gmail.Email{ID: tc.id, From: "test@test.com", Subject: "test"}
		clf := ai.Classification{Urgency: tc.urgency, Topic: "support", Confidence: 0.9}
		require.NoError(t, database.LogTriage(ctx, email, clf))
	}

	high, medium, low, total, err := database.Stats(ctx, 1)
	require.NoError(t, err)

	// Total should include at minimum our inserts
	assert.GreaterOrEqual(t, total, 6)
	assert.GreaterOrEqual(t, high, 2)
	assert.GreaterOrEqual(t, medium, 1)
	assert.GreaterOrEqual(t, low, 3)
}

// ── GetRecent tests ──────────────────────────────────────────

func TestGetRecent_ReturnsRows(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, database.Migrate(ctx))

	logs, err := database.GetRecent(ctx, 10)
	require.NoError(t, err)
	assert.NotNil(t, logs)
}

func TestGetRecent_RespectsLimit(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, database.Migrate(ctx))

	logs, err := database.GetRecent(ctx, 3)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(logs), 3)
}

// ── TopTopics tests ──────────────────────────────────────────

func TestTopTopics_ReturnsMap(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, database.Migrate(ctx))

	topics, err := database.TopTopics(ctx, 24, 5)
	require.NoError(t, err)
	assert.NotNil(t, topics)
}

// ── Cleanup tests ────────────────────────────────────────────

func TestCleanup_DeletesOldRows(t *testing.T) {
	database := getTestDB(t)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, database.Migrate(ctx))

	// Cleanup rows older than 90 days — should not error
	n, err := database.Cleanup(ctx, 90)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(0))
}