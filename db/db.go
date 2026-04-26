package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"email-triage-agent/ai"
	"email-triage-agent/config"
	"email-triage-agent/gmail"
)

type DB struct {
	conn *sql.DB
}

type TriageLog struct {
	ID          int64
	EmailID     string
	ThreadID    string
	From        string
	Subject     string
	Urgency     string
	Topic       string
	Sentiment   string
	Confidence  float64
	ActionItems []string
	ReplyDraft  string
	IsVIP       bool
	ProcessedAt time.Time
}

func New(cfg config.Config) (*DB, error) {
	conn, err := sql.Open("postgres", cfg.PostgresURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	conn.SetMaxOpenConns(cfg.PostgresMaxConns)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (d *DB) Migrate(ctx context.Context) error {
	_, err := d.conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS triage_log (
			id           BIGSERIAL PRIMARY KEY,
			email_id     TEXT NOT NULL UNIQUE,
			thread_id    TEXT,
			from_addr    TEXT,
			subject      TEXT,
			urgency      TEXT,
			topic        TEXT,
			sentiment    TEXT,
			confidence   FLOAT,
			action_items JSONB,
			reply_draft  TEXT,
			is_vip       BOOLEAN DEFAULT FALSE,
			processed_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_urgency      ON triage_log(urgency);
		CREATE INDEX IF NOT EXISTS idx_topic        ON triage_log(topic);
		CREATE INDEX IF NOT EXISTS idx_is_vip       ON triage_log(is_vip);
		CREATE INDEX IF NOT EXISTS idx_processed_at ON triage_log(processed_at);
	`)
	return err
}

// LogTriage inserts one triage result row
func (d *DB) LogTriage(ctx context.Context, email gmail.Email, clf ai.Classification) error {
	items, err := json.Marshal(clf.ActionItems)
	if err != nil {
		items = []byte("[]")
	}
	_, err = d.conn.ExecContext(ctx, `
		INSERT INTO triage_log
			(email_id, thread_id, from_addr, subject, urgency, topic,
			 sentiment, confidence, action_items, reply_draft, is_vip)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (email_id) DO NOTHING`,
		email.ID, email.ThreadID, email.From, email.Subject,
		clf.Urgency, clf.Topic, clf.Sentiment, clf.Confidence,
		string(items), clf.ReplyDraft, clf.IsVIP,
	)
	return err
}

// GetRecent returns the N most recent triage logs
func (d *DB) GetRecent(ctx context.Context, limit int) ([]TriageLog, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, email_id, thread_id, from_addr, subject, urgency, topic,
		       sentiment, confidence, action_items, reply_draft, is_vip, processed_at
		FROM triage_log
		ORDER BY processed_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []TriageLog
	for rows.Next() {
		var l TriageLog
		var itemsJSON string
		err := rows.Scan(
			&l.ID, &l.EmailID, &l.ThreadID, &l.From, &l.Subject,
			&l.Urgency, &l.Topic, &l.Sentiment, &l.Confidence,
			&itemsJSON, &l.ReplyDraft, &l.IsVIP, &l.ProcessedAt,
		)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(itemsJSON), &l.ActionItems)
		logs = append(logs, l)
	}
	return logs, nil
}

// Stats returns urgency counts for the last N hours
func (d *DB) Stats(ctx context.Context, hours int) (high, medium, low, total int, err error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT urgency, COUNT(*)
		FROM triage_log
		WHERE processed_at > NOW() - INTERVAL '1 hour' * $1
		GROUP BY urgency`, hours)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var urg string
		var cnt int
		rows.Scan(&urg, &cnt)
		total += cnt
		switch urg {
		case "HIGH":
			high = cnt
		case "MEDIUM":
			medium = cnt
		case "LOW":
			low = cnt
		}
	}
	return
}

// TopTopics returns the most common topics in the last N hours
func (d *DB) TopTopics(ctx context.Context, hours, limit int) (map[string]int, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT topic, COUNT(*) as cnt
		FROM triage_log
		WHERE processed_at > NOW() - INTERVAL '1 hour' * $1
		GROUP BY topic
		ORDER BY cnt DESC
		LIMIT $2`, hours, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int{}
	for rows.Next() {
		var topic string
		var cnt int
		rows.Scan(&topic, &cnt)
		result[topic] = cnt
	}
	return result, nil
}

// Cleanup deletes rows older than N days
func (d *DB) Cleanup(ctx context.Context, days int) (int64, error) {
	res, err := d.conn.ExecContext(ctx, `
		DELETE FROM triage_log
		WHERE processed_at < NOW() - INTERVAL '1 day' * $1`, days)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) Close() error {
	return d.conn.Close()
}