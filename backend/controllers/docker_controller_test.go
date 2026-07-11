package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utils "github.com/user/server-ops-backend/internal/agenttransport"
	"github.com/user/server-ops-backend/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDockerControllerTest(t *testing.T) models.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousDB := models.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Server{}))
	server := models.Server{Name: "docker-test", Online: true, Status: "online"}
	require.NoError(t, db.Create(&server).Error)
	models.DB = db

	t.Cleanup(func() {
		models.DB = previousDB
		utils.ConfigureAgentCommandSender(sendAgentCommandEnvelope)
		utils.FailAgentRequests(server.ID, errAgentConnectionClosed)
	})
	return server
}

func dockerTestContext(
	t *testing.T,
	method string,
	target string,
	body string,
	params gin.Params,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var requestBody *bytes.Reader
	requestBody = bytes.NewReader([]byte(body))
	req := httptest.NewRequest(method, target, requestBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = params
	return c, w
}

func TestDockerControllerRejectsInvalidInputsBeforeAgentDispatch(t *testing.T) {
	server := setupDockerControllerTest(t)
	dispatches := 0
	utils.ConfigureAgentCommandSender(func(_ uint, _ utils.AgentCommandEnvelope) error {
		dispatches++
		return nil
	})
	id := strconv.FormatUint(uint64(server.ID), 10)

	tests := []struct {
		name    string
		method  string
		target  string
		body    string
		params  gin.Params
		handler func(*gin.Context)
	}{
		{
			name: "non numeric tail", method: http.MethodGet, target: "/docker/logs?tail=abc",
			params: gin.Params{{Key: "id", Value: id}, {Key: "container_id", Value: "web"}}, handler: GetContainerLogs,
		},
		{
			name: "out of range tail", method: http.MethodGet, target: "/docker/logs?tail=0",
			params: gin.Params{{Key: "id", Value: id}, {Key: "container_id", Value: "web"}}, handler: GetContainerLogs,
		},
		{
			name: "non numeric timeout", method: http.MethodPost, target: "/docker/stop?timeout=nope",
			params: gin.Params{{Key: "id", Value: id}, {Key: "container_id", Value: "web"}}, handler: StopContainer,
		},
		{
			name: "invalid container ref", method: http.MethodPost, target: "/docker/start",
			params: gin.Params{{Key: "id", Value: id}, {Key: "container_id", Value: "-web"}}, handler: StartContainer,
		},
		{
			name: "invalid image", method: http.MethodPost, target: "/docker/images/pull", body: `{"image":"-alpine"}`,
			params: gin.Params{{Key: "id", Value: id}}, handler: PullImage,
		},
		{
			name: "invalid compose name", method: http.MethodPost, target: "/docker/composes/up",
			params: gin.Params{{Key: "id", Value: id}, {Key: "name", Value: "../project"}}, handler: ComposeUp,
		},
		{
			name: "oversized compose", method: http.MethodPost, target: "/docker/composes", body: `{"name":"project","content":"` + strings.Repeat("a", (1<<20)+1) + `"}`,
			params: gin.Params{{Key: "id", Value: id}}, handler: CreateCompose,
		},
		{
			name: "oversized create arrays", method: http.MethodPost, target: "/docker/containers", body: func() string {
				payload, err := json.Marshal(DockerCreateRequest{
					Name: "web", Image: "alpine:3.20", Ports: make([]string, 129),
				})
				require.NoError(t, err)
				return string(payload)
			}(),
			params: gin.Params{{Key: "id", Value: id}}, handler: CreateContainer,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, w := dockerTestContext(t, test.method, test.target, test.body, test.params)
			test.handler(c)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
	assert.Zero(t, dispatches)
}

func TestDockerControllerUsesTypedBrokerResponse(t *testing.T) {
	server := setupDockerControllerTest(t)
	sent := make(chan utils.AgentCommandEnvelope, 1)
	utils.ConfigureAgentCommandSender(func(serverID uint, command utils.AgentCommandEnvelope) error {
		assert.Equal(t, server.ID, serverID)
		sent <- command
		return nil
	})

	c, w := dockerTestContext(
		t,
		http.MethodGet,
		"/docker/images",
		"",
		gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(server.ID), 10)}},
	)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		GetImages(c)
		close(done)
	}()

	var command utils.AgentCommandEnvelope
	select {
	case command = <-sent:
	case <-done:
		t.Fatalf("handler returned before broker dispatch: status=%d body=%s", w.Code, w.Body.String())
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Docker broker dispatch")
	}
	assert.Equal(t, "docker_command", command.Type)
	payload, err := json.Marshal(command.Payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{"command":"images","action":"list"}`, string(payload))

	require.NoError(t, utils.DeliverAgentResponse(server.ID, []byte(`{
		"type":"docker_images",
		"request_id":"`+command.RequestID+`",
		"data":{"images":[{"id":"sha256:1"}]}
	}`)))

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after Agent response")
	}
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"images":[{"id":"sha256:1"}]}`, w.Body.String())
}

func TestLegacyDockerBrowserRPCRemoved(t *testing.T) {
	assert.True(t, rejectLegacyDockerBrowserRPC(false, TypeDockerCommand))
	assert.False(t, rejectLegacyDockerBrowserRPC(false, "docker_logs_stream"))
	assert.False(t, rejectLegacyDockerBrowserRPC(true, TypeDockerCommand))
}
