package runtimecfg

import (
	"os"
	"strings"
)

type Config struct {
	ListenAddr          string
	StoragePath         string
	NginxConfigPath     string
	ACMEWebroot         string
	CertsDir            string
	AdminEmail          string
	ReloadCommand       string
	ValidateCommand     string
	CertCommandTemplate string
	CertFileTemplate    string
	CertKeyTemplate     string
}

func Load() Config {
	return Config{
		ListenAddr:          getenv("REPROXY_LISTEN_ADDR", ":8080"),
		StoragePath:         getenv("REPROXY_STORAGE_PATH", "data/routes.json"),
		NginxConfigPath:     getenv("REPROXY_NGINX_CONFIG_PATH", "deployments/nginx/reproxy.conf"),
		ACMEWebroot:         getenv("REPROXY_ACME_WEBROOT", "/var/www/reproxy-acme"),
		CertsDir:            getenv("REPROXY_CERTS_DIR", "/etc/letsencrypt/live"),
		AdminEmail:          strings.TrimSpace(os.Getenv("REPROXY_ADMIN_EMAIL")),
		ReloadCommand:       strings.TrimSpace(os.Getenv("REPROXY_RELOAD_COMMAND")),
		ValidateCommand:     strings.TrimSpace(os.Getenv("REPROXY_VALIDATE_COMMAND")),
		CertCommandTemplate: strings.TrimSpace(os.Getenv("REPROXY_CERT_COMMAND_TEMPLATE")),
		CertFileTemplate:    getenv("REPROXY_CERT_FILE_TEMPLATE", "{{.CertsDir}}/{{.Domain}}/fullchain.pem"),
		CertKeyTemplate:     getenv("REPROXY_CERT_KEY_TEMPLATE", "{{.CertsDir}}/{{.Domain}}/privkey.pem"),
	}
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}
