package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	memoryadapter "fleet-management-system/internal/adapter/memory"
	mqttadapter "fleet-management-system/internal/adapter/mqtt"
	"fleet-management-system/internal/adapter/postgres"
	rabbitadapter "fleet-management-system/internal/adapter/rabbitmq"
	"fleet-management-system/internal/config"
	"fleet-management-system/internal/domain"
	httptransport "fleet-management-system/internal/transport/http"
	"fleet-management-system/internal/usecase"
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
	log.Info().Str("env", cfg.App.Env).Msg("starting backend")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Infrastructure ---
	pool := mustConnectPG(ctx, cfg, log)
	rmq := mustConnectRabbit(ctx, cfg, log)

	// --- Domain wiring ---
	repo := postgres.NewLocationRepo(pool)
	locUC := usecase.NewLocationUsecase(
		repo,
		int64(cfg.History.MaxRangeSeconds),
		cfg.History.DefaultLimit,
		cfg.History.MaxLimit,
	)

	geofenceState := memoryadapter.NewGeofenceState()

	var pub domain.EventPublisher = rmq
	geoUC := usecase.NewGeofenceUsecase(
		cfg.Geofence.Items,
		pub,
		geofenceState,
		log,
	)

	handler := func(hctx context.Context, loc domain.Location) {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Str("vehicle", loc.VehicleID).
					Msg("panic in handler recovered")
			}
		}()

		if err := locUC.Ingest(hctx, loc); err != nil {
			log.Warn().Err(err).Str("vehicle", loc.VehicleID).Msg("ingest failed")
			return
		}
		geoUC.Evaluate(hctx, loc)
	}

	sub, err := mqttadapter.NewSubscriber(
		ctx,
		cfg.MQTT.Broker,
		cfg.MQTT.ClientID,
		cfg.MQTT.Topic,
		cfg.MQTT.QoS,
		cfg.MQTT.KeepAlive,
		cfg.MQTT.ConnectTimeout,
		cfg.MQTT.WorkerPool,
		cfg.MQTT.JobBuffer,
		cfg.MQTT.CleanSession,
		handler,
		log,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("mqtt init")
	}

	// --- HTTP ---
	router := httptransport.NewRouter(
		httptransport.NewLocationHandler(locUC, log),
		httptransport.NewHealthHandler(pool, rmq),
		httptransport.NewGeofenceHandler(cfg.Geofence.Items, log),
		log,
		cfg.App.Env,
	)
	srv := &http.Server{
		Addr:         ":" + cfg.HTTP.Port,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	go func() {
		log.Info().Str("addr", srv.Addr).Msg("http server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("http server error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutdown signal received")

	// Ordered shutdown: HTTP → MQTT (drain) → RabbitMQ → Postgres.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn().Err(err).Msg("http shutdown")
	}
	log.Info().Msg("http server stopped")

	sub.Close()

	if err := rmq.Close(); err != nil {
		log.Warn().Err(err).Msg("rabbitmq close")
	}
	log.Info().Msg("rabbitmq closed")

	pool.Close()
	log.Info().Msg("postgres closed")

	log.Info().Msg("bye")
}

func mustConnectPG(ctx context.Context, cfg *config.Config, log zerolog.Logger) *pgxpool.Pool {
	var pool *pgxpool.Pool
	err := retry.Do(ctx, log, "postgres", 10, func() error {
		var e error
		pool, e = postgres.NewPool(
			ctx,
			cfg.PostgresDSN(),
			cfg.Postgres.MinConns,
			cfg.Postgres.MaxOpenConns,
			cfg.Postgres.ConnMaxLifetime,
		)
		if e != nil {
			return e
		}
		if e = pool.Ping(ctx); e != nil {
			pool.Close()
			return e
		}
		return nil
	})
	if err != nil {
		log.Fatal().Err(err).Msg("postgres unreachable")
	}
	log.Info().Msg("postgres connected")
	return pool
}

func mustConnectRabbit(ctx context.Context, cfg *config.Config, log zerolog.Logger) *rabbitadapter.Publisher {
	var pub *rabbitadapter.Publisher
	err := retry.Do(ctx, log, "rabbitmq", 10, func() error {
		var e error
		pub, e = rabbitadapter.NewPublisher(
			cfg.RabbitMQ.URL,
			cfg.RabbitMQ.Exchange,
			cfg.RabbitMQ.ExchangeType,
			cfg.RabbitMQ.Queue,
			cfg.RabbitMQ.RoutingKey,
			cfg.RabbitMQ.DLXExchange,
			cfg.RabbitMQ.DLQQueue,
		)
		return e
	})
	if err != nil {
		log.Fatal().Err(err).Msg("rabbitmq unreachable")
	}
	log.Info().Msg("rabbitmq ready")
	return pub
}
