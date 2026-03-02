package store

import (
	"errors"
	"sync"
	"time"

	"rabbitamq-queuecraft/internal/domain"
)

var ErrNotFound = errors.New("ticket not found")

type TicketStore interface {
	Create(ticket domain.Ticket) error
	Get(id string) (domain.Ticket, error)
	Update(ticket domain.Ticket) error
}

type MemoryTicketStore struct {
	mu      sync.RWMutex
	tickets map[string]domain.Ticket
}

func NewMemoryTicketStore() *MemoryTicketStore {
	return &MemoryTicketStore{tickets: make(map[string]domain.Ticket)}
}

func (s *MemoryTicketStore) Create(ticket domain.Ticket) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	ticket.CreatedAt = now
	ticket.UpdatedAt = now
	s.tickets[ticket.ID] = ticket
	return nil
}

func (s *MemoryTicketStore) Get(id string) (domain.Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ticket, ok := s.tickets[id]
	if !ok {
		return domain.Ticket{}, ErrNotFound
	}
	return ticket, nil
}

func (s *MemoryTicketStore) Update(ticket domain.Ticket) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.tickets[ticket.ID]
	if !ok {
		return ErrNotFound
	}
	ticket.UpdatedAt = time.Now().UTC()
	s.tickets[ticket.ID] = ticket
	return nil
}
