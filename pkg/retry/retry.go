package retry

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/rs/zerolog"
)

// sleepFn is the function used to wait between retries.
var sleepFn = func(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// Do runs fn with exponential backoff + jitter until it succeeds or ctx cancels.
func Do(ctx context.Context, log zerolog.Logger, name string, maxAttempts int, fn func() error) error {
	if maxAttempts <= 0 {
		return fmt.Errorf("%s: maxAttempts must be > 0", name)
	}

	backoff := 500 * time.Millisecond
	const maxBackoff = 30 * time.Second

	var lastErr error
	for i := 1; i <= maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		jitter := time.Duration(rand.Int63n(int64(backoff/2) + 1))
		wait := backoff + jitter

		log.Warn().
			Err(lastErr).
			Str("component", name).
			Int("attempt", i).
			Dur("wait", wait).
			Msg("retrying")

		if err := sleepFn(ctx, wait); err != nil {
			return err
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	return fmt.Errorf("%s: retry exhausted: %w", name, lastErr)
}
