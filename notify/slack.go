package notify

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"

    "email-triage-agent/ai"
    "email-triage-agent/config"
    "email-triage-agent/gmail"
)

type Slack struct{ webhookURL, channel string }

func NewSlack(cfg config.Config) *Slack {
    return &Slack{webhookURL: cfg.SlackWebhookURL, channel: cfg.SlackChannel}
}

type block struct {
    Type string `json:"type"`
    Text *btext `json:"text,omitempty"`
}
type btext struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

func (s *Slack) SendUrgentAlert(ctx context.Context, email gmail.Email, clf ai.Classification) error {
    actions := "None"
    if len(clf.ActionItems) > 0 {
        actions = "• " + strings.Join(clf.ActionItems, "\n• ")
    }
    return s.send(ctx, []block{
        {Type: "header", Text: &btext{"plain_text", "Urgent Email Alert"}},
        {Type: "section", Text: &btext{"mrkdwn", fmt.Sprintf(
            "*From:* %s\n*Subject:* %s\n*Topic:* %s | *Confidence:* %.0f%%",
            email.From, email.Subject, clf.Topic, clf.Confidence*100,
        )}},
        {Type: "section", Text: &btext{"mrkdwn", fmt.Sprintf("*Actions:*\n%s", actions)}},
        {Type: "section", Text: &btext{"mrkdwn", fmt.Sprintf("*Draft:*\n_%s_", clf.ReplyDraft)}},
    })
}

func (s *Slack) SendDigest(ctx context.Context, total, high, medium, low int) error {
    return s.send(ctx, []block{
        {Type: "header", Text: &btext{"plain_text", "Daily Email Digest"}},
        {Type: "section", Text: &btext{"mrkdwn", fmt.Sprintf(
            "*Total:* %d\nHigh: %d  Medium: %d  Low: %d", total, high, medium, low,
        )}},
    })
}

func (s *Slack) send(ctx context.Context, blocks []block) error {
    body, _ := json.Marshal(map[string]interface{}{"blocks": blocks})
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewBuffer(body))
    if err != nil { return err }
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return fmt.Errorf("slack: %w", err) }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK { return fmt.Errorf("slack status %d", resp.StatusCode) }
    return nil
}
