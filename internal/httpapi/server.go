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
	mux.HandleFunc("/", h.handleRoot)
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/status", h.handleStatus)
	mux.HandleFunc("/panel", h.handlePanelRoot)
	mux.Handle("/panel/", panelAssetHandler())
	mux.HandleFunc("/routes", h.handleRoutes)
	mux.HandleFunc("/routes/", h.handleRouteByName)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func (h *handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "not found"})
		return
	}

	http.Redirect(w, r, "/panel/", http.StatusTemporaryRedirect)
}

func (h *handler) handlePanelRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/panel" {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "not found"})
		return
	}

	http.Redirect(w, r, "/panel/", http.StatusTemporaryRedirect)
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
		writeJSON(w, errorStatus(err), routeResponse{Error: err.Error()})
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

	h.logger.Printf("route %s name=%s frontend=%s", result, route.Name, route.FrontendMode)
	writeJSON(w, status, routeResponse{
		Result: result,
		Route:  &details,
	})
}

func (h *handler) handleRouteByName(w http.ResponseWriter, r *http.Request) {
	name, ok := extractRouteNameFromPath(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "route not found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGetRouteByName(w, r, name)
	case http.MethodPut:
		h.handleUpdateRouteByName(w, r, name)
	case http.MethodDelete:
		h.handleDeleteRouteByName(w, r, name)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, routeResponse{Error: "method not allowed"})
	}
}

func (h *handler) handleGetRouteByName(w http.ResponseWriter, r *http.Request, name string) {
	route, found, err := h.manager.GetRouteDetail(r.Context(), name)
	if err != nil {
		writeJSON(w, errorStatus(err), routeResponse{Error: err.Error()})
		return
	}

	if !found {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "route not found"})
		return
	}

	writeJSON(w, http.StatusOK, routeResponse{Route: &route})
}

func (h *handler) handleUpdateRouteByName(w http.ResponseWriter, r *http.Request, name string) {
	defer r.Body.Close()

	current, found, err := h.manager.GetRouteDetail(r.Context(), name)
	if err != nil {
		writeJSON(w, errorStatus(err), routeResponse{Error: err.Error()})
		return
	}

	if !found {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "route not found"})
		return
	}

	input := app.UpsertRouteInput{
		Name:               current.Name,
		FrontendMode:       current.FrontendMode,
		Domain:             current.Domain,
		ListenIP:           current.ListenIP,
		ListenPort:         current.ListenPort,
		EnableTLS:          current.EnableTLS,
		UpstreamMode:       current.UpstreamMode,
		TargetIP:           current.TargetIP,
		TargetHost:         current.TargetHost,
		TargetPort:         current.TargetPort,
		TargetScheme:       current.TargetScheme,
		UpstreamHostHeader: current.UpstreamHostHeader,
		UpstreamSNI:        current.UpstreamSNI,
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, routeResponse{Error: "invalid JSON body"})
		return
	}

	input.Name = current.Name
	route, _, err := h.manager.UpsertRoute(r.Context(), input)
	if err != nil {
		writeJSON(w, errorStatus(err), routeResponse{Error: err.Error()})
		return
	}

	details, err := h.manager.DescribeRoute(r.Context(), route)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, routeResponse{Error: err.Error()})
		return
	}

	h.logger.Printf("route updated name=%s frontend=%s", route.Name, route.FrontendMode)
	writeJSON(w, http.StatusOK, routeResponse{
		Result: "updated",
		Route:  &details,
	})
}

func (h *handler) handleDeleteRouteByName(w http.ResponseWriter, r *http.Request, name string) {
	deleted, err := h.manager.DeleteRoute(r.Context(), name)
	if err != nil {
		writeJSON(w, errorStatus(err), routeResponse{Error: err.Error()})
		return
	}

	if !deleted {
		writeJSON(w, http.StatusNotFound, routeResponse{Error: "route not found"})
		return
	}

	h.logger.Printf("route deleted name=%s", name)
	writeJSON(w, http.StatusOK, routeResponse{Result: "deleted"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func extractRouteNameFromPath(path string) (string, bool) {
	name := strings.TrimPrefix(path, "/routes/")
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}

	unescaped, err := url.PathUnescape(name)
	if err != nil {
		return "", false
	}

	return unescaped, true
}

func errorStatus(err error) int {
	var validationErr app.ValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest
	}

	return http.StatusInternalServerError
}
