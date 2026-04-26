package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"email-triage-agent/ai"
	"email-triage-agent/config"
	"email-triage-agent/gmail"
)

var slackHTTPClient = &http.Client{Timeout: 5 * time.Second}

type Slack struct {
	webhookURL string
	channel    string
}

func NewSlack(cfg config.Config) *Slack {
	return &Slack{
		webhookURL: cfg.SlackWebhookURL,
		channel:    cfg.SlackChannel,
	}
}

type slackBlock struct {
	Type string      `json:"type"`
	Text *slackText  `json:"text,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackPayload struct {
	Blocks []slackBlock `json:"blocks"`
}

// SendUrgentAlert posts a HIGH urgency alert to Slack
func (s *Slack) SendUrgentAlert(ctx context.Context, email gmail.Email, clf ai.Classification) error {
	actions := "None"
	if len(clf.ActionItems) > 0 {
		actions = "• " + strings.Join(clf.ActionItems, "\n• ")
	}

	vipBadge := ""
	if clf.IsVIP {
		vipBadge = " ⭐ VIP"
	}

	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackText{Type: "plain_text", Text: fmt.Sprintf("🚨 Urgent Email%s", vipBadge)},
		},
		{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf(
				"*From:* %s\n*Subject:* %s\n*Topic:* `%s` | *Sentiment:* %s | *Confidence:* %.0f%%",
				email.From, email.Subject, clf.Topic, clf.Sentiment, clf.Confidence*100,
			)},
		},
		{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Action Items:*\n%s", actions)},
		},
		{Type: "divider"},
	}

	if clf.ReplyDraft != "" {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Suggested Reply:*\n>%s",
				strings.ReplaceAll(clf.ReplyDraft, "\n", "\n>"))},
		})
	}

	blocks = append(blocks, slackBlock{
		Type: "context",
		Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("📅 Received: %s", email.Date.Format(time.RFC1123))},
	})

	return s.send(ctx, blocks)
}

// SendDigest posts the daily summary to Slack
func (s *Slack) SendDigest(ctx context.Context, total, high, medium, low int, topTopics map[string]int) error {
	topicsStr := ""
	for topic, cnt := range topTopics {
		topicsStr += fmt.Sprintf("• `%s`: %d\n", topic, cnt)
	}
	if topicsStr == "" {
		topicsStr = "No data"
	}

	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackText{Type: "plain_text", Text: "📧 Daily Email Digest"},
		},
		{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf(
				"*Total processed:* %d\n🔴 *High:* %d   🟡 *Medium:* %d   🟢 *Low:* %d",
				total, high, medium, low,
			)},
		},
		{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Top Topics:*\n%s", topicsStr)},
		},
		{
			Type: "context",
			Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("Generated at %s", time.Now().Format(time.RFC1123))},
		},
	}

	return s.send(ctx, blocks)
}

func (s *Slack) send(ctx context.Context, blocks []slackBlock) error {
	payload := slackPayload{Blocks: blocks}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := slackHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}
	return nil
}