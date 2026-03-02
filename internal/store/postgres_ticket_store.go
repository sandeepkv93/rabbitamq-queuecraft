package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rabbitamq-queuecraft/internal/domain"
)

type PostgresTicketStore struct {
	db *sql.DB
}

func NewPostgresTicketStore(db *sql.DB) *PostgresTicketStore {
	return &PostgresTicketStore{db: db}
}

func (s *PostgresTicketStore) EnsureSchema(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS tickets (
  id TEXT PRIMARY KEY,
  customer_id TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL,
  priority TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

func (s *PostgresTicketStore) Create(ticket domain.Ticket) error {
	const q = `
INSERT INTO tickets (id, customer_id, subject, body, status, priority, action, error, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
RETURNING created_at, updated_at;`

	priority := ticket.Priority
	action := ticket.Action
	errorMsg := ticket.Error

	if err := s.db.QueryRowContext(context.Background(), q,
		ticket.ID,
		ticket.CustomerID,
		ticket.Subject,
		ticket.Body,
		ticket.Status,
		priority,
		action,
		errorMsg,
	).Scan(&ticket.CreatedAt, &ticket.UpdatedAt); err != nil {
		return fmt.Errorf("insert ticket: %w", err)
	}
	return nil
}

func (s *PostgresTicketStore) Get(id string) (domain.Ticket, error) {
	const q = `
SELECT id, customer_id, subject, body, status, priority, action, error, created_at, updated_at
FROM tickets
WHERE id = $1;`

	var ticket domain.Ticket
	err := s.db.QueryRowContext(context.Background(), q, id).Scan(
		&ticket.ID,
		&ticket.CustomerID,
		&ticket.Subject,
		&ticket.Body,
		&ticket.Status,
		&ticket.Priority,
		&ticket.Action,
		&ticket.Error,
		&ticket.CreatedAt,
		&ticket.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Ticket{}, ErrNotFound
	}
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("get ticket: %w", err)
	}
	return ticket, nil
}

func (s *PostgresTicketStore) Update(ticket domain.Ticket) error {
	const q = `
UPDATE tickets
SET customer_id = $2,
    subject = $3,
    body = $4,
    status = $5,
    priority = $6,
    action = $7,
    error = $8,
    updated_at = NOW()
WHERE id = $1;`

	res, err := s.db.ExecContext(context.Background(), q,
		ticket.ID,
		ticket.CustomerID,
		ticket.Subject,
		ticket.Body,
		ticket.Status,
		ticket.Priority,
		ticket.Action,
		ticket.Error,
	)
	if err != nil {
		return fmt.Errorf("update ticket: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}
