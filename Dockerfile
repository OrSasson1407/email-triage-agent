# ── Stage 1: Build ──────────────────────────────────
FROM golang:1.22-alpine AS builder

# Install git for go mod download
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Cache dependencies layer separately
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s" \
    -o email-triage-agent \
    ./main.go

# ── Stage 2: Final minimal image (~15 MB) ───────────
FROM alpine:3.19

# ca-certificates: needed for HTTPS calls (Gemini, Slack, etc.)
# tzdata: needed for correct cron scheduling
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/email-triage-agent .

# Copy VIP file if it exists (optional)
COPY --from=builder /app/vip.txt* ./

# Non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["./email-triage-agent"]