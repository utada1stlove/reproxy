package app

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Route struct {
	Domain     string    `json:"domain"`
	TargetIP   string    `json:"target_ip"`
	TargetPort int       `json:"target_port"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type UpsertRouteInput struct {
	Domain     string `json:"domain"`
	TargetIP   string `json:"target_ip"`
	TargetPort int    `json:"target_port"`
}

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

type Store interface {
	Load(ctx context.Context) ([]Route, error)
	Save(ctx context.Context, routes []Route) error
}

type Syncer interface {
	Sync(ctx context.Context, routes []Route) error
}

type Manager struct {
	store  Store
	syncer Syncer
	mu     sync.RWMutex
}

var domainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func NewManager(store Store, syncer Syncer) *Manager {
	return &Manager{
		store:  store,
		syncer: syncer,
	}
}

func (m *Manager) Sync(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	routes, err := m.store.Load(ctx)
	if err != nil {
		return err
	}

	sortRoutes(routes)
	return m.syncer.Sync(ctx, routes)
}

func (m *Manager) ListRoutes(ctx context.Context) ([]Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routes, err := m.store.Load(ctx)
	if err != nil {
		return nil, err
	}

	sortRoutes(routes)
	return routes, nil
}

func (m *Manager) UpsertRoute(ctx context.Context, input UpsertRouteInput) (Route, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleaned, err := normalizeAndValidate(input)
	if err != nil {
		return Route{}, false, err
	}

	routes, err := m.store.Load(ctx)
	if err != nil {
		return Route{}, false, err
	}

	now := time.Now().UTC()
	route := Route{
		Domain:     cleaned.Domain,
		TargetIP:   cleaned.TargetIP,
		TargetPort: cleaned.TargetPort,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	for index, existing := range routes {
		if existing.Domain != cleaned.Domain {
			continue
		}

		route.CreatedAt = existing.CreatedAt
		if route.CreatedAt.IsZero() {
			route.CreatedAt = now
		}

		routes[index] = route
		sortRoutes(routes)

		if err := m.store.Save(ctx, routes); err != nil {
			return Route{}, false, err
		}

		if err := m.syncer.Sync(ctx, routes); err != nil {
			return route, false, fmt.Errorf("route saved but proxy sync failed: %w", err)
		}

		return route, false, nil
	}

	routes = append(routes, route)
	sortRoutes(routes)

	if err := m.store.Save(ctx, routes); err != nil {
		return Route{}, true, err
	}

	if err := m.syncer.Sync(ctx, routes); err != nil {
		return route, true, fmt.Errorf("route saved but proxy sync failed: %w", err)
	}

	return route, true, nil
}

func (m *Manager) DeleteRoute(ctx context.Context, domain string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	domain = normalizeDomain(domain)
	if err := validateDomain(domain); err != nil {
		return false, ValidationError{
			Field:   "domain",
			Message: err.Error(),
		}
	}

	routes, err := m.store.Load(ctx)
	if err != nil {
		return false, err
	}

	filteredRoutes := make([]Route, 0, len(routes))
	deleted := false
	for _, route := range routes {
		if route.Domain == domain {
			deleted = true
			continue
		}

		filteredRoutes = append(filteredRoutes, route)
	}

	if !deleted {
		return false, nil
	}

	sortRoutes(filteredRoutes)

	if err := m.store.Save(ctx, filteredRoutes); err != nil {
		return false, err
	}

	if err := m.syncer.Sync(ctx, filteredRoutes); err != nil {
		return true, fmt.Errorf("route deleted but proxy sync failed: %w", err)
	}

	return true, nil
}

func (m *Manager) ListRouteDetails(ctx context.Context) ([]RouteDetails, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routes, err := m.store.Load(ctx)
	if err != nil {
		return nil, err
	}

	sortRoutes(routes)
	return m.describeRoutes(ctx, routes)
}

func (m *Manager) GetRouteDetail(ctx context.Context, domain string) (RouteDetails, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	domain = normalizeDomain(domain)
	if err := validateDomain(domain); err != nil {
		return RouteDetails{}, false, ValidationError{
			Field:   "domain",
			Message: err.Error(),
		}
	}

	routes, err := m.store.Load(ctx)
	if err != nil {
		return RouteDetails{}, false, err
	}

	for _, route := range routes {
		if route.Domain != domain {
			continue
		}

		details, err := m.describeRoutes(ctx, []Route{route})
		if err != nil {
			return RouteDetails{}, false, err
		}

		return details[0], true, nil
	}

	return RouteDetails{}, false, nil
}

func (m *Manager) DescribeRoute(ctx context.Context, route Route) (RouteDetails, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	details, err := m.describeRoutes(ctx, []Route{route})
	if err != nil {
		return RouteDetails{}, err
	}

	return details[0], nil
}

func (m *Manager) Status(ctx context.Context) (StatusSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routes, err := m.store.Load(ctx)
	if err != nil {
		return StatusSnapshot{}, err
	}

	sortRoutes(routes)
	details, err := m.describeRoutes(ctx, routes)
	if err != nil {
		return StatusSnapshot{}, err
	}

	tlsReadyCount := 0
	for _, detail := range details {
		if detail.TLSReady {
			tlsReadyCount++
		}
	}

	snapshot := StatusSnapshot{
		Status:        "ok",
		RouteCount:    len(details),
		TLSReadyCount: tlsReadyCount,
	}

	if provider, ok := m.syncer.(RuntimeStatusProvider); ok {
		snapshot.Sync = provider.SyncStatus()
		if snapshot.Sync.LastSyncError != "" || snapshot.Sync.LastValidationError != "" || snapshot.Sync.LastReloadError != "" || snapshot.Sync.LastCertificateError != "" {
			snapshot.Status = "degraded"
		}
	}

	return snapshot, nil
}

func normalizeAndValidate(input UpsertRouteInput) (UpsertRouteInput, error) {
	domain := normalizeDomain(input.Domain)
	if err := validateDomain(domain); err != nil {
		return UpsertRouteInput{}, ValidationError{
			Field:   "domain",
			Message: err.Error(),
		}
	}

	ip := strings.TrimSpace(input.TargetIP)
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return UpsertRouteInput{}, ValidationError{
			Field:   "target_ip",
			Message: "must be a valid IPv4 or IPv6 address",
		}
	}

	if input.TargetPort < 1 || input.TargetPort > 65535 {
		return UpsertRouteInput{}, ValidationError{
			Field:   "target_port",
			Message: "must be between 1 and 65535",
		}
	}

	return UpsertRouteInput{
		Domain:     domain,
		TargetIP:   parsedIP.String(),
		TargetPort: input.TargetPort,
	}, nil
}

func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	return domain
}

func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("must not be empty")
	}

	if len(domain) > 253 {
		return fmt.Errorf("must be 253 characters or fewer")
	}

	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return fmt.Errorf("must be a fully qualified domain name")
	}

	for _, part := range parts {
		if !domainLabelPattern.MatchString(part) {
			return fmt.Errorf("contains invalid label %q", part)
		}
	}

	return nil
}

func sortRoutes(routes []Route) {
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Domain < routes[j].Domain
	})
}

func (m *Manager) describeRoutes(ctx context.Context, routes []Route) ([]RouteDetails, error) {
	if provider, ok := m.syncer.(RuntimeStatusProvider); ok {
		return provider.DescribeRoutes(ctx, routes)
	}

	return DetailsFromRoutes(routes), nil
}
