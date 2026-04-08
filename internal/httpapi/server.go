package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/utada1stlove/reproxy/internal/app"
)

type handler struct {
	logger  *log.Logger
	manager *app.Manager
}

type routeResponse struct {
	Result string            `json:"result,omitempty"`
	Route  *app.RouteDetails `json:"route,omitempty"`
	Error  string            `json:"error,omitempty"`
}

type routesResponse struct {
	Routes []app.RouteDetails `json:"routes"`
}

func NewServer(addr string, logger *log.Logger, manager *app.Manager) *http.Server {
	h := &handler{
		logger:  logger,
		manager: manager,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/status", h.handleStatus)
	mux.HandleFunc("/routes", h.handleRoutes)
	mux.HandleFunc("/routes/", h.handleRouteByDomain)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func (h *handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, routeResponse{Error: "method not allowed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, routeResponse{Error: "method not allowed"})
		return
	}

	status, err := h.manager.Status(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, routeResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *handler) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleListRoutes(w, r)
	case http.MethodPost:
		h.handleUpsertRoute(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, routeResponse{Error: "method not allowed"})
	}
}

func (h *handler) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := h.manager.ListRouteDetails(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, routeResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, routesResponse{Routes: routes})
}

func (h *handler) handleUpsertRoute(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var input app.UpsertRouteInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, routeResponse{Error: "invalid JSON body"})
		return
	}

	route, created, err := h.manager.UpsertRoute(r.Context(), input)
	if err != nil {
		status := http.StatusInternalServerError

		var validationErr app.ValidationError
		if errors.As(err, &validationErr) {
			status = http.StatusBadRequest
		}

		writeJSON(w, status, routeResponse{Error: err.Error()})
		return
	}

	result := "updated"
	status := http.StatusOK
	if created {
		result = "created"
		status = http.StatusCreated
	}

	details, err := h.manager.DescribeRoute(r.Context(), route)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, routeResponse{Error: err.Error()})
		return
	}

	h.logger.Printf("route %s domain=%s target=%s:%d", result, route.Domain, route.TargetIP, route.TargetPort)
	writeJSON(w, status, routeResponse{
		Result: result,
		Route:  &details,
	})
}

func (h *handler) handleRouteByDomain(w http.ResponseWriter, r *http.Request) {
	domain, ok := extractDomainFromPath(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "route not found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGetRouteByDomain(w, r, domain)
	case http.MethodDelete:
		h.handleDeleteRouteByDomain(w, r, domain)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, routeResponse{Error: "method not allowed"})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *handler) handleGetRouteByDomain(w http.ResponseWriter, r *http.Request, domain string) {
	route, found, err := h.manager.GetRouteDetail(r.Context(), domain)
	if err != nil {
		status := http.StatusInternalServerError

		var validationErr app.ValidationError
		if errors.As(err, &validationErr) {
			status = http.StatusBadRequest
		}

		writeJSON(w, status, routeResponse{Error: err.Error()})
		return
	}

	if !found {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "route not found"})
		return
	}

	writeJSON(w, http.StatusOK, routeResponse{Route: &route})
}

func (h *handler) handleDeleteRouteByDomain(w http.ResponseWriter, r *http.Request, domain string) {
	deleted, err := h.manager.DeleteRoute(r.Context(), domain)
	if err != nil {
		status := http.StatusInternalServerError

		var validationErr app.ValidationError
		if errors.As(err, &validationErr) {
			status = http.StatusBadRequest
		}

		writeJSON(w, status, routeResponse{Error: err.Error()})
		return
	}

	if !deleted {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "route not found"})
		return
	}

	h.logger.Printf("route deleted domain=%s", domain)
	writeJSON(w, http.StatusOK, routeResponse{Result: "deleted"})
}

func extractDomainFromPath(path string) (string, bool) {
	domain := strings.TrimPrefix(path, "/routes/")
	if domain == "" || strings.Contains(domain, "/") {
		return "", false
	}

	unescaped, err := url.PathUnescape(domain)
	if err != nil {
		return "", false
	}

	return unescaped, true
}
