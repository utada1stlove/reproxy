package app

import (
	"context"
	"time"
)

type RouteDetails struct {
	Domain     string    `json:"domain"`
	TargetIP   string    `json:"target_ip"`
	TargetPort int       `json:"target_port"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	TLSReady   bool      `json:"tls_ready"`
	CertPath   string    `json:"cert_path,omitempty"`
	KeyPath    string    `json:"key_path,omitempty"`
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
			Domain:     route.Domain,
			TargetIP:   route.TargetIP,
			TargetPort: route.TargetPort,
			CreatedAt:  route.CreatedAt,
			UpdatedAt:  route.UpdatedAt,
		})
	}

	return details
}
