package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"email-triage-agent/retry"
)

var testLogger, _ = zap.NewDevelopment()

// ── Basic retry tests ────────────────────────────────────────

func TestDo_SucceedsFirstTry(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Default, testLogger, "test-op", func() error {
		calls++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls, "should only call once on success")
}

func TestDo_RetriesOnError(t *testing.T) {
	calls := 0
	expectedErr := errors.New("temporary error")

	cfg := retry.Config{
		MaxAttempts: 3,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     10 * time.Millisecond,
		Multiplier:  2.0,
	}

	err := retry.Do(context.Background(), cfg, testLogger, "test-op", func() error {
		calls++
		if calls < 3 {
			return expectedErr
		}
		return nil // succeed on 3rd attempt
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls, "should retry until success")
}

func TestDo_FailsAfterMaxAttempts(t *testing.T) {
	calls := 0
	expectedErr := errors.New("persistent error")

	cfg := retry.Config{
		MaxAttempts: 3,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     5 * time.Millisecond,
		Multiplier:  2.0,
	}

	err := retry.Do(context.Background(), cfg, testLogger, "test-op", func() error {
		calls++
		return expectedErr
	})

	assert.Error(t, err)
	assert.Equal(t, 3, calls, "should attempt exactly MaxAttempts times")
	assert.Contains(t, err.Error(), "test-op")
	assert.Contains(t, err.Error(), "3 attempts")
}

func TestDo_StopsOnContextCancel(t *testing.T) {
	calls := 0

	cfg := retry.Config{
		MaxAttempts: 10,
		InitialWait: 50 * time.Millisecond,
		MaxWait:     500 * time.Millisecond,
		Multiplier:  2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context after first failure
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := retry.Do(ctx, cfg, testLogger, "test-op", func() error {
		calls++
		return errors.New("always fails")
	})

	assert.Error(t, err)
	assert.Less(t, calls, 10, "should stop before MaxAttempts when context cancelled")
}

func TestDo_NilLogger(t *testing.T) {
	cfg := retry.Config{
		MaxAttempts: 2,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     5 * time.Millisecond,
		Multiplier:  2.0,
	}

	// Should not panic with nil logger
	err := retry.Do(context.Background(), cfg, nil, "test-op", func() error {
		return nil
	})
	assert.NoError(t, err)
}

// ── DoWithResult tests ───────────────────────────────────────

func TestDoWithResult_ReturnsValue(t *testing.T) {
	cfg := retry.Config{
		MaxAttempts: 3,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     5 * time.Millisecond,
		Multiplier:  2.0,
	}

	calls := 0
	result, err := retry.DoWithResult[string](
		context.Background(), cfg, testLogger, "test-op",
		func() (string, error) {
			calls++
			if calls < 2 {
				return "", errors.New("not ready")
			}
			return "success-value", nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, "success-value", result)
	assert.Equal(t, 2, calls)
}

func TestDoWithResult_ReturnsErrorAfterMaxAttempts(t *testing.T) {
	cfg := retry.Config{
		MaxAttempts: 2,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     5 * time.Millisecond,
		Multiplier:  2.0,
	}

	result, err := retry.DoWithResult[int](
		context.Background(), cfg, testLogger, "test-op",
		func() (int, error) {
			return 0, errors.New("always fails")
		},
	)

	assert.Error(t, err)
	assert.Equal(t, 0, result, "zero value on error")
}

// ── Backoff timing tests ─────────────────────────────────────

func TestDo_ExponentialBackoff(t *testing.T) {
	cfg := retry.Config{
		MaxAttempts: 4,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	start := time.Now()
	calls := 0

	retry.Do(context.Background(), cfg, nil, "timing-test", func() error {
		calls++
		return errors.New("always fail")
	})

	elapsed := time.Since(start)
	// Expected waits: 10ms + 20ms + 40ms = 70ms minimum
	assert.GreaterOrEqual(t, elapsed, 60*time.Millisecond,
		"should wait at least ~70ms total across retries")
}

func TestDo_RespectsMaxWait(t *testing.T) {
	cfg := retry.Config{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     15 * time.Millisecond, // cap at 15ms
		Multiplier:  10.0,                  // aggressive multiplier
	}

	start := time.Now()

	retry.Do(context.Background(), cfg, nil, "max-wait-test", func() error {
		return errors.New("always fail")
	})

	elapsed := time.Since(start)
	// Should be capped: 10ms + 15ms = 25ms, not 10ms + 100ms
	assert.Less(t, elapsed, 100*time.Millisecond,
		"should not exceed max wait even with high multiplier")
}

// ── Preset config tests ──────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	assert.Equal(t, 3, retry.Default.MaxAttempts)
	assert.Equal(t, 1*time.Second, retry.Default.InitialWait)
	assert.Equal(t, 30*time.Second, retry.Default.MaxWait)
	assert.InDelta(t, 2.0, retry.Default.Multiplier, 0.001)
}

func TestGeminiConfig(t *testing.T) {
	assert.Equal(t, 3, retry.GeminiConfig.MaxAttempts)
	assert.Equal(t, 5*time.Second, retry.GeminiConfig.InitialWait)
	assert.Equal(t, 60*time.Second, retry.GeminiConfig.MaxWait)
}