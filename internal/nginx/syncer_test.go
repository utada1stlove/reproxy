package nginx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utada1stlove/reproxy/internal/app"
	runtimecfg "github.com/utada1stlove/reproxy/internal/runtime"
)

func TestSyncWritesConfigWhenValidationPasses(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "reproxy.conf")

	syncer, err := NewSyncer(runtimecfg.Config{
		NginxConfigPath:  configPath,
		ValidateCommand:  "true",
		CertsDir:         tempDir,
		CertFileTemplate: "{{.CertsDir}}/{{.Domain}}/fullchain.pem",
		CertKeyTemplate:  "{{.CertsDir}}/{{.Domain}}/privkey.pem",
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	err = syncer.Sync(context.Background(), []app.Route{
		{Name: "demo.example.com", FrontendMode: app.FrontendModeDomain, Domain: "demo.example.com", EnableTLS: true, UpstreamMode: app.UpstreamModeIPPort, TargetIP: "10.0.0.1", TargetPort: 8080, TargetScheme: "http"},
	})
	if err != nil {
		t.Fatalf("sync routes: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read rendered config: %v", err)
	}

	if !strings.Contains(string(content), "demo.example.com") {
		t.Fatalf("expected rendered config to contain domain")
	}
}

func TestSyncRestoresPreviousStateWhenValidationFails(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "reproxy.conf")

	syncer, err := NewSyncer(runtimecfg.Config{
		NginxConfigPath:  configPath,
		ValidateCommand:  "false",
		CertsDir:         tempDir,
		CertFileTemplate: "{{.CertsDir}}/{{.Domain}}/fullchain.pem",
		CertKeyTemplate:  "{{.CertsDir}}/{{.Domain}}/privkey.pem",
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	err = syncer.Sync(context.Background(), []app.Route{
		{Name: "demo.example.com", FrontendMode: app.FrontendModeDomain, Domain: "demo.example.com", EnableTLS: true, UpstreamMode: app.UpstreamModeIPPort, TargetIP: "10.0.0.1", TargetPort: 8080, TargetScheme: "http"},
	})
	if err == nil {
		t.Fatalf("expected validation to fail")
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected invalid config to be rolled back")
	}
}

func TestDescribeRoutesIncludesTLSMaterialState(t *testing.T) {
	tempDir := t.TempDir()
	domainDir := filepath.Join(tempDir, "demo.example.com")
	if err := os.MkdirAll(domainDir, 0o755); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}

	for _, path := range []string{
		filepath.Join(domainDir, "fullchain.pem"),
		filepath.Join(domainDir, "privkey.pem"),
	} {
		if err := os.WriteFile(path, []byte("dummy"), 0o644); err != nil {
			t.Fatalf("write tls file: %v", err)
		}
	}

	syncer, err := NewSyncer(runtimecfg.Config{
		CertsDir:         tempDir,
		CertFileTemplate: "{{.CertsDir}}/{{.Domain}}/fullchain.pem",
		CertKeyTemplate:  "{{.CertsDir}}/{{.Domain}}/privkey.pem",
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	details, err := syncer.DescribeRoutes(context.Background(), []app.Route{
		{Name: "demo.example.com", FrontendMode: app.FrontendModeDomain, Domain: "demo.example.com", EnableTLS: true, UpstreamMode: app.UpstreamModeIPPort, TargetIP: "10.0.0.1", TargetPort: 8080, TargetScheme: "http"},
	})
	if err != nil {
		t.Fatalf("describe routes: %v", err)
	}

	if len(details) != 1 || !details[0].TLSReady {
		t.Fatalf("expected tls-ready route details")
	}
}

func TestCloudflareProviderWritesCredentialsFile(t *testing.T) {
	tempDir := t.TempDir()
	credentialsPath := filepath.Join(tempDir, "cloudflare.ini")
	configPath := filepath.Join(tempDir, "reproxy.conf")

	syncer, err := NewSyncer(runtimecfg.Config{
		NginxConfigPath:           configPath,
		ValidateCommand:           "true",
		CertProvider:              "cloudflare",
		CloudflareAPIToken:        "token-value",
		CloudflareCredentialsPath: credentialsPath,
		CertsDir:                  filepath.Join(tempDir, "certs"),
		CertFileTemplate:          "{{.CertsDir}}/{{.Domain}}/fullchain.pem",
		CertKeyTemplate:           "{{.CertsDir}}/{{.Domain}}/privkey.pem",
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	if err := syncer.prepareCertificateDependencies(); err != nil {
		t.Fatalf("prepare dependencies: %v", err)
	}

	content, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatalf("read credentials file: %v", err)
	}

	if !strings.Contains(string(content), "dns_cloudflare_api_token = token-value") {
		t.Fatalf("expected credentials file to include api token")
	}
}
