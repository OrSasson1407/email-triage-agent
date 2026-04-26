package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"go.uber.org/zap"
	"google.golang.org/api/option"

	"email-triage-agent/config"
	"email-triage-agent/gmail"
)

type Classification struct {
	Urgency     string   `json:"urgency"`
	Topic       string   `json:"topic"`
	Sentiment   string   `json:"sentiment"`
	ReplyDraft  string   `json:"reply_draft"`
	ActionItems []string `json:"action_items"`
	Confidence  float64  `json:"confidence"`
	IsVIP       bool     `json:"is_vip"`
}

type Classifier struct {
	client  *genai.Client
	model   *genai.GenerativeModel
	logger  *zap.Logger
	cfg     config.Config
	vipList map[string]bool
}

func NewClassifier(cfg config.Config, logger *zap.Logger) (*Classifier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiAPIKey))
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}

	model := client.GenerativeModel(cfg.GeminiModel)
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"urgency":      {Type: genai.TypeString},
			"topic":        {Type: genai.TypeString},
			"sentiment":    {Type: genai.TypeString},
			"reply_draft":  {Type: genai.TypeString},
			"action_items": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
			"confidence":   {Type: genai.TypeNumber},
			"is_vip":       {Type: genai.TypeBoolean},
		},
	}

	// VIP senders — always treated as HIGH priority
	vipList := map[string]bool{}
	if vips := cfg.VIPSenders; vips != "" {
		for _, v := range strings.Split(vips, ",") {
			vipList[strings.TrimSpace(strings.ToLower(v))] = true
		}
	}

	return &Classifier{
		client:  client,
		model:   model,
		logger:  logger,
		cfg:     cfg,
		vipList: vipList,
	}, nil
}

// Classify sends an email to Gemini and returns structured classification
func (c *Classifier) Classify(ctx context.Context, email gmail.Email, threadCtx []gmail.Email) (Classification, error) {
	timeout := time.Duration(c.cfg.GeminiTimeoutSecs) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// VIP fast path — skip Gemini, force HIGH urgency
	if c.cfg.EnableVIPFastPath && c.isVIP(email.From) {
		c.logger.Info("VIP fast path", zap.String("from", email.From))
		return Classification{
			Urgency:    "HIGH",
			Topic:      "vip",
			Sentiment:  "neutral",
			Confidence: 1.0,
			IsVIP:      true,
		}, nil
	}

	prompt := buildPrompt(email, threadCtx, c.cfg.EnableThreadCtx)

	resp, err := c.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return Classification{}, fmt.Errorf("gemini generate: %w", err)
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return Classification{}, fmt.Errorf("empty response from Gemini")
	}

	rawJSON, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
	if !ok {
		return Classification{}, fmt.Errorf("unexpected response type from Gemini")
	}

	var result Classification
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		return Classification{}, fmt.Errorf("unmarshal classification: %w", err)
	}

	c.logger.Info("Classified email",
		zap.String("id", email.ID),
		zap.String("urgency", result.Urgency),
		zap.String("topic", result.Topic),
		zap.Float64("confidence", result.Confidence),
	)

	return result, nil
}

func (c *Classifier) isVIP(from string) bool {
	from = strings.ToLower(from)
	for vip := range c.vipList {
		if strings.Contains(from, vip) {
			return true
		}
	}
	return false
}

func (c *Classifier) Close() {
	c.client.Close()
}

func buildPrompt(email gmail.Email, thread []gmail.Email, includeThread bool) string {
	threadSection := ""
	if includeThread && len(thread) > 1 {
		var sb strings.Builder
		sb.WriteString("\nPrevious messages in this thread (oldest first):\n")
		for i, t := range thread {
			if t.ID == email.ID {
				continue
			}
			sb.WriteString(fmt.Sprintf("--- Message %d ---\nFrom: %s\n%s\n",
				i+1, t.From, truncate(t.Body, 500)))
		}
		threadSection = sb.String()
	}

	return fmt.Sprintf(`You are an expert email triage assistant. Analyze the following email and return a structured JSON classification.

From: %s
Subject: %s
Date: %s
Body:
%s
%s
Classify with:
- urgency: HIGH (needs response within 1h), MEDIUM (within 24h), LOW (can wait)
- topic: billing | support | partnership | legal | hr | marketing | spam | newsletter | personal | other
- sentiment: positive | neutral | negative
- reply_draft: professional 2-3 sentence reply (leave blank for spam/newsletter)
- action_items: list of concrete actions required (empty array if none)
- confidence: 0.0-1.0 confidence score
- is_vip: false (set true only if sender appears to be a key stakeholder)

Respond with JSON only.`,
		email.From,
		email.Subject,
		email.Date.Format(time.RFC1123),
		truncate(email.Body, 2000),
		threadSection,
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}