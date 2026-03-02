package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"rabbitamq-queuecraft/internal/app"
	"rabbitamq-queuecraft/internal/config"

	"github.com/joho/godotenv"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("initialize app", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := application.Close(); err != nil {
			logger.Error("close app", "error", err)
		}
	}()

	if err := application.Run(ctx); err != nil {
		logger.Error("run app", "error", err)
		os.Exit(1)
	}
}
