package retry

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// ============================================================
// Test helpers
// ============================================================

// fastSleep replaces sleepFn with a no-op during a test.
// Restores original on cleanup via t.Cleanup.
func fastSleep(t *testing.T) {
	t.Helper()
	orig := sleepFn
	t.Cleanup(func() { sleepFn = orig })
	sleepFn = func(_ context.Context, _ time.Duration) error { return nil }
}

// ============================================================
// Happy path
// ============================================================

func TestDo_SucceedsFirstTry(t *testing.T) {
	fastSleep(t)

	calls := 0
	err := Do(context.Background(), zerolog.Nop(), "test", 5, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDo_SucceedsAfterRetry(t *testing.T) {
	fastSleep(t)

	calls := 0
	err := Do(context.Background(), zerolog.Nop(), "test", 5, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

// ============================================================
// Exhaustion path
// ============================================================

func TestDo_ExhaustsAttempts(t *testing.T) {
	fastSleep(t)

	calls := 0
	err := Do(context.Background(), zerolog.Nop(), "test", 3, func() error {
		calls++
		return errors.New("permanent")
	})
	if err == nil {
		t.Fatal("expected error after exhaustion")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if !strings.Contains(err.Error(), "retry exhausted") {
		t.Errorf("error should mention 'retry exhausted', got: %v", err)
	}
	if !strings.Contains(err.Error(), "test") {
		t.Errorf("error should mention component name, got: %v", err)
	}
}

func TestDo_WrapsLastError(t *testing.T) {
	fastSleep(t)

	sentinel := errors.New("final failure")
	callCount := 0
	err := Do(context.Background(), zerolog.Nop(), "test", 3, func() error {
		callCount++
		if callCount == 3 {
			return sentinel
		}
		return errors.New("transient")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap sentinel, got: %v", err)
	}
}

// ============================================================
// Edge cases
// ============================================================

func TestDo_MaxAttemptsZero(t *testing.T) {
	fastSleep(t)

	calls := 0
	err := Do(context.Background(), zerolog.Nop(), "test", 0, func() error {
		calls++
		return nil
	})
	if err == nil {
		t.Fatal("expected error for maxAttempts=0")
	}
	if calls != 0 {
		t.Errorf("expected 0 calls, got %d", calls)
	}
	if !strings.Contains(err.Error(), "maxAttempts must be > 0") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDo_MaxAttemptsNegative(t *testing.T) {
	fastSleep(t)

	calls := 0
	err := Do(context.Background(), zerolog.Nop(), "test", -5, func() error {
		calls++
		return nil
	})
	if err == nil {
		t.Fatal("expected error for negative maxAttempts")
	}
	if calls != 0 {
		t.Errorf("expected 0 calls, got %d", calls)
	}
}

func TestDo_MaxAttemptsOne(t *testing.T) {
	fastSleep(t)

	calls := 0
	err := Do(context.Background(), zerolog.Nop(), "test", 1, func() error {
		calls++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

// ============================================================
// Context cancellation — Do() level
// ============================================================

func TestDo_ContextCancelledBeforeFirstAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Do(ctx, zerolog.Nop(), "test", 10, func() error {
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestDo_ContextCancelledDuringBackoff uses real sleepFn to verify the
// production code path in sleepFn's select { ctx.Done() / time.After }.
func TestDo_ContextCancelledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel from another goroutine during the first backoff window
	// (500ms + jitter 0-250ms) so the select inside real sleepFn fires.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := Do(ctx, zerolog.Nop(), "test", 100, func() error {
		return errors.New("fail")
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("took too long to respect context cancellation: %v", elapsed)
	}
}

// ============================================================
// sleepFn — direct coverage of the production implementation
// ============================================================

// Test_sleepFn_ReturnsOnContextCancel covers the ctx.Done() branch
// of the production sleepFn directly.
func Test_sleepFn_ReturnsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	start := time.Now()
	err := sleepFn(ctx, 10*time.Second) // long duration, but ctx is already cancelled
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected ctx error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("sleepFn should return immediately on cancelled ctx, took %v", elapsed)
	}
}

// Test_sleepFn_ReturnsAfterDuration covers the time.After branch
// of the production sleepFn.
func Test_sleepFn_ReturnsAfterDuration(t *testing.T) {
	err := sleepFn(context.Background(), 10*time.Millisecond)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// Test_sleepFn_ReturnsOnCancelDuringSleep covers the ctx.Done() branch
// when cancel happens while sleepFn is already sleeping.
func Test_sleepFn_ReturnsOnCancelDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := sleepFn(ctx, 10*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected ctx error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("sleepFn should return promptly on cancel, took %v", elapsed)
	}
}

// ============================================================
// Backoff clamping
// ============================================================

func TestDo_BackoffCap(t *testing.T) {
	orig := sleepFn
	t.Cleanup(func() { sleepFn = orig })

	var (
		mu        sync.Mutex
		durations []time.Duration
	)
	sleepFn = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		durations = append(durations, d)
		mu.Unlock()
		return nil
	}

	_ = Do(context.Background(), zerolog.Nop(), "test", 15, func() error {
		return errors.New("always fail")
	})

	mu.Lock()
	defer mu.Unlock()

	if len(durations) != 15 {
		t.Fatalf("expected 15 backoff durations, got %d", len(durations))
	}

	// First backoff ~500ms + jitter(0-250ms) → [500ms, 750ms]
	if durations[0] < 500*time.Millisecond || durations[0] > 750*time.Millisecond {
		t.Errorf("first backoff out of range: %v", durations[0])
	}

	// Last backoff capped at 30s + jitter(0-15s) → [30s, 45s]
	last := durations[len(durations)-1]
	if last < 30*time.Second || last > 45*time.Second {
		t.Errorf("last backoff should be near maxBackoff, got: %v", last)
	}
}

// ============================================================
// Concurrency
// ============================================================

func TestDo_ConcurrentCallsAreIndependent(t *testing.T) {
	fastSleep(t)

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)

	var successCount int32
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			calls := 0
			err := Do(context.Background(), zerolog.Nop(), "test", 3, func() error {
				calls++
				if calls < 2 {
					return errors.New("transient")
				}
				return nil
			})
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&successCount); got != workers {
		t.Errorf("expected all %d to succeed, got %d", workers, got)
	}
}

// ============================================================
// Benchmarks
// ============================================================

func BenchmarkDo_SucceedsFirstTry(b *testing.B) {
	orig := sleepFn
	sleepFn = func(_ context.Context, _ time.Duration) error { return nil }
	b.Cleanup(func() { sleepFn = orig })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Do(context.Background(), zerolog.Nop(), "test", 5, func() error {
			return nil
		})
	}
}
