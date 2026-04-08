package app

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	routes []Route
}

func (s *memoryStore) Load(context.Context) ([]Route, error) {
	return cloneRoutes(s.routes), nil
}

func (s *memoryStore) Save(_ context.Context, routes []Route) error {
	s.routes = cloneRoutes(routes)
	return nil
}

type captureSyncer struct {
	routes []Route
}

func (s *captureSyncer) Sync(_ context.Context, routes []Route) error {
	s.routes = cloneRoutes(routes)
	return nil
}

type statusSyncer struct{}

func (s *statusSyncer) Sync(_ context.Context, _ []Route) error {
	return nil
}

func (s *statusSyncer) DescribeRoutes(_ context.Context, routes []Route) ([]RouteDetails, error) {
	details := DetailsFromRoutes(routes)
	for index := range details {
		if details[index].Domain == "tls.example.com" {
			details[index].TLSReady = true
			details[index].CertPath = "/etc/letsencrypt/live/tls.example.com/fullchain.pem"
			details[index].KeyPath = "/etc/letsencrypt/live/tls.example.com/privkey.pem"
		}
	}

	return details, nil
}

func (s *statusSyncer) SyncStatus() SyncStatus {
	return SyncStatus{
		Provider:   "nginx",
		ConfigPath: "/etc/nginx/conf.d/reproxy.conf",
	}
}

func TestUpsertRouteCreatesAndUpdatesByDomain(t *testing.T) {
	store := &memoryStore{}
	syncer := &captureSyncer{}
	manager := NewManager(store, syncer)

	createdRoute, created, err := manager.UpsertRoute(context.Background(), UpsertRouteInput{
		Domain:     "Example.com.",
		TargetIP:   "10.0.0.10",
		TargetPort: 8080,
	})
	if err != nil {
		t.Fatalf("create route: %v", err)
	}

	if !created {
		t.Fatalf("expected first upsert to create route")
	}

	if createdRoute.Domain != "example.com" {
		t.Fatalf("expected normalized domain, got %q", createdRoute.Domain)
	}

	updatedRoute, created, err := manager.UpsertRoute(context.Background(), UpsertRouteInput{
		Domain:     "example.com",
		TargetIP:   "10.0.0.11",
		TargetPort: 9090,
	})
	if err != nil {
		t.Fatalf("update route: %v", err)
	}

	if created {
		t.Fatalf("expected second upsert to update existing route")
	}

	if len(store.routes) != 1 {
		t.Fatalf("expected exactly one route in store, got %d", len(store.routes))
	}

	if updatedRoute.TargetPort != 9090 {
		t.Fatalf("expected target port 9090, got %d", updatedRoute.TargetPort)
	}

	if syncer.routes[0].TargetIP != "10.0.0.11" {
		t.Fatalf("expected syncer to receive updated route, got %q", syncer.routes[0].TargetIP)
	}
}

func TestUpsertRouteRejectsInvalidInput(t *testing.T) {
	manager := NewManager(&memoryStore{}, &captureSyncer{})

	_, _, err := manager.UpsertRoute(context.Background(), UpsertRouteInput{
		Domain:     "bad_domain",
		TargetIP:   "not-an-ip",
		TargetPort: 70000,
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}

	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestDeleteRouteRemovesRouteAndSyncs(t *testing.T) {
	store := &memoryStore{
		routes: []Route{
			{Domain: "keep.example.com", TargetIP: "10.0.0.10", TargetPort: 8080},
			{Domain: "drop.example.com", TargetIP: "10.0.0.11", TargetPort: 8081},
		},
	}
	syncer := &captureSyncer{}
	manager := NewManager(store, syncer)

	deleted, err := manager.DeleteRoute(context.Background(), "drop.example.com")
	if err != nil {
		t.Fatalf("delete route: %v", err)
	}

	if !deleted {
		t.Fatalf("expected route to be deleted")
	}

	if len(store.routes) != 1 {
		t.Fatalf("expected 1 route after delete, got %d", len(store.routes))
	}

	if store.routes[0].Domain != "keep.example.com" {
		t.Fatalf("expected keep.example.com to remain, got %q", store.routes[0].Domain)
	}

	if len(syncer.routes) != 1 || syncer.routes[0].Domain != "keep.example.com" {
		t.Fatalf("expected syncer to receive remaining route set")
	}
}

func TestStatusIncludesTLSCounts(t *testing.T) {
	store := &memoryStore{
		routes: []Route{
			{Domain: "plain.example.com", TargetIP: "10.0.0.10", TargetPort: 8080},
			{Domain: "tls.example.com", TargetIP: "10.0.0.11", TargetPort: 8443},
		},
	}

	manager := NewManager(store, &statusSyncer{})
	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if status.RouteCount != 2 {
		t.Fatalf("expected 2 routes, got %d", status.RouteCount)
	}

	if status.TLSReadyCount != 1 {
		t.Fatalf("expected 1 tls-ready route, got %d", status.TLSReadyCount)
	}

	if status.Sync.Provider != "nginx" {
		t.Fatalf("expected nginx provider, got %q", status.Sync.Provider)
	}
}

func cloneRoutes(routes []Route) []Route {
	cloned := make([]Route, len(routes))
	copy(cloned, routes)
	return cloned
}
