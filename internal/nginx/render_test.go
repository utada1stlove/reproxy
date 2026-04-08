package nginx

import (
	"strings"
	"testing"

	"github.com/utada1stlove/reproxy/internal/app"
)

func TestRenderIncludesTLSBlocksAndRedirect(t *testing.T) {
	output := Render([]Site{
		{
			Route: app.Route{
				Name:         "example.com",
				FrontendMode: app.FrontendModeDomain,
				Domain:       "example.com",
				EnableTLS:    true,
				UpstreamMode: app.UpstreamModeIPPort,
				TargetIP:     "2001:db8::10",
				TargetPort:   8443,
				TargetScheme: "http",
			},
			TLSReady: true,
			CertPath: "/etc/letsencrypt/live/example.com/fullchain.pem",
			KeyPath:  "/etc/letsencrypt/live/example.com/privkey.pem",
		},
	}, "/var/www/reproxy-acme")

	checks := []string{
		"listen 443 ssl http2;",
		"return 308 https://$host$request_uri;",
		"proxy_pass http://[2001:db8::10]:8443;",
		"ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem;",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected render output to contain %q", check)
		}
	}
}

func TestRenderSupportsPortFrontendAndHTTPSHostUpstream(t *testing.T) {
	output := Render([]Site{
		{
			Route: app.Route{
				Name:               "hv-port",
				FrontendMode:       app.FrontendModePort,
				ListenPort:         8080,
				UpstreamMode:       app.UpstreamModeHost,
				TargetHost:         "hentaiverse.org",
				TargetScheme:       "https",
				TargetPort:         443,
				UpstreamHostHeader: "hentaiverse.org",
				UpstreamSNI:        "hentaiverse.org",
			},
		},
	}, "/var/www/reproxy-acme")

	checks := []string{
		"listen 8080;",
		"proxy_pass https://hentaiverse.org;",
		"proxy_set_header Host hentaiverse.org;",
		"proxy_ssl_server_name on;",
		"proxy_ssl_name hentaiverse.org;",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected render output to contain %q", check)
		}
	}
}
