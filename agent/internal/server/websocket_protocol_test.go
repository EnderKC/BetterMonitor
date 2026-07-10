package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gorilla/websocket"
	"github.com/user/server-ops-agent/config"
	"github.com/user/server-ops-agent/pkg/logger"
	"github.com/user/server-ops-agent/pkg/version"
)

func TestBuildAgentWebSocketURLDoesNotContainSecret(t *testing.T) {
	url, err := BuildAgentWebSocketURL("https://monitor.example.com", 7)
	require.NoError(t, err)
	assert.Equal(t, "wss://monitor.example.com/api/servers/7/agent-ws", url)
	assert.NotContains(t, url, "token")
	assert.NotContains(t, url, "secret")
}

func TestBuildAgentWebSocketURLNormalizesSupportedSchemes(t *testing.T) {
	tests := map[string]string{
		"monitor.example.com:8080":      "ws://monitor.example.com:8080/api/servers/7/agent-ws",
		"http://monitor.example.com":    "ws://monitor.example.com/api/servers/7/agent-ws",
		"ws://monitor.example.com/base": "ws://monitor.example.com/api/servers/7/agent-ws",
		"wss://monitor.example.com":     "wss://monitor.example.com/api/servers/7/agent-ws",
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			actual, err := BuildAgentWebSocketURL(input, 7)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}

	_, err := BuildAgentWebSocketURL("ftp://monitor.example.com", 7)
	assert.Error(t, err)
	_, err = BuildAgentWebSocketURL("https://monitor.example.com", 0)
	assert.Error(t, err)
}

func TestBuildAgentWebSocketHeadersContainsCredentialAndMetadata(t *testing.T) {
	hello := AgentHello{
		Type:                     "agent_hello",
		Version:                  "1.2.3",
		AgentType:                "full",
		HeartbeatIntervalSeconds: 10,
	}

	headers := BuildAgentWebSocketHeaders("server-secret", hello)
	assert.Equal(t, "server-secret", headers.Get("X-Secret-Key"))
	assert.Equal(t, "1.2.3", headers.Get("X-Agent-Version"))
	assert.Equal(t, "full", headers.Get("X-Agent-Type"))
	assert.Equal(t, "10", headers.Get("X-Agent-Heartbeat-Seconds"))
}

func TestRunAgentHeartbeatStopsWithConnectionContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var sends atomic.Int32
	reached := make(chan struct{})
	var once sync.Once
	done := make(chan error, 1)

	go func() {
		done <- runAgentHeartbeat(ctx, 5*time.Millisecond, time.Now, func(heartbeat AgentHeartbeat) error {
			assert.Equal(t, "agent_heartbeat", heartbeat.Type)
			assert.NotZero(t, heartbeat.Timestamp)
			if sends.Add(1) >= 2 {
				once.Do(func() { close(reached) })
			}
			return nil
		})
	}()

	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("heartbeat loop did not send twice")
	}
	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
	countAfterCancel := sends.Load()
	time.Sleep(15 * time.Millisecond)
	assert.Equal(t, countAfterCancel, sends.Load())
}

func TestRunAgentHeartbeatReturnsSendError(t *testing.T) {
	expected := errors.New("send failed")
	err := runAgentHeartbeat(context.Background(), time.Millisecond, time.Now, func(AgentHeartbeat) error {
		return expected
	})
	assert.ErrorIs(t, err, expected)
}

func TestClientAgentWebSocketHandshakeAndReconnect(t *testing.T) {
	type observation struct {
		path      string
		rawQuery  string
		headers   http.Header
		hello     AgentHello
		heartbeat AgentHeartbeat
		err       error
	}

	observations := make(chan observation, 2)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/servers/7/agent-ws" {
			http.NotFound(w, r)
			return
		}
		obs := observation{
			path:     r.URL.Path,
			rawQuery: r.URL.RawQuery,
			headers:  r.Header.Clone(),
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			obs.err = err
			observations <- obs
			return
		}
		defer conn.Close()
		if err := conn.ReadJSON(&obs.hello); err != nil {
			obs.err = err
			observations <- obs
			return
		}
		if err := conn.ReadJSON(&obs.heartbeat); err != nil {
			obs.err = err
			observations <- obs
			return
		}
		observations <- obs
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	log, err := logger.New("", "error")
	require.NoError(t, err)
	client := New(&config.Config{
		ServerURL:         server.URL,
		ServerID:          7,
		SecretKey:         "server-secret",
		HeartbeatInterval: time.Second,
	}, log)
	t.Cleanup(client.CloseWebSocket)
	nextObservation := func() observation {
		t.Helper()
		select {
		case obs := <-observations:
			return obs
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for agent handshake")
			return observation{}
		}
	}

	require.NoError(t, client.ConnectWebSocket())
	first := nextObservation()
	require.NoError(t, first.err)

	require.NoError(t, client.ConnectWebSocket())
	second := nextObservation()
	require.NoError(t, second.err)

	for _, obs := range []observation{first, second} {
		assert.Equal(t, "/api/servers/7/agent-ws", obs.path)
		assert.Empty(t, obs.rawQuery)
		assert.Equal(t, "server-secret", obs.headers.Get("X-Secret-Key"))
		assert.Equal(t, version.Version, obs.headers.Get("X-Agent-Version"))
		assert.Equal(t, version.AgentType, obs.headers.Get("X-Agent-Type"))
		assert.Equal(t, "1", obs.headers.Get("X-Agent-Heartbeat-Seconds"))
		assert.Equal(t, "agent_hello", obs.hello.Type)
		assert.Equal(t, version.Version, obs.hello.Version)
		assert.Equal(t, version.AgentType, obs.hello.AgentType)
		assert.Equal(t, 1, obs.hello.HeartbeatIntervalSeconds)
		assert.Equal(t, "agent_heartbeat", obs.heartbeat.Type)
		assert.NotZero(t, obs.heartbeat.Timestamp)
	}
	assert.NotEqual(t, strings.TrimSpace(first.rawQuery), "token=server-secret")
}
