package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"rabbitamq-queuecraft/internal/config"
	"rabbitamq-queuecraft/internal/httpapi"
	"rabbitamq-queuecraft/internal/mq"
	"rabbitamq-queuecraft/internal/service"
	"rabbitamq-queuecraft/internal/store"
	"rabbitamq-queuecraft/internal/worker"
)

type App struct {
	cfg      config.Config
	logger   *slog.Logger
	mqClient *mq.RabbitMQ
	server   *http.Server
	worker   *worker.Runner
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	mqClient, err := connectWithRetry(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	repo := store.NewMemoryTicketStore()
	svc := service.NewTicketService(repo, mqClient, cfg.QueueName, time.Duration(cfg.WorkerSleepMs)*time.Millisecond, logger)

	apiHandler := httpapi.NewHandler(svc)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiHandler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	wk := &worker.Runner{
		Consumer: mqClient,
		Service:  svc,
		Queue:    cfg.QueueName,
		Logger:   logger,
	}

	return &App{cfg: cfg, logger: logger, mqClient: mqClient, server: server, worker: wk}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 2)

	if a.cfg.EnableWorker() {
		go func() {
			errCh <- a.worker.Run(ctx)
		}()
	}

	if a.cfg.EnableAPI() {
		go func() {
			a.logger.Info("http server started", "addr", a.cfg.HTTPAddr)
			if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("listen and serve: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	if !a.cfg.EnableAPI() && !a.cfg.EnableWorker() {
		return nil
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		defer cancel()

		if a.cfg.EnableAPI() {
			_ = a.server.Shutdown(shutdownCtx)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func (a *App) Close() error {
	if a.mqClient != nil {
		return a.mqClient.Close()
	}
	return nil
}

func connectWithRetry(ctx context.Context, cfg config.Config, logger *slog.Logger) (*mq.RabbitMQ, error) {
	var lastErr error
	for attempt := 1; attempt <= cfg.AMQPMaxRetries; attempt++ {
		client, err := mq.New(cfg.AMQPURL)
		if err == nil {
			logger.Info("connected to rabbitmq", "attempt", attempt)
			return client, nil
		}

		lastErr = err
		logger.Warn("rabbitmq connection failed", "attempt", attempt, "max_retries", cfg.AMQPMaxRetries, "error", err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(cfg.AMQPRetryBackoff):
		}
	}
	return nil, fmt.Errorf("connect rabbitmq after retries: %w", lastErr)
}
