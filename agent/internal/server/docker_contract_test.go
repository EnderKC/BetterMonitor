//go:build !monitor_only

package server

import (
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/server-ops-agent/pkg/logger"
)

type dockerTestResponse struct {
	requestID    string
	responseType string
	data         map[string]interface{}
}

type fakeDockerCommandManager struct {
	dockerCommandManager
	pullImage        func(string) error
	getContainerLogs func(string, int) (string, error)
	startContainer   func(string) error
	composeUp        func(string) error
	createContainer  func(string, string, []string, []string, map[string]string, string, string, string) (string, error)
	createCompose    func(string, string) error
	close            func() error
}

func (f *fakeDockerCommandManager) PullImage(image string) error {
	return f.pullImage(image)
}

func (f *fakeDockerCommandManager) Close() error {
	return f.close()
}

func (f *fakeDockerCommandManager) GetContainerLogs(containerID string, tail int) (string, error) {
	if f.getContainerLogs == nil {
		return "", nil
	}
	return f.getContainerLogs(containerID, tail)
}

func (f *fakeDockerCommandManager) StartContainer(containerID string) error {
	if f.startContainer == nil {
		return nil
	}
	return f.startContainer(containerID)
}

func (f *fakeDockerCommandManager) ComposeUp(name string) error {
	if f.composeUp == nil {
		return nil
	}
	return f.composeUp(name)
}

func (f *fakeDockerCommandManager) CreateContainer(
	name string,
	image string,
	ports []string,
	volumes []string,
	env map[string]string,
	command string,
	restart string,
	network string,
) (string, error) {
	if f.createContainer == nil {
		return "", nil
	}
	return f.createContainer(name, image, ports, volumes, env, command, restart, network)
}

func (f *fakeDockerCommandManager) CreateCompose(name, content string) error {
	if f.createCompose == nil {
		return nil
	}
	return f.createCompose(name, content)
}

func dockerPullMessage(t *testing.T, image string) []byte {
	t.Helper()
	message, err := json.Marshal(map[string]interface{}{
		"type":       "docker_command",
		"request_id": "request-pull",
		"payload": map[string]interface{}{
			"command": "images",
			"action":  "pull",
			"params":  map[string]interface{}{"image": image},
		},
	})
	require.NoError(t, err)
	return message
}

func dockerCommandMessage(t *testing.T, command, action string, params interface{}) []byte {
	t.Helper()
	message, err := json.Marshal(map[string]interface{}{
		"type":       "docker_command",
		"request_id": "request-contract",
		"payload": map[string]interface{}{
			"command": command,
			"action":  action,
			"params":  params,
		},
	})
	require.NoError(t, err)
	return message
}

func newDockerContractTestClient(t *testing.T) (*Client, <-chan dockerTestResponse) {
	t.Helper()
	log, err := logger.New("", "error")
	require.NoError(t, err)
	responses := make(chan dockerTestResponse, 2)
	client := &Client{log: log}
	client.responseSink = func(requestID, responseType string, data map[string]interface{}) {
		responses <- dockerTestResponse{requestID: requestID, responseType: responseType, data: data}
	}
	return client, responses
}

func TestDockerPullWaitsForCompletionBeforeSuccessAndManagerClose(t *testing.T) {
	client, responses := newDockerContractTestClient(t)
	started := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	manager := &fakeDockerCommandManager{
		pullImage: func(image string) error {
			assert.Equal(t, "alpine:3.20", image)
			close(started)
			<-release
			return nil
		},
		close: func() error {
			close(closed)
			return nil
		},
	}
	originalFactory := newDockerCommandManager
	newDockerCommandManager = func(*logger.Logger) (dockerCommandManager, error) {
		return manager, nil
	}
	t.Cleanup(func() { newDockerCommandManager = originalFactory })

	done := make(chan struct{})
	go func() {
		client.handleDockerCommand(dockerPullMessage(t, "alpine:3.20"))
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("pull runner did not start")
	}
	select {
	case response := <-responses:
		t.Fatalf("received response before pull completed: %#v", response)
	case <-closed:
		t.Fatal("Docker manager closed before pull completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	select {
	case response := <-responses:
		assert.Equal(t, "request-pull", response.requestID)
		assert.Equal(t, "success", response.responseType)
		assert.Equal(t, true, response.data["success"])
	case <-time.After(time.Second):
		t.Fatal("pull completion did not emit success")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Docker manager was not closed after pull completion")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Docker handler did not return")
	}
}

func TestDockerPullFailureReturnsStableRedactedError(t *testing.T) {
	client, responses := newDockerContractTestClient(t)
	manager := &fakeDockerCommandManager{
		pullImage: func(string) error {
			return errors.New("exit status 1: registry token and private output")
		},
		close: func() error { return nil },
	}
	originalFactory := newDockerCommandManager
	newDockerCommandManager = func(*logger.Logger) (dockerCommandManager, error) {
		return manager, nil
	}
	t.Cleanup(func() { newDockerCommandManager = originalFactory })

	client.handleDockerCommand(dockerPullMessage(t, "alpine:3.20"))
	response := <-responses
	assert.Equal(t, "docker_error", response.responseType)
	assert.Equal(t, "docker_image_pull_failed", response.data["code"])
	encoded, err := json.Marshal(response.data)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "registry token")
	assert.NotContains(t, string(encoded), "private output")
}

func TestDockerPullRejectsLeadingDashWithoutExecuting(t *testing.T) {
	client, responses := newDockerContractTestClient(t)
	var pulls atomic.Int32
	manager := &fakeDockerCommandManager{
		pullImage: func(string) error {
			pulls.Add(1)
			return nil
		},
		close: func() error { return nil },
	}
	originalFactory := newDockerCommandManager
	newDockerCommandManager = func(*logger.Logger) (dockerCommandManager, error) {
		return manager, nil
	}
	t.Cleanup(func() { newDockerCommandManager = originalFactory })

	client.handleDockerCommand(dockerPullMessage(t, "-alpine"))
	response := <-responses
	assert.Equal(t, "docker_error", response.responseType)
	assert.Equal(t, "invalid_docker_image", response.data["code"])
	assert.Zero(t, pulls.Load())
	assert.NotContains(t, strings.ToLower(response.data["error"].(string)), "-alpine")
}

func TestDockerAgentRejectsInvalidContainerAndComposeParameters(t *testing.T) {
	tests := []struct {
		name    string
		command string
		action  string
		params  interface{}
	}{
		{name: "tail zero", command: "containers", action: "logs", params: map[string]interface{}{"container_id": "web", "tail": 0}},
		{name: "container leading dash", command: "containers", action: "start", params: map[string]interface{}{"container_id": "-web"}},
		{name: "compose traversal", command: "composes", action: "up", params: map[string]interface{}{"name": "../project"}},
		{name: "create invalid image", command: "containers", action: "create", params: map[string]interface{}{"name": "web", "image": "-alpine"}},
		{name: "compose content too large", command: "composes", action: "create", params: map[string]interface{}{"name": "project", "content": strings.Repeat("a", (1<<20)+1)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, responses := newDockerContractTestClient(t)
			var calls atomic.Int32
			manager := &fakeDockerCommandManager{
				getContainerLogs: func(string, int) (string, error) { calls.Add(1); return "", nil },
				startContainer:   func(string) error { calls.Add(1); return nil },
				composeUp:        func(string) error { calls.Add(1); return nil },
				createContainer: func(string, string, []string, []string, map[string]string, string, string, string) (string, error) {
					calls.Add(1)
					return "", nil
				},
				createCompose: func(string, string) error { calls.Add(1); return nil },
				close:         func() error { return nil },
			}
			originalFactory := newDockerCommandManager
			newDockerCommandManager = func(*logger.Logger) (dockerCommandManager, error) { return manager, nil }
			t.Cleanup(func() { newDockerCommandManager = originalFactory })

			client.handleDockerCommand(dockerCommandMessage(t, test.command, test.action, test.params))
			response := <-responses
			assert.Equal(t, "docker_error", response.responseType)
			assert.Equal(t, "invalid_docker_request", response.data["code"])
			assert.Zero(t, calls.Load())
		})
	}
}

func TestDockerAgentRedactsContainerCommandFailure(t *testing.T) {
	client, responses := newDockerContractTestClient(t)
	manager := &fakeDockerCommandManager{
		startContainer: func(string) error {
			return errors.New("docker output contains private environment data")
		},
		close: func() error { return nil },
	}
	originalFactory := newDockerCommandManager
	newDockerCommandManager = func(*logger.Logger) (dockerCommandManager, error) { return manager, nil }
	t.Cleanup(func() { newDockerCommandManager = originalFactory })

	client.handleDockerCommand(dockerCommandMessage(
		t,
		"containers",
		"start",
		map[string]interface{}{"container_id": "web"},
	))
	response := <-responses
	assert.Equal(t, "docker_error", response.responseType)
	assert.Equal(t, "docker_container_start_failed", response.data["code"])
	encoded, err := json.Marshal(response.data)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private environment data")
}
