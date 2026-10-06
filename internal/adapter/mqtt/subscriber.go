package mqtt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
)

type HandlerFunc func(ctx context.Context, loc domain.Location)

type Subscriber struct {
	client    mqtt.Client
	topic     string
	qos       byte
	handler   HandlerFunc
	log       zerolog.Logger
	rootCtx   context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once

	jobs chan domain.Location
	wg   sync.WaitGroup
}

func NewSubscriber(
	rootCtx context.Context,
	broker, clientID, topic string,
	qos byte,
	keepAlive, connectTimeout time.Duration,
	workerPool, jobBuffer int,
	cleanSession bool,
	handler HandlerFunc,
	log zerolog.Logger,
) (*Subscriber, error) {
	if workerPool <= 0 {
		workerPool = 4
	}
	if jobBuffer <= 0 {
		jobBuffer = 1000
	}

	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second).
		SetKeepAlive(keepAlive).
		SetConnectTimeout(connectTimeout).
		SetCleanSession(cleanSession).
		SetOrderMatters(false).
		SetDefaultPublishHandler(func(_ mqtt.Client, _ mqtt.Message) {})

	c := mqtt.NewClient(opts)
	tok := c.Connect()
	if !tok.WaitTimeout(connectTimeout) {
		return nil, errors.New("mqtt connect timeout")
	}
	if err := tok.Error(); err != nil {
		return nil, fmt.Errorf("mqtt connect: %w", err)
	}

	ctx, cancel := context.WithCancel(rootCtx)

	s := &Subscriber{
		client:  c,
		topic:   topic,
		qos:     qos,
		handler: handler,
		log:     log,
		rootCtx: ctx,
		cancel:  cancel,
		jobs:    make(chan domain.Location, jobBuffer),
	}

	for i := 0; i < workerPool; i++ {
		s.wg.Add(1)
		go s.worker()
	}

	if tok := c.Subscribe(topic, qos, s.onMessage); tok.Wait() && tok.Error() != nil {
		cancel()
		return nil, fmt.Errorf("mqtt subscribe: %w", tok.Error())
	}
	log.Info().Str("topic", topic).Int("workers", workerPool).Msg("mqtt subscribed")
	return s, nil
}

func (s *Subscriber) onMessage(_ mqtt.Client, msg mqtt.Message) {
	loc, err := ParseLocation(msg.Topic(), msg.Payload())
	if err != nil {
		s.log.Warn().Err(err).Str("topic", msg.Topic()).Msg("invalid message dropped")
		return
	}

	select {
	case s.jobs <- loc:
	case <-s.rootCtx.Done():
	default:
		s.log.Warn().Str("vehicle", loc.VehicleID).Msg("job queue full, dropping")
	}
}

func (s *Subscriber) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.rootCtx.Done():
			return
		case loc := <-s.jobs:
			s.processOne(loc)
		}
	}
}

func (s *Subscriber) processOne(loc domain.Location) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.handler(ctx, loc)
}

func (s *Subscriber) Close() {
	s.closeOnce.Do(func() {
		s.log.Info().Msg("shutting down mqtt subscriber")

		if tok := s.client.Unsubscribe(s.topic); tok.WaitTimeout(2*time.Second) && tok.Error() != nil {
			s.log.Warn().Err(tok.Error()).Msg("unsubscribe failed")
		}
		s.client.Disconnect(250)

		s.cancel()
		s.wg.Wait()

		s.log.Info().Msg("mqtt subscriber stopped")
	})
}

// ParseLocation validates the payload AND cross-checks that the vehicle_id
// embedded in the topic matches the one in the JSON body.
func ParseLocation(topic string, payload []byte) (domain.Location, error) {
	if len(payload) == 0 {
		return domain.Location{}, errors.New("empty payload")
	}
	if len(payload) > 1024 {
		return domain.Location{}, fmt.Errorf("payload too large: %d bytes", len(payload))
	}

	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()

	var loc domain.Location
	if err := dec.Decode(&loc); err != nil {
		return domain.Location{}, fmt.Errorf("invalid json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.Location{}, errors.New("unexpected trailing data")
	}
	if err := loc.Validate(); err != nil {
		return domain.Location{}, fmt.Errorf("validation: %w", err)
	}

	if topicID := extractVehicleIDFromTopic(topic); topicID != "" && topicID != loc.VehicleID {
		return domain.Location{}, fmt.Errorf(
			"vehicle_id mismatch: topic=%q payload=%q", topicID, loc.VehicleID)
	}
	return loc, nil
}

// extractVehicleIDFromTopic parses "/fleet/vehicle/{id}/location".
func extractVehicleIDFromTopic(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) != 5 {
		return ""
	}
	if parts[0] != "" || parts[1] != "fleet" || parts[2] != "vehicle" ||
		parts[4] != "location" || parts[3] == "" {
		return ""
	}
	return parts[3]
}
