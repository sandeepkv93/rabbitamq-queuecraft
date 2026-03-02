package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"rabbitamq-queuecraft/internal/domain"
	"rabbitamq-queuecraft/internal/mq"
	"rabbitamq-queuecraft/internal/store"
)

type CreateTicketRequest struct {
	CustomerID string `json:"customer_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
}

type ticketMessage struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
}

type TicketService struct {
	store       store.TicketStore
	publisher   mq.Publisher
	queue       string
	workerDelay time.Duration
	logger      *slog.Logger
}

func NewTicketService(s store.TicketStore, p mq.Publisher, queue string, workerDelay time.Duration, logger *slog.Logger) *TicketService {
	return &TicketService{
		store:       s,
		publisher:   p,
		queue:       queue,
		workerDelay: workerDelay,
		logger:      logger,
	}
}

func (s *TicketService) CreateTicket(ctx context.Context, req CreateTicketRequest) (domain.Ticket, error) {
	if strings.TrimSpace(req.CustomerID) == "" {
		return domain.Ticket{}, errors.New("customer_id is required")
	}
	if strings.TrimSpace(req.Subject) == "" {
		return domain.Ticket{}, errors.New("subject is required")
	}
	if strings.TrimSpace(req.Body) == "" {
		return domain.Ticket{}, errors.New("body is required")
	}

	id := fmt.Sprintf("tkt_%d", time.Now().UTC().UnixNano())
	ticket := domain.Ticket{
		ID:         id,
		CustomerID: req.CustomerID,
		Subject:    req.Subject,
		Body:       req.Body,
		Status:     domain.TicketQueued,
	}

	if err := s.store.Create(ticket); err != nil {
		return domain.Ticket{}, fmt.Errorf("store create: %w", err)
	}

	msgBytes, err := json.Marshal(ticketMessage{
		ID:         id,
		CustomerID: req.CustomerID,
		Subject:    req.Subject,
		Body:       req.Body,
	})
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("marshal message: %w", err)
	}

	if err := s.publisher.Publish(ctx, s.queue, msgBytes); err != nil {
		return domain.Ticket{}, fmt.Errorf("publish ticket: %w", err)
	}

	s.logger.Info("ticket queued", "ticket_id", id, "queue", s.queue)
	return s.store.Get(id)
}

func (s *TicketService) GetTicket(id string) (domain.Ticket, error) {
	return s.store.Get(id)
}

func (s *TicketService) ProcessTicketMessage(ctx context.Context, payload []byte) error {
	var msg ticketMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return fmt.Errorf("unmarshal ticket message: %w", err)
	}

	ticket, err := s.store.Get(msg.ID)
	if err != nil {
		return fmt.Errorf("load ticket: %w", err)
	}

	ticket.Status = domain.TicketProcessing
	if err := s.store.Update(ticket); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.workerDelay):
	}

	ticket.Priority, ticket.Action = classify(msg.Subject + " " + msg.Body)
	ticket.Status = domain.TicketCompleted
	if err := s.store.Update(ticket); err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}

	s.logger.Info("ticket processed", "ticket_id", ticket.ID, "priority", ticket.Priority, "action", ticket.Action)
	return nil
}

func classify(raw string) (priority string, action string) {
	text := strings.ToLower(raw)
	highIndicators := []string{"payment failed", "outage", "security", "data loss", "urgent"}
	mediumIndicators := []string{"bug", "error", "slow", "timeout", "times out"}

	for _, key := range highIndicators {
		if strings.Contains(text, key) {
			return "high", "Escalate to on-call and create incident bridge"
		}
	}

	for _, key := range mediumIndicators {
		if strings.Contains(text, key) {
			return "medium", "Assign to support engineering queue"
		}
	}

	return "low", "Route to standard support workflow"
}
