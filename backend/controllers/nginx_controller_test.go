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

func setupNginxControllerTest(t *testing.T) models.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousDB := models.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Server{}))
	server := models.Server{
		Name:                  "nginx-test",
		Online:                true,
		Status:                "online",
		LastHeartbeat:         time.Now(),
		AgentHeartbeatSeconds: 10,
	}
	require.NoError(t, db.Create(&server).Error)
	models.DB = db
	t.Cleanup(func() {
		models.DB = previousDB
		utils.ConfigureAgentCommandSender(sendAgentCommandEnvelope)
		utils.FailAgentRequests(server.ID, errAgentConnectionClosed)
	})
	return server
}

func nginxTestContext(
	t *testing.T,
	method string,
	target string,
	body string,
	params gin.Params,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = params
	return c, w
}

func TestNginxControllerRejectsPathFallbackAndInvalidManagedInputs(t *testing.T) {
	server := setupNginxControllerTest(t)
	serverID := strconv.FormatUint(uint64(server.ID), 10)
	dispatches := 0
	utils.ConfigureAgentCommandSender(func(_ uint, _ utils.AgentCommandEnvelope) error {
		dispatches++
		return nil
	})

	tests := []struct {
		name    string
		method  string
		body    string
		params  gin.Params
		handler func(*gin.Context)
	}{
		{
			name: "invalid config id", method: http.MethodGet,
			params: gin.Params{{Key: "id", Value: serverID}, {Key: "config_id", Value: "not-an-id"}}, handler: NginxConfigContent,
		},
		{
			name: "save path field", method: http.MethodPut, body: `{"content":"server {}","path":"/etc/passwd"}`,
			params: gin.Params{{Key: "id", Value: serverID}, {Key: "config_id", Value: strings.Repeat("a", 32)}}, handler: SaveNginxConfig,
		},
		{
			name: "create path field", method: http.MethodPost, body: `{"name":"site","path":"/tmp/site.conf","content":"server {}"}`,
			params: gin.Params{{Key: "id", Value: serverID}}, handler: CreateNginxConfig,
		},
		{
			name: "create traversal name", method: http.MethodPost, body: `{"name":"../site","content":"server {}"}`,
			params: gin.Params{{Key: "id", Value: serverID}}, handler: CreateNginxConfig,
		},
		{
			name: "oversized content", method: http.MethodPut, body: `{"content":"` + strings.Repeat("a", (1<<20)+1) + `"}`,
			params: gin.Params{{Key: "id", Value: serverID}, {Key: "config_id", Value: strings.Repeat("a", 32)}}, handler: SaveNginxConfig,
		},
		{
			name: "invalid log id", method: http.MethodGet,
			params: gin.Params{{Key: "id", Value: serverID}, {Key: "log_id", Value: "not-an-id"}}, handler: NginxLogContent,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, w := nginxTestContext(t, test.method, "/nginx", test.body, test.params)
			test.handler(c)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
	assert.Zero(t, dispatches)
}

func TestNginxControllerSaveUsesSingleIDOnlyBrokerRequest(t *testing.T) {
	server := setupNginxControllerTest(t)
	serverID := strconv.FormatUint(uint64(server.ID), 10)
	configID := strings.Repeat("a", 32)
	sent := make(chan utils.AgentCommandEnvelope, 1)
	utils.ConfigureAgentCommandSender(func(_ uint, command utils.AgentCommandEnvelope) error {
		sent <- command
		return nil
	})
	c, w := nginxTestContext(
		t,
		http.MethodPut,
		"/nginx/configs/"+configID,
		`{"content":"server { listen 80; }"}`,
		gin.Params{{Key: "id", Value: serverID}, {Key: "config_id", Value: configID}},
	)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		SaveNginxConfig(c)
		close(done)
	}()

	var command utils.AgentCommandEnvelope
	select {
	case command = <-sent:
	case <-done:
		t.Fatalf("handler returned before broker dispatch: status=%d body=%s", w.Code, w.Body.String())
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Nginx broker request")
	}
	assert.Equal(t, "nginx_command", command.Type)
	payload, err := json.Marshal(command.Payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{"action":"nginx_save_config","config_id":"`+configID+`","content":"server { listen 80; }"}`, string(payload))
	assert.NotContains(t, string(payload), `"path"`)

	require.NoError(t, utils.DeliverAgentResponse(server.ID, []byte(`{
		"type":"nginx_success",
		"request_id":"`+command.RequestID+`",
		"data":{"success":true}
	}`)))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after Nginx response")
	}
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"success":true}`, w.Body.String())
}

func TestNginxControllerCreateAndLogUseManagedNamesAndLogIDs(t *testing.T) {
	server := setupNginxControllerTest(t)
	serverID := strconv.FormatUint(uint64(server.ID), 10)
	logID := strings.Repeat("b", 32)
	sent := make(chan utils.AgentCommandEnvelope, 2)
	utils.ConfigureAgentCommandSender(func(_ uint, command utils.AgentCommandEnvelope) error {
		sent <- command
		return nil
	})

	tests := []struct {
		name         string
		method       string
		body         string
		params       gin.Params
		handler      func(*gin.Context)
		wantPayload  string
		responseData string
	}{
		{
			name: "create", method: http.MethodPost, body: `{"name":"site","content":"server {}"}`,
			params: gin.Params{{Key: "id", Value: serverID}}, handler: CreateNginxConfig,
			wantPayload: `{"action":"nginx_create_config","name":"site","content":"server {}"}`, responseData: `{"success":true}`,
		},
		{
			name: "log content", method: http.MethodGet,
			params: gin.Params{{Key: "id", Value: serverID}, {Key: "log_id", Value: logID}}, handler: NginxLogContent,
			wantPayload: `{"action":"nginx_log_content","log_id":"` + logID + `"}`, responseData: `"log line"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, w := nginxTestContext(t, test.method, "/nginx", test.body, test.params)
			done := make(chan struct{})
			go func() {
				test.handler(c)
				close(done)
			}()
			command := <-sent
			payload, err := json.Marshal(command.Payload)
			require.NoError(t, err)
			assert.JSONEq(t, test.wantPayload, string(payload))
			assert.NotContains(t, string(payload), `"path"`)
			require.NoError(t, utils.DeliverAgentResponse(server.ID, []byte(`{
				"type":"nginx_success",
				"request_id":"`+command.RequestID+`",
				"data":`+test.responseData+`
			}`)))
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("handler did not finish")
			}
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestNginxControllerServiceAndWebsiteQueriesUseBroker(t *testing.T) {
	server := setupNginxControllerTest(t)
	serverID := strconv.FormatUint(uint64(server.ID), 10)
	sent := make(chan utils.AgentCommandEnvelope, 1)
	utils.ConfigureAgentCommandSender(func(_ uint, command utils.AgentCommandEnvelope) error {
		sent <- command
		return nil
	})

	tests := []struct {
		name         string
		params       gin.Params
		handler      func(*gin.Context)
		wantPayload  string
		responseData string
	}{
		{
			name: "restart", params: gin.Params{{Key: "id", Value: serverID}}, handler: RestartNginx,
			wantPayload: `{"action":"nginx_restart"}`, responseData: `{"success":true}`,
		},
		{
			name: "processes", params: gin.Params{{Key: "id", Value: serverID}}, handler: GetNginxProcesses,
			wantPayload: `{"action":"nginx_processes"}`, responseData: `[]`,
		},
		{
			name: "website detail", params: gin.Params{{Key: "id", Value: serverID}, {Key: "domain", Value: "Example.COM"}}, handler: GetWebsiteDetail,
			wantPayload: `{"action":"nginx_site_detail","domain":"example.com"}`, responseData: `{"domain":"example.com"}`,
		},
		{
			name: "openresty status", params: gin.Params{{Key: "id", Value: serverID}}, handler: OpenRestyStatus,
			wantPayload: `{"action":"openresty_status"}`, responseData: `{"installed":true}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, w := nginxTestContext(t, http.MethodGet, "/nginx", "", test.params)
			done := make(chan struct{})
			go func() {
				test.handler(c)
				close(done)
			}()

			var command utils.AgentCommandEnvelope
			select {
			case command = <-sent:
			case <-done:
				t.Fatalf("handler returned before broker dispatch: status=%d body=%s", w.Code, w.Body.String())
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for Nginx broker request")
			}
			payload, err := json.Marshal(command.Payload)
			require.NoError(t, err)
			assert.JSONEq(t, test.wantPayload, string(payload))
			require.NoError(t, utils.DeliverAgentResponse(server.ID, []byte(`{
				"type":"nginx_success",
				"request_id":"`+command.RequestID+`",
				"data":`+test.responseData+`
			}`)))
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("handler did not finish")
			}
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		})
	}
}
