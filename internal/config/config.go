package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	AMQPURL          string
	QueueName        string
	Mode             string
	ShutdownTimeout  time.Duration
	WorkerSleepMs    int
	DBMaxRetries     int
	DBRetryBackoff   time.Duration
	AMQPMaxRetries   int
	AMQPRetryBackoff time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://postgres:postgres@postgres:5432/tickets?sslmode=disable"),
		AMQPURL:          getEnv("AMQP_URL", "amqp://guest:guest@rabbitmq:5672/"),
		QueueName:        getEnv("AMQP_QUEUE", "tickets.triage"),
		Mode:             strings.ToLower(getEnv("APP_MODE", "all")),
		ShutdownTimeout:  time.Duration(getEnvInt("SHUTDOWN_TIMEOUT_SECONDS", 15)) * time.Second,
		WorkerSleepMs:    getEnvInt("WORKER_SLEEP_MS", 1200),
		DBMaxRetries:     getEnvInt("DB_MAX_RETRIES", 20),
		DBRetryBackoff:   time.Duration(getEnvInt("DB_RETRY_BACKOFF_SECONDS", 2)) * time.Second,
		AMQPMaxRetries:   getEnvInt("AMQP_MAX_RETRIES", 20),
		AMQPRetryBackoff: time.Duration(getEnvInt("AMQP_RETRY_BACKOFF_SECONDS", 2)) * time.Second,
	}

	switch cfg.Mode {
	case "all", "api", "worker":
	default:
		return Config{}, fmt.Errorf("invalid APP_MODE %q, expected one of: all, api, worker", cfg.Mode)
	}

	if cfg.AMQPMaxRetries < 1 {
		return Config{}, fmt.Errorf("AMQP_MAX_RETRIES must be >= 1")
	}
	if cfg.DBMaxRetries < 1 {
		return Config{}, fmt.Errorf("DB_MAX_RETRIES must be >= 1")
	}

	if cfg.WorkerSleepMs < 0 {
		return Config{}, fmt.Errorf("WORKER_SLEEP_MS must be >= 0")
	}

	return cfg, nil
}

func (c Config) EnableAPI() bool {
	return c.Mode == "all" || c.Mode == "api"
}

func (c Config) EnableWorker() bool {
	return c.Mode == "all" || c.Mode == "worker"
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return parsed
}
