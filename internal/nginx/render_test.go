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
				Domain:     "example.com",
				TargetIP:   "2001:db8::10",
				TargetPort: 8443,
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
