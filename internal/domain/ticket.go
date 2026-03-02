package domain

import "time"

type TicketStatus string

const (
	TicketQueued     TicketStatus = "queued"
	TicketProcessing TicketStatus = "processing"
	TicketCompleted  TicketStatus = "completed"
	TicketFailed     TicketStatus = "failed"
)

type Ticket struct {
	ID         string       `json:"id"`
	CustomerID string       `json:"customer_id"`
	Subject    string       `json:"subject"`
	Body       string       `json:"body"`
	Status     TicketStatus `json:"status"`
	Priority   string       `json:"priority,omitempty"`
	Action     string       `json:"action,omitempty"`
	Error      string       `json:"error,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}
