package nginx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/utada1stlove/reproxy/internal/app"
	runtimecfg "github.com/utada1stlove/reproxy/internal/runtime"
)

type Syncer struct {
	config              runtimecfg.Config
	certCommandTemplate *template.Template
	certFileTemplate    *template.Template
	certKeyTemplate     *template.Template
	mu                  sync.RWMutex
	status              app.SyncStatus
}

type Site struct {
	Route    app.Route
	TLSReady bool
	CertPath string
	KeyPath  string
}

type tlsMaterial struct {
	CertPath string
	KeyPath  string
	Ready    bool
}

type templateData struct {
	Domain                    string
	Email                     string
	Webroot                   string
	CertsDir                  string
	CloudflareCredentialsPath string
}

func NewSyncer(config runtimecfg.Config) (*Syncer, error) {
	certFileTemplate, err := parseTemplate("cert-file", config.CertFileTemplate)
	if err != nil {
		return nil, err
	}

	certKeyTemplate, err := parseTemplate("cert-key", config.CertKeyTemplate)
	if err != nil {
		return nil, err
	}

	commandTemplate := strings.TrimSpace(config.CertCommandTemplate)
	if commandTemplate == "" && config.CertProvider == "cloudflare" {
		commandTemplate = "certbot certonly --dns-cloudflare --dns-cloudflare-credentials {{.CloudflareCredentialsPath}} -d {{.Domain}} --email {{.Email}} --agree-tos --non-interactive --keep-until-expiring"
	}

	var certCommandTemplate *template.Template
	if commandTemplate != "" {
		certCommandTemplate, err = parseTemplate("cert-command", commandTemplate)
		if err != nil {
			return nil, err
		}
	}

	return &Syncer{
		config:              config,
		certCommandTemplate: certCommandTemplate,
		certFileTemplate:    certFileTemplate,
		certKeyTemplate:     certKeyTemplate,
		status: app.SyncStatus{
			Provider:   "nginx",
			ConfigPath: config.NginxConfigPath,
		},
	}, nil
}

func (s *Syncer) Sync(ctx context.Context, routes []app.Route) error {
	s.recordSyncAttempt()

	sites, err := s.buildSites(ctx, routes)
	if err != nil {
		s.recordSyncError(err)
		return err
	}

	rendered := Render(sites, s.config.ACMEWebroot)
	previousContent, existed, err := readFileIfExists(s.config.NginxConfigPath)
	if err != nil {
		wrappedErr := fmt.Errorf("read current nginx config: %w", err)
		s.recordSyncError(wrappedErr)
		return wrappedErr
	}

	if err := writeFileAtomically(s.config.NginxConfigPath, []byte(rendered)); err != nil {
		wrappedErr := fmt.Errorf("write nginx config: %w", err)
		s.recordSyncError(wrappedErr)
		return wrappedErr
	}

	if err := s.runValidate(ctx); err != nil {
		if restoreErr := restoreFile(s.config.NginxConfigPath, previousContent, existed); restoreErr != nil {
			wrappedErr := fmt.Errorf("%v; restore previous config: %w", err, restoreErr)
			s.recordSyncError(wrappedErr)
			return wrappedErr
		}

		s.recordSyncError(err)
		return err
	}

	if err := s.runReload(ctx); err != nil {
		s.recordSyncError(err)
		return err
	}

	s.recordSyncSuccess()
	return nil
}

func (s *Syncer) DescribeRoutes(ctx context.Context, routes []app.Route) ([]app.RouteDetails, error) {
	_ = ctx

	details := make([]app.RouteDetails, 0, len(routes))
	for _, route := range routes {
		material, err := s.materialFor(route)
		if err != nil {
			return nil, err
		}

		details = append(details, app.RouteDetails{
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
			TLSReady:           material.Ready,
			CertPath:           material.CertPath,
			KeyPath:            material.KeyPath,
		})
	}

	return details, nil
}

func (s *Syncer) SyncStatus() app.SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return cloneSyncStatus(s.status)
}

func (s *Syncer) buildSites(ctx context.Context, routes []app.Route) ([]Site, error) {
	sites := make([]Site, 0, len(routes))
	for _, route := range routes {
		site, err := s.buildSite(ctx, route)
		if err != nil {
			return nil, err
		}

		sites = append(sites, site)
	}

	return sites, nil
}

func (s *Syncer) buildSite(ctx context.Context, route app.Route) (Site, error) {
	material, err := s.materialFor(route)
	if err != nil {
		return Site{}, err
	}

	if shouldEnsureTLS(route) && !material.Ready && s.certCommandTemplate != nil {
		if err := s.prepareCertificateDependencies(); err != nil {
			return Site{}, err
		}

		if err := s.ensureCertificate(ctx, route.Domain); err != nil {
			return Site{}, err
		}

		material, err = s.materialFor(route)
		if err != nil {
			return Site{}, err
		}
	}

	return Site{
		Route:    route,
		TLSReady: material.Ready,
		CertPath: material.CertPath,
		KeyPath:  material.KeyPath,
	}, nil
}

func (s *Syncer) materialFor(route app.Route) (tlsMaterial, error) {
	if !shouldEnsureTLS(route) {
		return tlsMaterial{}, nil
	}

	data := s.templateData(route.Domain)

	certPath, err := renderTemplate(s.certFileTemplate, data)
	if err != nil {
		return tlsMaterial{}, err
	}

	keyPath, err := renderTemplate(s.certKeyTemplate, data)
	if err != nil {
		return tlsMaterial{}, err
	}

	return tlsMaterial{
		CertPath: certPath,
		KeyPath:  keyPath,
		Ready:    fileExists(certPath) && fileExists(keyPath),
	}, nil
}

func shouldEnsureTLS(route app.Route) bool {
	return route.FrontendMode == app.FrontendModeDomain && route.EnableTLS && strings.TrimSpace(route.Domain) != ""
}

func (s *Syncer) prepareCertificateDependencies() error {
	if strings.TrimSpace(s.config.ACMEWebroot) != "" {
		if err := os.MkdirAll(s.config.ACMEWebroot, 0o755); err != nil {
			return fmt.Errorf("prepare ACME webroot: %w", err)
		}
	}

	if s.config.CertProvider == "cloudflare" {
		if strings.TrimSpace(s.config.CloudflareAPIToken) == "" {
			return fmt.Errorf("cloudflare cert provider requires REPROXY_CLOUDFLARE_API_TOKEN")
		}

		content := fmt.Sprintf("dns_cloudflare_api_token = %s\n", s.config.CloudflareAPIToken)
		if err := writeSensitiveFile(s.config.CloudflareCredentialsPath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write cloudflare credentials: %w", err)
		}
	}

	return nil
}

func (s *Syncer) ensureCertificate(ctx context.Context, domain string) error {
	s.recordCertificateAttempt(domain)

	command, err := renderTemplate(s.certCommandTemplate, s.templateData(domain))
	if err != nil {
		s.recordCertificateError(err)
		return err
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			wrappedErr := fmt.Errorf("ensure certificate for %s: %w: %s", domain, err, message)
			s.recordCertificateError(wrappedErr)
			return wrappedErr
		}

		wrappedErr := fmt.Errorf("ensure certificate for %s: %w", domain, err)
		s.recordCertificateError(wrappedErr)
		return wrappedErr
	}

	s.recordCertificateSuccess()
	return nil
}

func (s *Syncer) runReload(ctx context.Context) error {
	if strings.TrimSpace(s.config.ReloadCommand) == "" {
		s.clearReloadError()
		return nil
	}

	now := time.Now().UTC()
	s.mu.Lock()
	s.status.LastReloadAt = &now
	s.status.LastReloadError = ""
	s.mu.Unlock()

	cmd := exec.CommandContext(ctx, "sh", "-c", s.config.ReloadCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			wrappedErr := fmt.Errorf("reload nginx: %w: %s", err, message)
			s.recordReloadError(wrappedErr)
			return wrappedErr
		}

		wrappedErr := fmt.Errorf("reload nginx: %w", err)
		s.recordReloadError(wrappedErr)
		return wrappedErr
	}

	s.clearReloadError()
	return nil
}

func (s *Syncer) runValidate(ctx context.Context) error {
	if strings.TrimSpace(s.config.ValidateCommand) == "" {
		s.clearValidationError()
		return nil
	}

	now := time.Now().UTC()
	s.mu.Lock()
	s.status.LastValidationAt = &now
	s.status.LastValidationError = ""
	s.mu.Unlock()

	cmd := exec.CommandContext(ctx, "sh", "-c", s.config.ValidateCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			wrappedErr := fmt.Errorf("validate nginx config: %w: %s", err, message)
			s.recordValidationError(wrappedErr)
			return wrappedErr
		}

		wrappedErr := fmt.Errorf("validate nginx config: %w", err)
		s.recordValidationError(wrappedErr)
		return wrappedErr
	}

	s.clearValidationError()
	return nil
}

func (s *Syncer) templateData(domain string) templateData {
	return templateData{
		Domain:                    domain,
		Email:                     s.config.AdminEmail,
		Webroot:                   s.config.ACMEWebroot,
		CertsDir:                  s.config.CertsDir,
		CloudflareCredentialsPath: s.config.CloudflareCredentialsPath,
	}
}

func parseTemplate(name, content string) (*template.Template, error) {
	return template.New(name).Option("missingkey=error").Parse(content)
}

func renderTemplate(tmpl *template.Template, data templateData) (string, error) {
	var builder bytes.Buffer
	if err := tmpl.Execute(&builder, data); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func writeFileAtomically(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(dir, "nginx-*.conf")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tempPath, 0o644); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}

func writeSensitiveFile(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(dir, "secret-*")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tempPath, mode); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func readFileIfExists(path string) ([]byte, bool, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	return content, true, nil
}

func restoreFile(path string, content []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}

		return nil
	}

	return writeFileAtomically(path, content)
}

func (s *Syncer) recordSyncAttempt() {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastSyncAttemptAt = &now
	s.status.LastSyncError = ""
}

func (s *Syncer) recordSyncSuccess() {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastSyncSuccessAt = &now
	s.status.LastSyncError = ""
}

func (s *Syncer) recordSyncError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastSyncError = err.Error()
}

func (s *Syncer) recordValidationError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastValidationError = err.Error()
}

func (s *Syncer) clearValidationError() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastValidationError = ""
}

func (s *Syncer) recordReloadError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastReloadError = err.Error()
}

func (s *Syncer) clearReloadError() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastReloadError = ""
}

func (s *Syncer) recordCertificateAttempt(domain string) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastCertificateDomain = domain
	s.status.LastCertificateAt = &now
	s.status.LastCertificateError = ""
}

func (s *Syncer) recordCertificateError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastCertificateError = err.Error()
}

func (s *Syncer) recordCertificateSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.LastCertificateError = ""
}

func cloneSyncStatus(status app.SyncStatus) app.SyncStatus {
	cloned := status
	cloned.LastSyncAttemptAt = cloneTimePointer(status.LastSyncAttemptAt)
	cloned.LastSyncSuccessAt = cloneTimePointer(status.LastSyncSuccessAt)
	cloned.LastValidationAt = cloneTimePointer(status.LastValidationAt)
	cloned.LastReloadAt = cloneTimePointer(status.LastReloadAt)
	cloned.LastCertificateAt = cloneTimePointer(status.LastCertificateAt)
	return cloned
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}
