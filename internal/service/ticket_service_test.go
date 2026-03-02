package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"rabbitamq-queuecraft/internal/domain"
	"rabbitamq-queuecraft/internal/store"
)

type mockPublisher struct {
	body []byte
}

func (m *mockPublisher) Publish(_ context.Context, _ string, body []byte) error {
	m.body = body
	return nil
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		priority string
	}{
		{name: "high", input: "urgent security issue", priority: "high"},
		{name: "medium", input: "request times out", priority: "medium"},
		{name: "low", input: "need invoice copy", priority: "low"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			priority, _ := classify(tc.input)
			if priority != tc.priority {
				t.Fatalf("expected %s, got %s", tc.priority, priority)
			}
		})
	}
}

func TestProcessTicketMessage(t *testing.T) {
	repo := store.NewMemoryTicketStore()
	pub := &mockPublisher{}
	svc := NewTicketService(repo, pub, "test.queue", 0, slog.Default())

	ticket := domain.Ticket{
		ID:         "tkt_123",
		CustomerID: "c_1",
		Subject:    "API timeout",
		Body:       "our requests timeout",
		Status:     domain.TicketQueued,
	}

	if err := repo.Create(ticket); err != nil {
		t.Fatalf("create seed ticket: %v", err)
	}

	payload := []byte(`{"id":"tkt_123","customer_id":"c_1","subject":"API timeout","body":"our requests timeout"}`)
	if err := svc.ProcessTicketMessage(context.Background(), payload); err != nil {
		t.Fatalf("process ticket message: %v", err)
	}

	updated, err := repo.Get("tkt_123")
	if err != nil {
		t.Fatalf("get updated ticket: %v", err)
	}

	if updated.Status != domain.TicketCompleted {
		t.Fatalf("expected status %s, got %s", domain.TicketCompleted, updated.Status)
	}
	if updated.Priority != "medium" {
		t.Fatalf("expected medium priority, got %s", updated.Priority)
	}
	if updated.UpdatedAt.Before(updated.CreatedAt.Add(-1 * time.Second)) {
		t.Fatalf("unexpected updated_at timestamp")
	}
}
