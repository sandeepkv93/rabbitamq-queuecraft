package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"rabbitamq-queuecraft/internal/config"
	"rabbitamq-queuecraft/internal/worker"

	amqp "github.com/rabbitmq/amqp091-go"
)

type testConsumer struct {
	onConsume func(ctx context.Context, queue string) error
}

func (c *testConsumer) Publish(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (c *testConsumer) Consume(ctx context.Context, queue string, _ func(context.Context, []byte) error) error {
	if c.onConsume != nil {
		return c.onConsume(ctx, queue)
	}
	<-ctx.Done()
	return nil
}

func (c *testConsumer) Close() error {
	return nil
}

func (c *testConsumer) Retry(_ context.Context, _ amqp.Delivery, _ string, _ error) error {
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRun_APIMode_ShutsDownGracefully(t *testing.T) {
	cfg := configForTests("api")
	cfg.HTTPAddr = "127.0.0.1:0"
	cfg.ShutdownTimeout = 2 * time.Second

	a := &App{
		cfg:    cfg,
		logger: testLogger(),
		server: &http.Server{Addr: cfg.HTTPAddr, Handler: http.NewServeMux()},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected graceful shutdown, got error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for api mode shutdown")
	}
}

func TestRun_WorkerMode_DoesNotStartAPI(t *testing.T) {
	cfg := configForTests("worker")
	cfg.ShutdownTimeout = 300 * time.Millisecond

	var consumeCalls atomic.Int32
	consumer := &testConsumer{
		onConsume: func(ctx context.Context, _ string) error {
			consumeCalls.Add(1)
			<-ctx.Done()
			return nil
		},
	}

	a := &App{
		cfg:    cfg,
		logger: testLogger(),
		server: nil,
		worker: &worker.Runner{
			Consumer: consumer,
			Queue:    "test.queue",
			Logger:   testLogger(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected graceful shutdown, got error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for worker mode shutdown")
	}

	if consumeCalls.Load() != 1 {
		t.Fatalf("expected worker consume to start once, got %d", consumeCalls.Load())
	}
}

func TestRun_WorkerMode_ShutdownTimeoutExceeded(t *testing.T) {
	cfg := configForTests("worker")
	cfg.ShutdownTimeout = 50 * time.Millisecond

	consumer := &testConsumer{
		onConsume: func(_ context.Context, _ string) error {
			select {}
		},
	}

	a := &App{
		cfg:    cfg,
		logger: testLogger(),
		worker: &worker.Runner{
			Consumer: consumer,
			Queue:    "test.queue",
			Logger:   testLogger(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Run(ctx)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected shutdown timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "shutdown timeout exceeded") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for shutdown timeout error")
	}
}

func configForTests(mode string) config.Config {
	return config.Config{
		Mode:            mode,
		ShutdownTimeout: time.Second,
	}
}
