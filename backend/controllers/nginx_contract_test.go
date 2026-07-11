package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utils "github.com/user/server-ops-backend/internal/agenttransport"
)

func TestNginxContractManagedFileValidation(t *testing.T) {
	assert.NoError(t, ValidateNginxManagedID(strings.Repeat("a", 32)))
	for _, id := range []string{"", "not-an-id", strings.Repeat("a", 31), strings.Repeat("A", 32), strings.Repeat("g", 32)} {
		assert.Error(t, ValidateNginxManagedID(id), id)
	}

	for _, name := range []string{"site", "site.conf", "site-name_1"} {
		assert.NoError(t, ValidateNginxConfigName(name), name)
	}
	for _, name := range []string{"", ".", "..", "../site", "nested/site", `nested\\site`, "-site", strings.Repeat("a", 129)} {
		assert.Error(t, ValidateNginxConfigName(name), name)
	}

	assert.NoError(t, ValidateNginxConfigContent(strings.Repeat("a", 1<<20)))
	assert.Error(t, ValidateNginxConfigContent(strings.Repeat("a", (1<<20)+1)))
}

func TestNginxContractDomainProviderAndDNSValidation(t *testing.T) {
	tests := map[string]string{
		"Example.COM":       "example.com",
		"www.example.com":   "www.example.com",
		"*.Example.COM":     "*.example.com",
		"a-b.example.co.uk": "a-b.example.co.uk",
	}
	for input, expected := range tests {
		actual, err := NormalizeNginxDomain(input, true)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}

	invalid := []string{
		"", "example.com/path", `example.com\\path`, "example..com", ".example.com", "example.com.",
		"exa\nmple.com", "*example.com", "www.*.example.com", "*.*.example.com",
		strings.Repeat("a", 64) + ".com", strings.Repeat("a", 250) + ".com",
	}
	for _, domain := range invalid {
		_, err := NormalizeNginxDomain(domain, true)
		assert.Error(t, err, domain)
	}

	providers := map[string]string{
		"": "http01", "http01": "http01", "alidns": "alidns", "aliyun": "alidns",
		"cloudflare": "cloudflare", "cf": "cloudflare",
	}
	for input, expected := range providers {
		actual, err := NormalizeCertificateProvider(input)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}
	_, err := NormalizeCertificateProvider("route53")
	assert.Error(t, err)

	validDNS := make(map[string]string, 32)
	for index := range 32 {
		validDNS["KEY_"+string(rune('A'+index%26))+string(rune('A'+index/26))] = strings.Repeat("v", 4096)
	}
	assert.NoError(t, ValidateCertificateDNSConfig(validDNS))
	validDNS["EXTRA"] = "value"
	assert.Error(t, ValidateCertificateDNSConfig(validDNS))
	assert.Error(t, ValidateCertificateDNSConfig(map[string]string{strings.Repeat("k", 65): "value"}))
	assert.Error(t, ValidateCertificateDNSConfig(map[string]string{"KEY": strings.Repeat("v", 4097)}))
}

func TestNginxContractAgentErrorsDoNotEchoSensitiveDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeNginxAgentError(c, &utils.AgentCommandError{
		Code:    "nginx_issue_ssl_failed",
		Message: "dns_token=super-secret package-manager raw output",
	})

	assert.Equal(t, 502, w.Code)
	assert.NotContains(t, w.Body.String(), "super-secret")
	assert.NotContains(t, w.Body.String(), "raw output")
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "nginx_issue_ssl_failed", body["code"])
}

func TestNginxContractWebsiteRequestBoundaries(t *testing.T) {
	domains, err := NormalizeNginxDomains([]string{"Example.COM", "*.Example.COM", "example.com"}, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com", "*.example.com"}, domains)
	_, err = NormalizeNginxDomains(make([]string, 101), true)
	assert.Error(t, err)

	assert.NoError(t, ValidateCertificateEmail("admin@example.com"))
	assert.NoError(t, ValidateCertificateEmail(""))
	assert.Error(t, ValidateCertificateEmail("admin example.com"))
	assert.Error(t, ValidateCertificateEmail(strings.Repeat("a", 255)))

	assert.NoError(t, ValidateCertificateWebroot("/var/www/acme"))
	assert.Error(t, ValidateCertificateWebroot("relative/path"))
	assert.Error(t, ValidateCertificateWebroot("/var/www/\nsecret"))

	assert.NoError(t, ValidateNginxJSONPayload(map[string]interface{}{"content": strings.Repeat("a", (1<<20)-32)}))
	assert.Error(t, ValidateNginxJSONPayload(map[string]interface{}{"content": strings.Repeat("a", (1<<20)+1)}))

	assert.NoError(t, ValidateNginxSessionID("install-session_1"))
	assert.Error(t, ValidateNginxSessionID("../session"))
	assert.Error(t, ValidateNginxSessionID(strings.Repeat("a", 129)))
}
