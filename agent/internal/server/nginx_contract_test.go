//go:build !monitor_only

package server

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/server-ops-agent/pkg/logger"
)

func TestNginxAgentErrorProjectionRedactsOperationDetails(t *testing.T) {
	client, responses := newDockerContractTestClient(t)
	original := handleNginxMonitorCommand
	handleNginxMonitorCommand = func(string, map[string]interface{}) (string, error) {
		return "", errors.New("dns_token=super-secret package-manager raw output")
	}
	t.Cleanup(func() { handleNginxMonitorCommand = original })

	message := []byte(`{
		"type":"nginx_command",
		"request_id":"request-nginx",
		"payload":{"action":"issue_ssl","domains":["example.com"]}
	}`)
	client.handleNginxCommand(message)
	response := <-responses
	assert.Equal(t, "nginx_error", response.responseType)
	assert.Equal(t, "nginx_issue_ssl_failed", response.data["code"])
	encoded, err := json.Marshal(response.data)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "super-secret")
	assert.NotContains(t, string(encoded), "raw output")
}

func TestNginxRawResponseUsesClientWriteBoundary(t *testing.T) {
	log, err := logger.New("", "error")
	require.NoError(t, err)
	client := &Client{log: log}
	written := make(chan interface{}, 1)
	client.jsonSink = func(value interface{}) error {
		written <- value
		return nil
	}

	client.sendRawResponse("request-nginx", "nginx_success", `{"success":true}`)
	value := <-written
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"request_id":"request-nginx"`)
	assert.Contains(t, string(encoded), `"type":"nginx_success"`)
	assert.False(t, strings.Contains(string(encoded), "secret_key"))
}
