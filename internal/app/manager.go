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

const (
	FrontendModeDomain = "domain"
	FrontendModePort   = "port"

	UpstreamModeIPPort = "ip_port"
	UpstreamModeHost   = "host"
)

type Route struct {
	Name               string    `json:"name"`
	FrontendMode       string    `json:"frontend_mode"`
	Domain             string    `json:"domain,omitempty"`
	ListenIP           string    `json:"listen_ip,omitempty"`
	ListenPort         int       `json:"listen_port,omitempty"`
	EnableTLS          bool      `json:"enable_tls,omitempty"`
	UpstreamMode       string    `json:"upstream_mode"`
	TargetIP           string    `json:"target_ip,omitempty"`
	TargetHost         string    `json:"target_host,omitempty"`
	TargetPort         int       `json:"target_port,omitempty"`
	TargetScheme       string    `json:"target_scheme,omitempty"`
	UpstreamHostHeader string    `json:"upstream_host_header,omitempty"`
	UpstreamSNI        string    `json:"upstream_sni,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type UpsertRouteInput struct {
	Name               string `json:"name"`
	FrontendMode       string `json:"frontend_mode"`
	Domain             string `json:"domain"`
	ListenIP           string `json:"listen_ip"`
	ListenPort         int    `json:"listen_port"`
	EnableTLS          bool   `json:"enable_tls"`
	UpstreamMode       string `json:"upstream_mode"`
	TargetIP           string `json:"target_ip"`
	TargetHost         string `json:"target_host"`
	TargetPort         int    `json:"target_port"`
	TargetScheme       string `json:"target_scheme"`
	UpstreamHostHeader string `json:"upstream_host_header"`
	UpstreamSNI        string `json:"upstream_sni"`
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

var (
	domainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	routeNamePattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,126})$`)
)

func NewManager(store Store, syncer Syncer) *Manager {
	return &Manager{
		store:  store,
		syncer: syncer,
	}
}

func (m *Manager) Sync(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	routes, err := m.loadRoutes(ctx)
	if err != nil {
		return err
	}

	return m.syncer.Sync(ctx, routes)
}

func (m *Manager) ListRoutes(ctx context.Context) ([]Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.loadRoutes(ctx)
}

func (m *Manager) UpsertRoute(ctx context.Context, input UpsertRouteInput) (Route, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleaned, err := normalizeAndValidate(input)
	if err != nil {
		return Route{}, false, err
	}

	routes, err := m.loadRoutes(ctx)
	if err != nil {
		return Route{}, false, err
	}

	now := time.Now().UTC()
	route := routeFromInput(cleaned, now, now)

	for index, existing := range routes {
		if existing.Name != cleaned.Name {
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

func (m *Manager) DeleteRoute(ctx context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = normalizeName(name)
	if err := validateRouteName(name); err != nil {
		return false, ValidationError{
			Field:   "name",
			Message: err.Error(),
		}
	}

	routes, err := m.loadRoutes(ctx)
	if err != nil {
		return false, err
	}

	filteredRoutes := make([]Route, 0, len(routes))
	deleted := false
	for _, route := range routes {
		if route.Name == name {
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

	routes, err := m.loadRoutes(ctx)
	if err != nil {
		return nil, err
	}

	return m.describeRoutes(ctx, routes)
}

func (m *Manager) GetRouteDetail(ctx context.Context, name string) (RouteDetails, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name = normalizeName(name)
	if err := validateRouteName(name); err != nil {
		return RouteDetails{}, false, ValidationError{
			Field:   "name",
			Message: err.Error(),
		}
	}

	routes, err := m.loadRoutes(ctx)
	if err != nil {
		return RouteDetails{}, false, err
	}

	for _, route := range routes {
		if route.Name != name {
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

	routes, err := m.loadRoutes(ctx)
	if err != nil {
		return StatusSnapshot{}, err
	}

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
	listenIP := strings.TrimSpace(input.ListenIP)
	name := normalizeName(input.Name)

	frontendMode := strings.TrimSpace(strings.ToLower(input.FrontendMode))
	if frontendMode == "" {
		if domain != "" {
			frontendMode = FrontendModeDomain
		} else {
			frontendMode = FrontendModePort
		}
	}

	if name == "" {
		switch frontendMode {
		case FrontendModeDomain:
			name = domain
		case FrontendModePort:
			if input.ListenPort > 0 {
				name = fmt.Sprintf("port-%d", input.ListenPort)
			}
		}
	}

	if err := validateRouteName(name); err != nil {
		return UpsertRouteInput{}, ValidationError{
			Field:   "name",
			Message: err.Error(),
		}
	}

	enableTLS := false
	switch frontendMode {
	case FrontendModeDomain:
		if err := validateDomain(domain); err != nil {
			return UpsertRouteInput{}, ValidationError{
				Field:   "domain",
				Message: err.Error(),
			}
		}
		enableTLS = true
	case FrontendModePort:
		if input.ListenPort < 1 || input.ListenPort > 65535 {
			return UpsertRouteInput{}, ValidationError{
				Field:   "listen_port",
				Message: "must be between 1 and 65535",
			}
		}

		if listenIP != "" {
			parsedIP := net.ParseIP(listenIP)
			if parsedIP == nil {
				return UpsertRouteInput{}, ValidationError{
					Field:   "listen_ip",
					Message: "must be a valid IPv4 or IPv6 address",
				}
			}

			listenIP = parsedIP.String()
		}
	default:
		return UpsertRouteInput{}, ValidationError{
			Field:   "frontend_mode",
			Message: "must be one of domain or port",
		}
	}

	upstreamMode := strings.TrimSpace(strings.ToLower(input.UpstreamMode))
	if upstreamMode == "" {
		if strings.TrimSpace(input.TargetHost) != "" {
			upstreamMode = UpstreamModeHost
		} else {
			upstreamMode = UpstreamModeIPPort
		}
	}

	targetScheme := strings.TrimSpace(strings.ToLower(input.TargetScheme))
	if targetScheme == "" {
		targetScheme = "http"
	}

	if targetScheme != "http" && targetScheme != "https" {
		return UpsertRouteInput{}, ValidationError{
			Field:   "target_scheme",
			Message: "must be http or https",
		}
	}

	targetIP := strings.TrimSpace(input.TargetIP)
	targetHost := normalizeDomain(input.TargetHost)
	targetPort := input.TargetPort
	upstreamHostHeader := strings.TrimSpace(input.UpstreamHostHeader)
	upstreamSNI := strings.TrimSpace(input.UpstreamSNI)

	switch upstreamMode {
	case UpstreamModeIPPort:
		parsedIP := net.ParseIP(targetIP)
		if parsedIP == nil {
			return UpsertRouteInput{}, ValidationError{
				Field:   "target_ip",
				Message: "must be a valid IPv4 or IPv6 address",
			}
		}

		if targetPort < 1 || targetPort > 65535 {
			return UpsertRouteInput{}, ValidationError{
				Field:   "target_port",
				Message: "must be between 1 and 65535",
			}
		}

		targetIP = parsedIP.String()
		targetHost = ""
	case UpstreamModeHost:
		if err := validateUpstreamHost(targetHost); err != nil {
			return UpsertRouteInput{}, ValidationError{
				Field:   "target_host",
				Message: err.Error(),
			}
		}

		if targetPort == 0 {
			targetPort = defaultPortForScheme(targetScheme)
		}

		if targetPort < 1 || targetPort > 65535 {
			return UpsertRouteInput{}, ValidationError{
				Field:   "target_port",
				Message: "must be between 1 and 65535",
			}
		}

		if upstreamHostHeader == "" {
			upstreamHostHeader = targetHost
		}

		if targetScheme == "https" && upstreamSNI == "" {
			upstreamSNI = targetHost
		}

		targetIP = ""
	default:
		return UpsertRouteInput{}, ValidationError{
			Field:   "upstream_mode",
			Message: "must be one of ip_port or host",
		}
	}

	if upstreamMode == UpstreamModeIPPort && upstreamHostHeader == "" && domain != "" {
		upstreamHostHeader = "$host"
	}

	return UpsertRouteInput{
		Name:               name,
		FrontendMode:       frontendMode,
		Domain:             domain,
		ListenIP:           listenIP,
		ListenPort:         input.ListenPort,
		EnableTLS:          enableTLS,
		UpstreamMode:       upstreamMode,
		TargetIP:           targetIP,
		TargetHost:         targetHost,
		TargetPort:         targetPort,
		TargetScheme:       targetScheme,
		UpstreamHostHeader: upstreamHostHeader,
		UpstreamSNI:        upstreamSNI,
	}, nil
}

func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	return domain
}

func normalizeName(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}

func validateRouteName(name string) error {
	if name == "" {
		return fmt.Errorf("must not be empty")
	}

	if len(name) > 127 {
		return fmt.Errorf("must be 127 characters or fewer")
	}

	if !routeNamePattern.MatchString(name) {
		return fmt.Errorf("may only contain lowercase letters, numbers, dots, underscores, and hyphens")
	}

	return nil
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

func validateUpstreamHost(host string) error {
	if host == "" {
		return fmt.Errorf("must not be empty")
	}

	if net.ParseIP(host) != nil {
		return nil
	}

	return validateDomain(host)
}

func defaultPortForScheme(scheme string) int {
	if scheme == "https" {
		return 443
	}

	return 80
}

func sortRoutes(routes []Route) {
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Name < routes[j].Name
	})
}

func (m *Manager) describeRoutes(ctx context.Context, routes []Route) ([]RouteDetails, error) {
	if provider, ok := m.syncer.(RuntimeStatusProvider); ok {
		return provider.DescribeRoutes(ctx, routes)
	}

	return DetailsFromRoutes(routes), nil
}

func (m *Manager) loadRoutes(ctx context.Context) ([]Route, error) {
	routes, err := m.store.Load(ctx)
	if err != nil {
		return nil, err
	}

	normalizedRoutes := make([]Route, 0, len(routes))
	for _, route := range routes {
		normalized, err := normalizeLoadedRoute(route)
		if err != nil {
			return nil, err
		}

		normalizedRoutes = append(normalizedRoutes, normalized)
	}

	sortRoutes(normalizedRoutes)
	return normalizedRoutes, nil
}

func normalizeLoadedRoute(route Route) (Route, error) {
	normalized, err := normalizeAndValidate(UpsertRouteInput{
		Name:               route.Name,
		FrontendMode:       route.FrontendMode,
		Domain:             route.Domain,
		ListenIP:           route.ListenIP,
		ListenPort:         route.ListenPort,
		EnableTLS:          route.EnableTLS,
		UpstreamMode:       route.UpstreamMode,
		TargetIP:           route.TargetIP,
		TargetHost:         route.TargetHost,
		TargetPort:         route.TargetPort,
		TargetScheme:       route.TargetScheme,
		UpstreamHostHeader: route.UpstreamHostHeader,
		UpstreamSNI:        route.UpstreamSNI,
	})
	if err != nil {
		return Route{}, err
	}

	return routeFromInput(normalized, route.CreatedAt, route.UpdatedAt), nil
}

func routeFromInput(input UpsertRouteInput, createdAt, updatedAt time.Time) Route {
	return Route{
		Name:               input.Name,
		FrontendMode:       input.FrontendMode,
		Domain:             input.Domain,
		ListenIP:           input.ListenIP,
		ListenPort:         input.ListenPort,
		EnableTLS:          input.EnableTLS,
		UpstreamMode:       input.UpstreamMode,
		TargetIP:           input.TargetIP,
		TargetHost:         input.TargetHost,
		TargetPort:         input.TargetPort,
		TargetScheme:       input.TargetScheme,
		UpstreamHostHeader: input.UpstreamHostHeader,
		UpstreamSNI:        input.UpstreamSNI,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}
}
