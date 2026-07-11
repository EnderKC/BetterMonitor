//go:build !monitor_only

package nginx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNginxDomainAndProviderContract(t *testing.T) {
	for input, expected := range map[string]string{
		"Example.COM":   "example.com",
		"*.Example.COM": "*.example.com",
	} {
		actual, err := NormalizeDomain(input, true)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}
	for _, domain := range []string{
		"", "example.com/path", `example.com\\path`, "example..com", "exa\nmple.com",
		"www.*.example.com", strings.Repeat("a", 64) + ".com", strings.Repeat("a", 250) + ".com",
	} {
		_, err := NormalizeDomain(domain, true)
		assert.Error(t, err, domain)
	}

	for input, expected := range map[string]string{
		"": "http01", "http01": "http01", "aliyun": "alidns", "alidns": "alidns", "cf": "cloudflare", "cloudflare": "cloudflare",
	} {
		actual, err := NormalizeProvider(input)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}
	_, err := NormalizeProvider("route53")
	assert.Error(t, err)
}

func TestNginxCertificateRequestContract(t *testing.T) {
	request := CertificateRequest{
		Domains:   []string{"Example.COM", "*.example.com"},
		Email:     "admin@example.com",
		Provider:  "cf",
		DNSConfig: map[string]string{"api_token": "secret"},
	}
	require.NoError(t, NormalizeCertificateRequest(&request))
	assert.Equal(t, []string{"example.com", "*.example.com"}, request.Domains)
	assert.Equal(t, "cloudflare", request.Provider)

	tooMany := make(map[string]string, 33)
	for index := range 33 {
		tooMany[string(rune('A'+index%26))+strings.Repeat("K", index/26)] = "value"
	}
	request.DNSConfig = tooMany
	assert.Error(t, NormalizeCertificateRequest(&request))

	request = CertificateRequest{Domains: []string{"example.com"}, Provider: "http01", Webroot: "relative/path"}
	assert.Error(t, NormalizeCertificateRequest(&request))
}
