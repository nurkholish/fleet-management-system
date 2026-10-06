package rabbitmq

import (
	"context"
	"os"
	"testing"
	"time"

	"fleet-management-system/internal/domain"
)

func rabbitURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TJ_RABBITMQ_TEST_URL")
	if url == "" {
		t.Skip("TJ_RABBITMQ_TEST_URL not set, skipping integration test")
	}
	return url
}

// uniqueNames returns a suffix to avoid clashing between test runs.
func uniqueNames(suffix string) (exchange, queue, key, dlx, dlq string) {
	exchange = "test.events." + suffix
	queue = "test_queue." + suffix
	key = "test.key." + suffix
	dlx = exchange + ".dlx"
	dlq = queue + ".dlq"
	return
}

func TestPublisher_Integration(t *testing.T) {
	url := rabbitURL(t)
	exchange, queue, key, dlx, dlq := uniqueNames("basic")

	pub, err := NewPublisher(url, exchange, "topic", queue, key, dlx, dlq)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evt := domain.GeofenceEvent{
		VehicleID: "B1234XYZ",
		Event:     "geofence_entry",
		Location:  domain.GeoPoint{Latitude: -6.1754, Longitude: 106.8272},
		Timestamp: time.Now().Unix(),
	}
	if err := pub.PublishGeofenceEntry(ctx, evt); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestPublisher_PublishAfterClose_ReturnsError(t *testing.T) {
	url := rabbitURL(t)
	exchange, queue, key, dlx, dlq := uniqueNames("closed")

	pub, err := NewPublisher(url, exchange, "topic", queue, key, dlx, dlq)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	if err := pub.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if pub.IsHealthy() {
		t.Fatal("expected publisher unhealthy after close")
	}

	err = pub.PublishGeofenceEntry(context.Background(), domain.GeofenceEvent{})
	if err == nil {
		t.Fatal("expected error publishing after close")
	}
}

func TestPublisher_ConfirmMode(t *testing.T) {
	url := rabbitURL(t)
	exchange, queue, key, dlx, dlq := uniqueNames("confirm")

	pub, err := NewPublisher(url, exchange, "topic", queue, key, dlx, dlq)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		evt := domain.GeofenceEvent{
			VehicleID: "B1234XYZ",
			Event:     "geofence_entry",
			Timestamp: time.Now().Unix() + int64(i),
		}
		if err := pub.PublishGeofenceEntry(ctx, evt); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}
}

func TestPublisher_ContextCancelled(t *testing.T) {
	url := rabbitURL(t)
	exchange, queue, key, dlx, dlq := uniqueNames("cancel")

	pub, err := NewPublisher(url, exchange, "topic", queue, key, dlx, dlq)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer pub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	err = pub.PublishGeofenceEntry(ctx, domain.GeofenceEvent{
		VehicleID: "B1234XYZ",
		Event:     "geofence_entry",
		Timestamp: time.Now().Unix(),
	})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}
