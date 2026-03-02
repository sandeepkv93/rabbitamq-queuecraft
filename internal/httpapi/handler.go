package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"rabbitamq-queuecraft/internal/service"
	"rabbitamq-queuecraft/internal/store"
)

type Handler struct {
	svc *service.TicketService
}

func NewHandler(svc *service.TicketService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/v1/tickets", h.handleCreateTicket)
	mux.HandleFunc("/v1/tickets/", h.handleGetTicket)
	return mux
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()
	var req service.CreateTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	ticket, err := h.svc.CreateTicket(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, ticket)
}

func (h *Handler) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, r.Method+" method not allowed")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v1/tickets/")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "ticket id is required")
		return
	}

	ticket, err := h.svc.GetTicket(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load ticket")
		return
	}

	writeJSON(w, http.StatusOK, ticket)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
