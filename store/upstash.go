package store

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
)

func upstashDo(ctx context.Context, baseURL, token string, args ...string) (string, error) {
    parts := make([]string, len(args))
    for i, a := range args { parts[i] = url.PathEscape(a) }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet,
        baseURL+"/"+strings.Join(parts, "/"), nil)
    if err != nil { return "", fmt.Errorf("upstash request: %w", err) }
    req.Header.Set("Authorization", "Bearer "+token)
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return "", fmt.Errorf("upstash http: %w", err) }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    return string(body), nil
}
