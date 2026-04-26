package config

import (
    "fmt"
    "os"
    "strconv"
)

type Config struct {
    GeminiAPIKey      string
    GeminiModel       string
    GeminiMaxTokens   int
    GeminiTimeoutSecs int
    GmailClientID     string
    GmailClientSecret string
    GmailRefreshToken string
    GmailUserEmail    string
    UpstashRedisURL   string
    UpstashRedisToken string
    RedisTTLHours     int
    RedisURL          string
    PostgresURL       string
    PostgresMaxConns  int
    SlackWebhookURL   string
    SlackChannel      string
    SMTPHost          string
    SMTPPort          int
    SMTPUser          string
    SMTPPass          string
    DigestRecipient   string
    PollInterval      string
    MaxEmailsPerRun   int
    WorkerCount       int
    DigestCron        string
    APIPort           string
    APISecret         string
    UrgencyHighConf   float64
    AutoDraftConf     float64
    AutoArchiveConf   float64
    EnableThreadCtx   bool
    EnableVIPFastPath bool
    DryRun            bool
}

func Load() (Config, error) {
    cfg := Config{
        GeminiAPIKey:      require("GEMINI_API_KEY"),
        GeminiModel:       getOr("GEMINI_MODEL", "gemini-1.5-flash"),
        GeminiMaxTokens:   intOr("GEMINI_MAX_TOKENS", 1000),
        GeminiTimeoutSecs: intOr("GEMINI_TIMEOUT_SECONDS", 30),
        GmailClientID:     require("GMAIL_CLIENT_ID"),
        GmailClientSecret: require("GMAIL_CLIENT_SECRET"),
        GmailRefreshToken: require("GMAIL_REFRESH_TOKEN"),
        GmailUserEmail:    require("GMAIL_USER_EMAIL"),
        UpstashRedisURL:   os.Getenv("UPSTASH_REDIS_REST_URL"),
        UpstashRedisToken: os.Getenv("UPSTASH_REDIS_REST_TOKEN"),
        RedisTTLHours:     intOr("REDIS_TTL_HOURS", 72),
        RedisURL:          getOr("REDIS_URL", "upstash"),
        PostgresURL:       require("POSTGRES_URL"),
        PostgresMaxConns:  intOr("POSTGRES_MAX_CONNS", 3),
        SlackWebhookURL:   require("SLACK_WEBHOOK_URL"),
        SlackChannel:      getOr("SLACK_CHANNEL", "#email-urgent"),
        SMTPHost:          getOr("SMTP_HOST", "smtp.gmail.com"),
        SMTPPort:          intOr("SMTP_PORT", 587),
        SMTPUser:          os.Getenv("SMTP_USER"),
        SMTPPass:          os.Getenv("SMTP_PASS"),
        DigestRecipient:   os.Getenv("DIGEST_RECIPIENT"),
        PollInterval:      getOr("POLL_INTERVAL", "@every 48m"),
        MaxEmailsPerRun:   intOr("MAX_EMAILS_PER_RUN", 40),
        WorkerCount:       intOr("WORKER_COUNT", 3),
        DigestCron:        getOr("DIGEST_CRON", "0 18 * * *"),
        APIPort:           getOr("API_PORT", "8080"),
        APISecret:         getOr("API_SECRET", "changeme"),
        UrgencyHighConf:   floatOr("URGENCY_HIGH_CONFIDENCE", 0.80),
        AutoDraftConf:     floatOr("AUTO_DRAFT_CONFIDENCE", 0.90),
        AutoArchiveConf:   floatOr("AUTO_ARCHIVE_CONFIDENCE", 0.95),
        EnableThreadCtx:   boolOr("ENABLE_THREAD_CONTEXT", true),
        EnableVIPFastPath: boolOr("ENABLE_VIP_FAST_PATH", true),
        DryRun:            boolOr("DRY_RUN", false),
    }
    return cfg, nil
}

func require(key string) string {
    v := os.Getenv(key)
    if v == "" { fmt.Printf("WARNING: %s is not set\n", key) }
    return v
}
func getOr(key, def string) string {
    if v := os.Getenv(key); v != "" { return v }
    return def
}
func intOr(key string, def int) int {
    if v := os.Getenv(key); v != "" {
        if i, err := strconv.Atoi(v); err == nil { return i }
    }
    return def
}
func floatOr(key string, def float64) float64 {
    if v := os.Getenv(key); v != "" {
        if f, err := strconv.ParseFloat(v, 64); err == nil { return f }
    }
    return def
}
func boolOr(key string, def bool) bool {
    if v := os.Getenv(key); v != "" {
        if b, err := strconv.ParseBool(v); err == nil { return b }
    }
    return def
}
