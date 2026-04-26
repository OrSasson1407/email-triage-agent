package gmail

import (
    "context"
    "encoding/base64"
    "fmt"
    "strings"
    "time"

    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    "google.golang.org/api/gmail/v1"
    "google.golang.org/api/option"

    "email-triage-agent/config"
)

type Email struct {
    ID       string
    ThreadID string
    From     string
    Subject  string
    Body     string
    Date     time.Time
    Labels   []string
}

type Client struct {
    svc  *gmail.Service
    user string
}

func NewClient(cfg config.Config) (*Client, error) {
    ctx := context.Background()
    oauthCfg := &oauth2.Config{
        ClientID:     cfg.GmailClientID,
        ClientSecret: cfg.GmailClientSecret,
        Endpoint:     google.Endpoint,
        Scopes:       []string{gmail.GmailReadonlyScope, gmail.GmailModifyScope, gmail.GmailComposeScope},
    }
    token := &oauth2.Token{RefreshToken: cfg.GmailRefreshToken, TokenType: "Bearer"}
    svc, err := gmail.NewService(ctx, option.WithTokenSource(oauthCfg.TokenSource(ctx, token)))
    if err != nil {
        return nil, fmt.Errorf("gmail service: %w", err)
    }
    return &Client{svc: svc, user: cfg.GmailUserEmail}, nil
}

func (c *Client) FetchUnread(ctx context.Context, maxCount int) ([]Email, error) {
    res, err := c.svc.Users.Messages.List(c.user).Q("is:unread").MaxResults(int64(maxCount)).Context(ctx).Do()
    if err != nil {
        return nil, fmt.Errorf("list messages: %w", err)
    }
    var emails []Email
    for _, msg := range res.Messages {
        email, err := c.fetchMessage(ctx, msg.Id)
        if err != nil { continue }
        emails = append(emails, email)
    }
    return emails, nil
}

func (c *Client) fetchMessage(ctx context.Context, id string) (Email, error) {
    msg, err := c.svc.Users.Messages.Get(c.user, id).Format("full").Context(ctx).Do()
    if err != nil {
        return Email{}, fmt.Errorf("get message %s: %w", id, err)
    }
    email := Email{ID: msg.Id, ThreadID: msg.ThreadId, Labels: msg.LabelIds}
    for _, h := range msg.Payload.Headers {
        switch h.Name {
        case "From":    email.From = h.Value
        case "Subject": email.Subject = h.Value
        case "Date":    email.Date, _ = time.Parse(time.RFC1123Z, h.Value)
        }
    }
    email.Body = extractBody(msg.Payload)
    return email, nil
}

func (c *Client) MarkProcessed(ctx context.Context, id string) error {
    _, err := c.svc.Users.Messages.Modify(c.user, id, &gmail.ModifyMessageRequest{
        RemoveLabelIds: []string{"UNREAD"},
    }).Context(ctx).Do()
    return err
}

func (c *Client) Archive(ctx context.Context, id string) error {
    _, err := c.svc.Users.Messages.Modify(c.user, id, &gmail.ModifyMessageRequest{
        RemoveLabelIds: []string{"INBOX"},
    }).Context(ctx).Do()
    return err
}

func (c *Client) CreateDraft(ctx context.Context, email Email, body string) error {
    raw := fmt.Sprintf("To: %s\r\nSubject: Re: %s\r\nIn-Reply-To: %s\r\n\r\n%s",
        email.From, email.Subject, email.ID, body)
    _, err := c.svc.Users.Drafts.Create(c.user, &gmail.Draft{
        Message: &gmail.Message{
            Raw:      base64.URLEncoding.EncodeToString([]byte(raw)),
            ThreadId: email.ThreadID,
        },
    }).Context(ctx).Do()
    return err
}

func extractBody(payload *gmail.MessagePart) string {
    if payload == nil { return "" }
    if payload.MimeType == "text/plain" && payload.Body != nil {
        data, _ := base64.URLEncoding.DecodeString(payload.Body.Data)
        return string(data)
    }
    for _, part := range payload.Parts {
        if part.MimeType == "text/plain" && part.Body != nil {
            data, _ := base64.URLEncoding.DecodeString(part.Body.Data)
            if s := strings.TrimSpace(string(data)); s != "" { return s }
        }
    }
    return ""
}
