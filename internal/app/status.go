package app

import (
	"context"
	"time"
)

type RouteDetails struct {
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
	TLSReady           bool      `json:"tls_ready"`
	CertPath           string    `json:"cert_path,omitempty"`
	KeyPath            string    `json:"key_path,omitempty"`
}

type SyncStatus struct {
	Provider              string     `json:"provider"`
	ConfigPath            string     `json:"config_path"`
	LastSyncAttemptAt     *time.Time `json:"last_sync_attempt_at,omitempty"`
	LastSyncSuccessAt     *time.Time `json:"last_sync_success_at,omitempty"`
	LastSyncError         string     `json:"last_sync_error,omitempty"`
	LastValidationAt      *time.Time `json:"last_validation_at,omitempty"`
	LastValidationError   string     `json:"last_validation_error,omitempty"`
	LastReloadAt          *time.Time `json:"last_reload_at,omitempty"`
	LastReloadError       string     `json:"last_reload_error,omitempty"`
	LastCertificateDomain string     `json:"last_certificate_domain,omitempty"`
	LastCertificateAt     *time.Time `json:"last_certificate_at,omitempty"`
	LastCertificateError  string     `json:"last_certificate_error,omitempty"`
}

type StatusSnapshot struct {
	Status        string     `json:"status"`
	RouteCount    int        `json:"route_count"`
	TLSReadyCount int        `json:"tls_ready_count"`
	Sync          SyncStatus `json:"sync"`
}

type RuntimeStatusProvider interface {
	DescribeRoutes(ctx context.Context, routes []Route) ([]RouteDetails, error)
	SyncStatus() SyncStatus
}

func DetailsFromRoutes(routes []Route) []RouteDetails {
	details := make([]RouteDetails, 0, len(routes))
	for _, route := range routes {
		details = append(details, RouteDetails{
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
			CreatedAt:          route.CreatedAt,
			UpdatedAt:          route.UpdatedAt,
		})
	}

	return details
}
