package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rabbitamq-queuecraft/internal/domain"
	"rabbitamq-queuecraft/internal/service"
	"rabbitamq-queuecraft/internal/store"

	"log/slog"
)

type stubPublisher struct{}

func (s *stubPublisher) Publish(_ context.Context, _ string, _ []byte) error {
	return nil
}

func setupTestRouter() http.Handler {
	repo := store.NewMemoryTicketStore()
	svc := service.NewTicketService(repo, &stubPublisher{}, "test.queue", 0, slog.Default())
	h := NewHandler(svc)
	return h.Routes()
}

func TestRoutes_CreateAndGetTicket(t *testing.T) {
	router := setupTestRouter()

	createBody := []byte(`{"customer_id":"cust_1","subject":"billing issue","body":"payment failed"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/tickets", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, createRec.Code)
	}

	var created domain.Ticket
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected non-empty ticket id")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/tickets/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, getRec.Code)
	}

	var fetched domain.Ticket
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ticket id %s, got %s", created.ID, fetched.ID)
	}
}

func TestRoutes_NotFound_ReturnsJSON(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal not found response: %v", err)
	}
	if payload["error"] != "route not found" {
		t.Fatalf("expected error %q, got %q", "route not found", payload["error"])
	}
}

func TestRoutes_MethodNotAllowed_ReturnsJSON(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodPut, "/v1/tickets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal method-not-allowed response: %v", err)
	}
	if payload["error"] != "method not allowed" {
		t.Fatalf("expected error %q, got %q", "method not allowed", payload["error"])
	}
}

func TestRoutes_GetCollection_MethodNotAllowed(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/v1/tickets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}
