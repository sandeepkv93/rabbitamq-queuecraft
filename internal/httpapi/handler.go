package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"rabbitamq-queuecraft/internal/service"
	"rabbitamq-queuecraft/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	svc *service.TicketService
}

func NewHandler(svc *service.TicketService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	r.Get("/healthz", h.handleHealth)
	r.Post("/v1/tickets", h.handleCreateTicket)
	r.Get("/v1/tickets/{ticketID}", h.handleGetTicket)
	return r
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
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
	id := chi.URLParam(r, "ticketID")
	if id == "" {
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
