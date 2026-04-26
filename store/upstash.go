package store

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// upstashDo executes a Redis command via Upstash REST API
// Example: upstashDo(ctx, url, token, "SET", "key", "value", "EX", "3600")
func upstashDo(ctx context.Context, baseURL, token string, args ...string) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("upstash URL not configured")
	}

	// Build path from args: /SET/key/value/EX/3600
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = url.PathEscape(a)
	}
	endpoint := baseURL + "/" + strings.Join(parts, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("upstash build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upstash http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("upstash read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upstash status %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}