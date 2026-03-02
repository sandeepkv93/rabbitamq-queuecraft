package worker

import (
	"context"
	"fmt"
	"log/slog"

	"rabbitamq-queuecraft/internal/mq"
	"rabbitamq-queuecraft/internal/service"
)

type Runner struct {
	Consumer mq.Consumer
	Service  *service.TicketService
	Queue    string
	Logger   *slog.Logger
}

func (r *Runner) Run(ctx context.Context) error {
	r.Logger.Info("worker started", "queue", r.Queue)
	err := r.Consumer.Consume(ctx, r.Queue, func(ctx context.Context, payload []byte) error {
		if err := r.Service.ProcessTicketMessage(ctx, payload); err != nil {
			r.Logger.Error("ticket processing failed", "error", err)
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("worker consume: %w", err)
	}
	r.Logger.Info("worker stopped")
	return nil
}
