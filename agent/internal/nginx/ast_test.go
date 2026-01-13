package nginx

import (
	"strings"
	"testing"
)

func TestRender_StaticSiteUses404Fallback(t *testing.T) {
	cfg := &NginxConfig{
		Servers: []*ServerBlock{
			{
				Listen:        []string{"80"},
				ServerNames:   []string{"example.com"},
				Root:          "/www/sites/example.com",
				Index:         []string{"index.html"},
				ChallengeRoot: "/www/common",
			},
		},
	}

	out, err := cfg.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out, "index.php") {
		t.Fatalf("static site should not reference index.php, got:\n%s", out)
	}
	if !strings.Contains(out, "try_files $uri $uri/ =404;") {
		t.Fatalf("static site should use =404 fallback, got:\n%s", out)
	}
}

func TestRender_PHPUsesIndexPHPFallback(t *testing.T) {
	cfg := &NginxConfig{
		Servers: []*ServerBlock{
			{
				Listen:        []string{"80"},
				ServerNames:   []string{"example.com"},
				Root:          "/www/sites/example.com",
				Index:         []string{"index.php", "index.html"},
				ChallengeRoot: "/www/common",
				PHP: &PHPConfig{
					FastCGIPass: "unix:/run/php/php8.2-fpm.sock",
					Index:       []string{"index.php"},
				},
			},
		},
	}

	out, err := cfg.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "try_files $uri $uri/ /index.php$is_args$args;") {
		t.Fatalf("php site should use index.php fallback, got:\n%s", out)
	}
}

func TestDetermineSiteType(t *testing.T) {
	if got := determineSiteType(SiteConfig{}); got != "static" {
		t.Fatalf("expected static, got %q", got)
	}
	if got := determineSiteType(SiteConfig{PHPVersion: "8.2"}); got != "php" {
		t.Fatalf("expected php, got %q", got)
	}
	if got := determineSiteType(SiteConfig{Proxy: ProxyConfig{Enable: true}}); got != "proxy" {
		t.Fatalf("expected proxy, got %q", got)
	}
}

