package ai

import (
    "context"
    "encoding/json"
    "fmt"
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
}

type Classifier struct {
    client *genai.Client
    model  *genai.GenerativeModel
    logger *zap.Logger
    cfg    config.Config
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
        },
    }
    return &Classifier{client: client, model: model, logger: logger, cfg: cfg}, nil
}

func (c *Classifier) Classify(ctx context.Context, email gmail.Email) (Classification, error) {
    ctx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.GeminiTimeoutSecs)*time.Second)
    defer cancel()
    resp, err := c.model.GenerateContent(ctx, genai.Text(buildPrompt(email)))
    if err != nil {
        return Classification{}, fmt.Errorf("gemini generate: %w", err)
    }
    if len(resp.Candidates) == 0 {
        return Classification{}, fmt.Errorf("empty response from Gemini")
    }
    rawJSON, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
    if !ok {
        return Classification{}, fmt.Errorf("unexpected response type")
    }
    var result Classification
    if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
        return Classification{}, fmt.Errorf("unmarshal: %w", err)
    }
    c.logger.Debug("Classified", zap.String("id", email.ID), zap.String("urgency", result.Urgency))
    return result, nil
}

func (c *Classifier) Close() { c.client.Close() }

func buildPrompt(email gmail.Email) string {
    return fmt.Sprintf(`You are an expert email triage assistant. Analyze this email and return structured JSON.

From: %s
Subject: %s
Body:
%s

Classify with:
- urgency: HIGH (respond within 1h) / MEDIUM (within 24h) / LOW (can wait)
- topic: billing|support|partnership|legal|hr|marketing|spam|newsletter|personal|other
- sentiment: positive|neutral|negative
- reply_draft: professional 2-3 sentence reply (blank for spam/newsletter)
- action_items: list of concrete actions (empty array if none)
- confidence: 0.0-1.0

Respond with JSON only.`, email.From, email.Subject, truncate(email.Body, 2000))
}

func truncate(s string, max int) string {
    if len(s) <= max { return s }
    return s[:max] + "...[truncated]"
}
