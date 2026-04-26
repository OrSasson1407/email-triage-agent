package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"email-triage-agent/config"
	"email-triage-agent/store"
)

// ── MemoryStore tests (no Redis needed) ─────────────────────

func TestMemoryStore_IsProcessed_NotFound(t *testing.T) {
	cfg := config.Config{RedisURL: "memory"}
	s := store.New(cfg)
	defer s.Close()

	ctx := context.Background()
	found, err := s.IsProcessed(ctx, "email-123")

	require.NoError(t, err)
	assert.False(t, found, "should not be found before marking")
}

func TestMemoryStore_MarkAndCheck(t *testing.T) {
	cfg := config.Config{RedisURL: "memory"}
	s := store.New(cfg)
	defer s.Close()

	ctx := context.Background()
	id := "email-abc"

	// Not processed yet
	found, err := s.IsProcessed(ctx, id)
	require.NoError(t, err)
	assert.False(t, found)

	// Mark as processed
	err = s.MarkProcessed(ctx, id)
	require.NoError(t, err)

	// Now should be found
	found, err = s.IsProcessed(ctx, id)
	require.NoError(t, err)
	assert.True(t, found)
}

func TestMemoryStore_MultipleIDs(t *testing.T) {
	cfg := config.Config{RedisURL: "memory"}
	s := store.New(cfg)
	defer s.Close()

	ctx := context.Background()
	ids := []string{"id-1", "id-2", "id-3", "id-4", "id-5"}

	// Mark odd ones
	for i, id := range ids {
		if i%2 == 0 {
			require.NoError(t, s.MarkProcessed(ctx, id))
		}
	}

	// Check all
	for i, id := range ids {
		found, err := s.IsProcessed(ctx, id)
		require.NoError(t, err)
		if i%2 == 0 {
			assert.True(t, found, "id %s should be marked", id)
		} else {
			assert.False(t, found, "id %s should not be marked", id)
		}
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	cfg := config.Config{RedisURL: "memory"}
	s := store.New(cfg)
	defer s.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// 50 goroutines writing simultaneously
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-email-%d", n)
			if err := s.MarkProcessed(ctx, id); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent write error: %v", err)
	}

	// Verify all were written
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("concurrent-email-%d", i)
		found, err := s.IsProcessed(ctx, id)
		require.NoError(t, err)
		assert.True(t, found, "id %s should be marked", id)
	}
}

func TestMemoryStore_IdempotentMark(t *testing.T) {
	cfg := config.Config{RedisURL: "memory"}
	s := store.New(cfg)
	defer s.Close()

	ctx := context.Background()
	id := "idempotent-test"

	// Mark twice — should not error
	require.NoError(t, s.MarkProcessed(ctx, id))
	require.NoError(t, s.MarkProcessed(ctx, id))

	found, err := s.IsProcessed(ctx, id)
	require.NoError(t, err)
	assert.True(t, found)
}

// ── Store factory tests ──────────────────────────────────────

func TestNew_ReturnsMemoryStore_WhenRedisURLIsMemory(t *testing.T) {
	cfg := config.Config{RedisURL: "memory"}
	s := store.New(cfg)
	defer s.Close()

	// Should work without any Redis config
	ctx := context.Background()
	err := s.MarkProcessed(ctx, "test-id")
	assert.NoError(t, err)
}

func TestNew_ReturnsMemoryStore_WhenUpstashURLEmpty(t *testing.T) {
	cfg := config.Config{
		UpstashRedisURL: "", // empty — fallback to memory
	}
	s := store.New(cfg)
	defer s.Close()

	ctx := context.Background()
	err := s.MarkProcessed(ctx, "test-id")
	assert.NoError(t, err)
}