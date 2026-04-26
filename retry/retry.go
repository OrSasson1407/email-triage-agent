package retry

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Config holds retry settings
type Config struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

// Default is a sensible retry config for API calls
var Default = Config{
	MaxAttempts: 3,
	InitialWait: 1 * time.Second,
	MaxWait:     30 * time.Second,
	Multiplier:  2.0,
}

// GeminiConfig is tuned for Gemini rate limits
var GeminiConfig = Config{
	MaxAttempts: 3,
	InitialWait: 5 * time.Second,
	MaxWait:     60 * time.Second,
	Multiplier:  2.0,
}

// Do runs fn with exponential backoff retries
// It stops retrying if ctx is cancelled or fn returns a non-retryable error
func Do(ctx context.Context, cfg Config, logger *zap.Logger, name string, fn func() error) error {
	wait := cfg.InitialWait

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		// Don't retry if context cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("%s cancelled: %w", name, ctx.Err())
		}

		// Don't retry if this is the last attempt
		if attempt == cfg.MaxAttempts {
			return fmt.Errorf("%s failed after %d attempts: %w", name, cfg.MaxAttempts, err)
		}

		// Log and wait before retrying
		if logger != nil {
			logger.Warn("Retrying after error",
				zap.String("op", name),
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", cfg.MaxAttempts),
				zap.Duration("wait", wait),
				zap.Error(err),
			)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%s cancelled during backoff: %w", name, ctx.Err())
		case <-time.After(wait):
		}

		// Exponential backoff with cap
		wait = time.Duration(float64(wait) * cfg.Multiplier)
		if wait > cfg.MaxWait {
			wait = cfg.MaxWait
		}
	}

	return nil
}

// DoWithResult is like Do but for functions that return a value
func DoWithResult[T any](ctx context.Context, cfg Config, logger *zap.Logger, name string, fn func() (T, error)) (T, error) {
	var result T
	err := Do(ctx, cfg, logger, name, func() error {
		var e error
		result, e = fn()
		return e
	})
	return result, err
}