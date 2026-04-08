package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/utada1stlove/reproxy/internal/app"
)

type memoryStore struct {
	routes []app.Route
}

func (s *memoryStore) Load(context.Context) ([]app.Route, error) {
	cloned := make([]app.Route, len(s.routes))
	copy(cloned, s.routes)
	return cloned, nil
}

func (s *memoryStore) Save(_ context.Context, routes []app.Route) error {
	s.routes = make([]app.Route, len(routes))
	copy(s.routes, routes)
	return nil
}

type statusSyncer struct{}

func (s *statusSyncer) Sync(context.Context, []app.Route) error {
	return nil
}

func (s *statusSyncer) DescribeRoutes(_ context.Context, routes []app.Route) ([]app.RouteDetails, error) {
	return app.DetailsFromRoutes(routes), nil
}

func (s *statusSyncer) SyncStatus() app.SyncStatus {
	return app.SyncStatus{
		Provider:   "nginx",
		ConfigPath: "/etc/nginx/conf.d/reproxy.conf",
	}
}

func TestPanelServesEmbeddedIndex(t *testing.T) {
	server := newTestHTTPServer()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/panel/", nil)

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	body := recorder.Body.String()
	if !bytes.Contains([]byte(body), []byte("reproxy Panel")) {
		t.Fatalf("expected panel HTML, got %q", body)
	}
}

func TestUpdateRouteByName(t *testing.T) {
	server := newTestHTTPServer()

	createPayload := `{"name":"demo-route","frontend_mode":"domain","domain":"demo.example.com","upstream_mode":"ip_port","target_ip":"10.0.0.12","target_port":8080}`
	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/routes", bytes.NewBufferString(createPayload))
	server.Handler.ServeHTTP(createRecorder, createRequest)

	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d", createRecorder.Code)
	}

	updatePayload := `{"frontend_mode":"domain","domain":"demo.example.com","upstream_mode":"host","target_host":"hentaiverse.org","target_scheme":"https"}`
	updateRecorder := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/routes/demo-route", bytes.NewBufferString(updatePayload))
	server.Handler.ServeHTTP(updateRecorder, updateRequest)

	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("expected update status 200, got %d", updateRecorder.Code)
	}

	getRecorder := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/routes/demo-route", nil)
	server.Handler.ServeHTTP(getRecorder, getRequest)

	if getRecorder.Code != http.StatusOK {
		t.Fatalf("expected get status 200, got %d", getRecorder.Code)
	}

	var payload routeResponse
	if err := json.NewDecoder(bytes.NewReader(getRecorder.Body.Bytes())).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Route == nil {
		t.Fatalf("expected route payload")
	}

	if payload.Route.TargetHost != "hentaiverse.org" {
		t.Fatalf("expected updated target host, got %q", payload.Route.TargetHost)
	}
}

func TestCreatePortListenerRoute(t *testing.T) {
	server := newTestHTTPServer()

	createPayload := `{"name":"hv-port","frontend_mode":"port","listen_port":8080,"upstream_mode":"host","target_host":"hentaiverse.org","target_scheme":"https"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/routes", bytes.NewBufferString(createPayload))
	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d", recorder.Code)
	}
}

func newTestHTTPServer() *http.Server {
	manager := app.NewManager(&memoryStore{}, &statusSyncer{})
	logger := log.New(io.Discard, "", 0)
	return NewServer(":0", logger, manager)
}
