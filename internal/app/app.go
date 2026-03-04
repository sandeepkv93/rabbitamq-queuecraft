package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"rabbitamq-queuecraft/internal/config"
	"rabbitamq-queuecraft/internal/httpapi"
	"rabbitamq-queuecraft/internal/mq"
	"rabbitamq-queuecraft/internal/service"
	"rabbitamq-queuecraft/internal/store"
	"rabbitamq-queuecraft/internal/worker"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type App struct {
	cfg      config.Config
	logger   *slog.Logger
	db       *sql.DB
	mqClient *mq.RabbitMQ
	server   *http.Server
	worker   *worker.Runner
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	db, err := connectPostgresWithRetry(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	mqClient, err := connectWithRetry(ctx, cfg, logger)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	repo := store.NewPostgresTicketStore(db)
	if err := repo.EnsureSchema(ctx); err != nil {
		_ = mqClient.Close()
		_ = db.Close()
		return nil, fmt.Errorf("ensure ticket schema: %w", err)
	}

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

	return &App{cfg: cfg, logger: logger, db: db, mqClient: mqClient, server: server, worker: wk}, nil
}

func (a *App) Run(ctx context.Context) error {
	type componentResult struct {
		name string
		err  error
	}

	componentCount := 0
	if a.cfg.EnableAPI() {
		componentCount++
	}
	if a.cfg.EnableWorker() {
		componentCount++
	}

	if componentCount == 0 {
		a.logger.Info("no components enabled for selected mode", "mode", a.cfg.Mode)
		return nil
	}

	a.logger.Info(
		"app mode started",
		"mode",
		a.cfg.Mode,
		"api_enabled",
		a.cfg.EnableAPI(),
		"worker_enabled",
		a.cfg.EnableWorker(),
		"shutdown_timeout",
		a.cfg.ShutdownTimeout,
	)

	resultCh := make(chan componentResult, componentCount)
	var wg sync.WaitGroup
	startComponent := func(name string, run func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resultCh <- componentResult{name: name, err: run()}
		}()
	}

	if a.cfg.EnableWorker() {
		startComponent("worker", func() error {
			return a.worker.Run(ctx)
		})
	}

	if a.cfg.EnableAPI() {
		startComponent("api", func() error {
			a.logger.Info("http server started", "addr", a.cfg.HTTPAddr)
			if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("listen and serve: %w", err)
			}
			a.logger.Info("http server stopped")
			return nil
		})
	}

	remaining := componentCount
	shutdownStarted := false
	select {
	case <-ctx.Done():
		shutdownStarted = true
	default:
	}

	for remaining > 0 {
		if shutdownStarted {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
			if a.cfg.EnableAPI() {
				a.logger.Info("initiating graceful shutdown", "mode", a.cfg.Mode)
				if err := a.server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
					cancel()
					return fmt.Errorf("shutdown http server: %w", err)
				}
			}

			waitCh := make(chan struct{})
			go func() {
				wg.Wait()
				close(waitCh)
			}()

			select {
			case <-waitCh:
				cancel()
				a.logger.Info("graceful shutdown complete", "mode", a.cfg.Mode)
				return nil
			case <-shutdownCtx.Done():
				cancel()
				return fmt.Errorf("shutdown timeout exceeded after %s", a.cfg.ShutdownTimeout)
			}
		}

		select {
		case <-ctx.Done():
			shutdownStarted = true
		case result := <-resultCh:
			remaining--
			if result.err != nil {
				return fmt.Errorf("%s exited with error: %w", result.name, result.err)
			}
		}
	}

	a.logger.Info("all components stopped", "mode", a.cfg.Mode)
	return nil
}

func (a *App) Close() error {
	var closeErr error
	if a.mqClient != nil {
		if err := a.mqClient.Close(); err != nil {
			closeErr = err
		}
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func connectWithRetry(ctx context.Context, cfg config.Config, logger *slog.Logger) (*mq.RabbitMQ, error) {
	var lastErr error
	for attempt := 1; attempt <= cfg.AMQPMaxRetries; attempt++ {
		client, err := mq.New(cfg.AMQPURL, mq.TopologyConfig{
			QueueName:  cfg.QueueName,
			MaxRetries: cfg.AMQPMsgMaxRetries,
			RetryDelay: cfg.AMQPMsgRetryDelay,
		})
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

func connectPostgresWithRetry(ctx context.Context, cfg config.Config, logger *slog.Logger) (*sql.DB, error) {
	var lastErr error
	for attempt := 1; attempt <= cfg.DBMaxRetries; attempt++ {
		db, err := sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			lastErr = err
		} else {
			pingErr := db.PingContext(ctx)
			if pingErr == nil {
				logger.Info("connected to postgres", "attempt", attempt)
				return db, nil
			}
			lastErr = pingErr
			_ = db.Close()
		}

		logger.Warn("postgres connection failed", "attempt", attempt, "max_retries", cfg.DBMaxRetries, "error", lastErr)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(cfg.DBRetryBackoff):
		}
	}
	return nil, fmt.Errorf("connect postgres after retries: %w", lastErr)
}
