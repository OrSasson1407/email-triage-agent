# 📧 Smart Email Triage Agent

> Automatically classify, prioritize, and action your Gmail inbox using AI — 100% free stack, $0/month forever.

**Stack:** Go 1.22 · Gemini AI · Gmail API · Upstash Redis · Neon PostgreSQL · Slack · Railway

---

## How It Works

```
Gmail (unread) → Redis dedup check → Gemini AI classify
    → PostgreSQL log → Gmail (mark/archive/draft) → Slack alert
```

Every 48 minutes the agent:
1. Fetches up to 40 unread emails from Gmail
2. Skips already-processed IDs (Redis cache)
3. Classifies each email with Gemini AI (urgency / topic / sentiment / draft reply)
4. Logs results to PostgreSQL
5. Marks emails as read, archives LOW urgency, saves draft replies
6. Sends Slack alerts for HIGH urgency emails
7. Sends a daily digest at 6pm via Slack + email

---

## Free Tier Usage

| Service | Free Limit | This Project Uses |
|---------|-----------|-------------------|
| Gemini AI | 1,500 req/day | ~1,200/day |
| Gmail API | 1B quota units/day | ~24K/day |
| Upstash Redis | 10,000 cmd/day | ~2,400/day |
| Neon PostgreSQL | 512 MB storage | ~1.5 MB/day |
| Slack Webhooks | Unlimited | ~50/day |
| Railway Hosting | $5 credit/month | ~$0.50/month |

**Total: $0.00/month**

---

## Quick Start

### 1. Get Free API Keys

| Service | URL | What to get |
|---------|-----|-------------|
| Gemini AI | [aistudio.google.com](https://aistudio.google.com) | API Key |
| Gmail | [console.cloud.google.com](https://console.cloud.google.com) | OAuth2 Client ID + Secret |
| Upstash | [upstash.com](https://upstash.com) | REST URL + Token |
| Neon | [neon.tech](https://neon.tech) | Connection string |
| Slack | [api.slack.com/apps](https://api.slack.com/apps) | Webhook URL |

### 2. Configure

```powershell
Copy-Item .env.example .env
# Edit .env with your values
notepad .env
```

### 3. Generate Gmail Token

```powershell
go run scripts/setup_oauth.go
# Opens browser → sign in → paste GMAIL_REFRESH_TOKEN into .env
```

### 4. Run DB Migration

```powershell
go run scripts/migrate.go
```

### 5. Test Locally

```powershell
# Dry run — classifies emails but does NOT modify Gmail
$env:DRY_RUN="true"
go run main.go
```

### 6. Push to GitHub

```powershell
git init -b main
git add .
git commit -m "Initial commit: Email Triage Agent"
git remote add origin https://github.com/OrSasson1407/email-triage-agent.git
git push -u origin main
```

### 7. Deploy to Railway

1. Go to [railway.app](https://railway.app) → New Project
2. Deploy from GitHub → select `email-triage-agent`
3. Variables → add all values from your `.env`
4. Deploy → live in ~30 seconds

---

## Project Structure

```
email-triage-agent/
├── main.go                     # Entry point
├── go.mod / go.sum             # Go modules
├── Dockerfile                  # Railway / Docker deploy
├── .env.example                # All config variables documented
├── config/
│   ├── config.go               # Env loader
│   └── vip.go                  # VIP sender list
├── ai/
│   ├── classifier.go           # Gemini AI classification
│   └── classifier_test.go
├── gmail/
│   └── client.go               # Gmail fetch/read/draft/archive
├── store/
│   ├── store.go                # Redis dedup cache + memory fallback
│   ├── upstash.go              # Upstash REST client
│   └── store_test.go
├── db/
│   ├── db.go                   # Neon PostgreSQL triage log
│   └── db_test.go
├── notify/
│   ├── slack.go                # Slack Block Kit alerts + digest
│   ├── slack_test.go
│   └── email.go                # HTML email digest via SMTP
├── scheduler/
│   └── scheduler.go            # Cron jobs + worker pipeline
├── api/
│   ├── server.go               # HTTP API + auth middleware
│   └── dashboard.go            # Built-in web dashboard
├── retry/
│   ├── retry.go                # Exponential backoff
│   └── retry_test.go
└── scripts/
    ├── setup_oauth.go          # Gmail OAuth token generator
    └── migrate.go              # DB migration runner
```

---

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | None | Health check |
| POST | `/trigger` | Secret | Manual pipeline run |
| GET | `/stats` | Secret | Runtime + 24h metrics |
| GET | `/logs` | Secret | Recent 50 triage logs |
| GET | `/dashboard` | Secret | Web dashboard UI |

Auth: `X-API-Secret: your_secret` header or `?secret=your_secret` query param.

---

## Dashboard

Visit `https://your-app.railway.app/dashboard?secret=YOUR_SECRET`

Shows real-time stats and recent email classifications.

---

## Configuration Reference

See `.env.example` — every variable is documented with defaults and setup instructions.

Key tuning variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `POLL_INTERVAL` | `@every 48m` | How often to check Gmail |
| `MAX_EMAILS_PER_RUN` | `40` | Max emails per poll |
| `WORKER_COUNT` | `3` | Parallel Gemini calls |
| `DRY_RUN` | `false` | Classify only, don't modify Gmail |
| `ENABLE_VIP_FAST_PATH` | `true` | Skip Gemini for VIP senders |
| `VIP_SENDERS` | `` | Comma-separated VIP emails/domains |

---

## Running Tests

```powershell
# All tests (unit only — no external services needed)
go test ./...

# With DB tests (requires POSTGRES_URL)
$env:POSTGRES_URL="your_neon_url"
go test ./... -v

# Single package
go test ./store/... -v
go test ./retry/... -v
```

---

## Upgrade Path

When you outgrow free limits:

| Service | Paid Plan | Cost | What you get |
|---------|-----------|------|-------------|
| Gemini | Pay-as-you-go | ~$1-3/mo | 10x more requests |
| Upstash | Pro | $10/mo | 500K cmd/day |
| Neon | Pro | $19/mo | 10 GB storage |
| Railway | Pro | $20/mo | Dedicated resources |

---

**Total free tier cost: $0.00/month** · Ship it today.