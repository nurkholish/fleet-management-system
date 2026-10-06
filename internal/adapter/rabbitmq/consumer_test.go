package rabbitmq

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
)

func TestConsumer_Integration(t *testing.T) {
	url := rabbitURL(t)
	exchange, queue, key, dlx, dlq := uniqueNames("consume")

	// Publish 1 event first.
	pub, err := NewPublisher(url, exchange, "topic", queue, key, dlx, dlq)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pub.PublishGeofenceEntry(ctx, domain.GeofenceEvent{
		VehicleID: "B1234XYZ",
		Event:     "geofence_entry",
		Timestamp: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Consume.
	cons, err := NewConsumer(url, exchange, "topic", queue, key, dlx, dlq, zerolog.Nop())
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}
	defer cons.Close()

	consumeCtx, consumeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer consumeCancel()

	done := make(chan error, 1)
	go func() { done <- cons.Consume(consumeCtx) }()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("consumer did not return within timeout")
	}
}

// NEW: consumer should work even if queue doesn't exist yet.
func TestConsumer_DeclaresQueueIfMissing(t *testing.T) {
	url := rabbitURL(t)
	exchange, queue, key, dlx, dlq := uniqueNames("declare")

	// No publisher called first — queue doesn't exist.
	cons, err := NewConsumer(url, exchange, "topic", queue, key, dlx, dlq, zerolog.Nop())
	if err != nil {
		t.Fatalf("new consumer (should declare queue): %v", err)
	}
	defer cons.Close()
	// If we got here without error, topology was declared successfully.
}
