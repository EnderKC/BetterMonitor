package controllers

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerContractImageReferenceValidation(t *testing.T) {
	valid := []string{
		"alpine:3.20",
		"registry.example.test:5000/team/image@sha256:" + strings.Repeat("a", 64),
		strings.Repeat("a", 512),
	}
	for _, image := range valid {
		t.Run("valid_"+image[:min(len(image), 24)], func(t *testing.T) {
			assert.NoError(t, ValidateImageRef(image))
		})
	}

	invalid := []string{
		"",
		" ",
		" alpine:latest",
		"alpine latest",
		"alpine\tlatest",
		"alpine\nlatest",
		"-alpine:latest",
		strings.Repeat("a", 513),
	}
	for _, image := range invalid {
		t.Run("invalid_"+strings.ReplaceAll(image[:min(len(image), 24)], "\n", "newline"), func(t *testing.T) {
			assert.Error(t, ValidateImageRef(image))
		})
	}
}

func TestDockerContractContainerAndComposeReferences(t *testing.T) {
	for _, ref := range []string{"abc123", "container_name-1", strings.Repeat("a", 128)} {
		assert.NoError(t, ValidateContainerRef(ref), ref)
	}
	for _, ref := range []string{"", "-container", "container/name", "container name", "container\nname", strings.Repeat("a", 129)} {
		assert.Error(t, ValidateContainerRef(ref), ref)
	}

	for _, name := range []string{"project", "Project_1.test", strings.Repeat("a", 128)} {
		assert.NoError(t, ValidateComposeName(name), name)
	}
	for _, name := range []string{"", ".", "..", "-project", "project/name", "project\\name", "project..name", "project name", strings.Repeat("a", 129)} {
		assert.Error(t, ValidateComposeName(name), name)
	}
}

func TestDockerContractTailTimeoutAndComposeContentBoundaries(t *testing.T) {
	for _, tail := range []int{1, 100, 10_000} {
		assert.NoError(t, ValidateDockerTail(tail))
	}
	for _, tail := range []int{0, -1, 10_001} {
		assert.Error(t, ValidateDockerTail(tail))
	}

	for _, timeout := range []int{1, 10, 300} {
		assert.NoError(t, ValidateDockerTimeout(timeout))
	}
	for _, timeout := range []int{0, -1, 301} {
		assert.Error(t, ValidateDockerTimeout(timeout))
	}

	assert.NoError(t, ValidateComposeContent(strings.Repeat("a", 1<<20)))
	assert.Error(t, ValidateComposeContent(strings.Repeat("a", (1<<20)+1)))
}

func TestDockerContractCreateContainerBoundaries(t *testing.T) {
	valid := DockerCreateRequest{
		Name:    "web-1",
		Image:   "nginx:1.27",
		Ports:   []string{"127.0.0.1:8080:80/tcp"},
		Volumes: []string{"/srv/web:/usr/share/nginx/html:ro"},
		Env:     map[string]string{"APP_ENV": "production"},
		Command: "nginx -g daemon off;",
		Restart: "unless-stopped",
		Network: "frontend_net",
	}
	assert.NoError(t, ValidateDockerCreate(valid))

	tests := []struct {
		name   string
		mutate func(*DockerCreateRequest)
	}{
		{name: "invalid name", mutate: func(req *DockerCreateRequest) { req.Name = "-web" }},
		{name: "invalid image", mutate: func(req *DockerCreateRequest) { req.Image = "-nginx" }},
		{name: "too many ports", mutate: func(req *DockerCreateRequest) { req.Ports = make([]string, 129) }},
		{name: "too many volumes", mutate: func(req *DockerCreateRequest) { req.Volumes = make([]string, 129) }},
		{name: "too many env", mutate: func(req *DockerCreateRequest) {
			req.Env = make(map[string]string, 257)
			for i := range 257 {
				req.Env["KEY_"+strings.Repeat("A", i/26)+string(rune('A'+i%26))] = "value"
			}
		}},
		{name: "oversized port", mutate: func(req *DockerCreateRequest) { req.Ports = []string{strings.Repeat("a", 257)} }},
		{name: "oversized volume", mutate: func(req *DockerCreateRequest) { req.Volumes = []string{strings.Repeat("a", 1025)} }},
		{name: "invalid env key", mutate: func(req *DockerCreateRequest) { req.Env = map[string]string{"BAD-KEY": "value"} }},
		{name: "oversized env value", mutate: func(req *DockerCreateRequest) { req.Env = map[string]string{"KEY": strings.Repeat("a", 4097)} }},
		{name: "oversized command", mutate: func(req *DockerCreateRequest) { req.Command = strings.Repeat("a", 8193) }},
		{name: "invalid restart", mutate: func(req *DockerCreateRequest) { req.Restart = "sometimes" }},
		{name: "invalid network", mutate: func(req *DockerCreateRequest) { req.Network = "-network" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			assert.Error(t, ValidateDockerCreate(request))
		})
	}
}

func TestDockerContractActionResponseAndTimeoutTable(t *testing.T) {
	tests := []struct {
		command         string
		action          string
		allowedResponse string
		timeout         time.Duration
	}{
		{command: "containers", action: "list", allowedResponse: "docker_containers", timeout: 30 * time.Second},
		{command: "containers", action: "logs", allowedResponse: "docker_container_logs", timeout: 30 * time.Second},
		{command: "containers", action: "start", allowedResponse: "success", timeout: 2 * time.Minute},
		{command: "containers", action: "stop", allowedResponse: "success", timeout: 2 * time.Minute},
		{command: "containers", action: "restart", allowedResponse: "success", timeout: 2 * time.Minute},
		{command: "containers", action: "remove", allowedResponse: "success", timeout: 2 * time.Minute},
		{command: "containers", action: "create", allowedResponse: "success", timeout: 2 * time.Minute},
		{command: "images", action: "list", allowedResponse: "docker_images", timeout: 30 * time.Second},
		{command: "images", action: "pull", allowedResponse: "success", timeout: 10 * time.Minute},
		{command: "images", action: "remove", allowedResponse: "success", timeout: 2 * time.Minute},
		{command: "composes", action: "list", allowedResponse: "docker_composes", timeout: 30 * time.Second},
		{command: "composes", action: "config", allowedResponse: "docker_compose_config", timeout: 30 * time.Second},
		{command: "composes", action: "up", allowedResponse: "success", timeout: 10 * time.Minute},
		{command: "composes", action: "down", allowedResponse: "success", timeout: 10 * time.Minute},
		{command: "composes", action: "create", allowedResponse: "success", timeout: 2 * time.Minute},
		{command: "composes", action: "remove", allowedResponse: "success", timeout: 2 * time.Minute},
	}

	for _, test := range tests {
		t.Run(test.command+"_"+test.action, func(t *testing.T) {
			spec, err := LookupDockerActionSpec(test.command, test.action)
			require.NoError(t, err)
			assert.Equal(t, test.command, spec.Command)
			assert.Equal(t, test.action, spec.Action)
			assert.Equal(t, test.allowedResponse, spec.AllowedResponse)
			assert.Equal(t, test.timeout, spec.Timeout)
		})
	}

	_, err := LookupDockerActionSpec("containers", "unknown")
	assert.Error(t, err)
	_, err = LookupDockerActionSpec("unknown", "list")
	assert.Error(t, err)
}
