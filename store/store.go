package store

import (
    "context"
    "fmt"
    "strings"
    "sync"

    "email-triage-agent/config"
)

type Store interface {
    IsProcessed(ctx context.Context, id string) (bool, error)
    MarkProcessed(ctx context.Context, id string) error
}

func New(cfg config.Config) Store {
    if strings.ToLower(cfg.RedisURL) == "memory" || cfg.UpstashRedisURL == "" {
        return &MemoryStore{seen: make(map[string]bool)}
    }
    return &UpstashStore{url: cfg.UpstashRedisURL, token: cfg.UpstashRedisToken, ttl: cfg.RedisTTLHours * 3600}
}

type MemoryStore struct {
    mu   sync.RWMutex
    seen map[string]bool
}
func (s *MemoryStore) IsProcessed(_ context.Context, id string) (bool, error) {
    s.mu.RLock(); defer s.mu.RUnlock(); return s.seen[id], nil
}
func (s *MemoryStore) MarkProcessed(_ context.Context, id string) error {
    s.mu.Lock(); defer s.mu.Unlock(); s.seen[id] = true; return nil
}

type UpstashStore struct{ url, token string; ttl int }
func (s *UpstashStore) IsProcessed(ctx context.Context, id string) (bool, error) {
    body, err := upstashDo(ctx, s.url, s.token, "GET", fmt.Sprintf("processed:%s", id))
    if err != nil { return false, err }
    return strings.Contains(body, `"result":"1"`), nil
}
func (s *UpstashStore) MarkProcessed(ctx context.Context, id string) error {
    _, err := upstashDo(ctx, s.url, s.token, "SET",
        fmt.Sprintf("processed:%s", id), "1", "EX", fmt.Sprintf("%d", s.ttl))
    return err
}
