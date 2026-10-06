package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	rabbitadapter "fleet-management-system/internal/adapter/rabbitmq"
	"fleet-management-system/internal/config"
	"fleet-management-system/pkg/logger"
	"fleet-management-system/pkg/retry"
)

func main() {
	cfgPath := os.Getenv("TJ_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.App.LogLevel)
	log.Info().Msg("starting geofence worker")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for {
		err := run(ctx, cfg, log)
		if err == nil || errors.Is(err, context.Canceled) {
			log.Info().Msg("worker stopped")
			return
		}
		log.Error().Err(err).Msg("consumer error, reconnecting in 5s")
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			log.Info().Msg("worker stopped")
			return
		}
	}
}

func run(ctx context.Context, cfg *config.Config, log zerolog.Logger) error {
	var consumer *rabbitadapter.Consumer
	err := retry.Do(ctx, log, "rabbitmq-consumer", 10, func() error {
		var e error
		consumer, e = rabbitadapter.NewConsumer(
			cfg.RabbitMQ.URL,
			cfg.RabbitMQ.Exchange,
			cfg.RabbitMQ.ExchangeType,
			cfg.RabbitMQ.Queue,
			cfg.RabbitMQ.RoutingKey,
			cfg.RabbitMQ.DLXExchange,
			cfg.RabbitMQ.DLQQueue,
			log,
		)
		return e
	})
	if err != nil {
		return err
	}
	defer consumer.Close()

	log.Info().Str("queue", cfg.RabbitMQ.Queue).Msg("worker started, consuming")
	return consumer.Consume(ctx)
}
