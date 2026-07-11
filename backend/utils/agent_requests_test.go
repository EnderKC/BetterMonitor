package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agentRequestTestResult struct {
	data json.RawMessage
	err  error
}

func resetAgentRequestBrokerTestState(t *testing.T) {
	t.Helper()

	ConfigureAgentCommandSender(nil)
	FailAgentRequests(7, errors.New("test cleanup"))
	FailAgentRequests(8, errors.New("test cleanup"))
	agentRequestIDGenerator = defaultAgentRequestIDGenerator

	t.Cleanup(func() {
		ConfigureAgentCommandSender(nil)
		FailAgentRequests(7, errors.New("test cleanup"))
		FailAgentRequests(8, errors.New("test cleanup"))
		agentRequestIDGenerator = defaultAgentRequestIDGenerator
		require.Zero(t, pendingAgentRequestCount())
	})
}

func configureAgentRequestTestSender(
	t *testing.T,
	requestIDs ...string,
) <-chan AgentCommandEnvelope {
	t.Helper()

	var next atomic.Uint64
	agentRequestIDGenerator = func() string {
		index := int(next.Add(1)) - 1
		if index >= len(requestIDs) {
			return fmt.Sprintf("request-%d", index+1)
		}
		return requestIDs[index]
	}

	sent := make(chan AgentCommandEnvelope, len(requestIDs)+1)
	ConfigureAgentCommandSender(func(_ uint, command AgentCommandEnvelope) error {
		sent <- command
		return nil
	})
	return sent
}

func startAgentRequest(
	ctx context.Context,
	serverID uint,
	payload interface{},
	allowedTypes ...string,
) <-chan agentRequestTestResult {
	result := make(chan agentRequestTestResult, 1)
	go func() {
		data, err := SendAgentCommand(ctx, serverID, "nginx_command", payload, allowedTypes...)
		result <- agentRequestTestResult{data: data, err: err}
	}()
	return result
}

func receiveAgentCommand(t *testing.T, sent <-chan AgentCommandEnvelope) AgentCommandEnvelope {
	t.Helper()

	select {
	case command := <-sent:
		return command
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Agent command")
		return AgentCommandEnvelope{}
	}
}

func receiveAgentRequestResult(t *testing.T, result <-chan agentRequestTestResult) agentRequestTestResult {
	t.Helper()

	select {
	case response := <-result:
		return response
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Agent request result")
		return agentRequestTestResult{}
	}
}

func TestAgentRequestBrokerSuccess(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	sent := configureAgentRequestTestSender(t, "request-success")
	payload := map[string]interface{}{"action": "nginx_status"}
	result := startAgentRequest(context.Background(), 7, payload, "nginx_success")

	command := receiveAgentCommand(t, sent)
	assert.Equal(t, "nginx_command", command.Type)
	assert.Equal(t, "request-success", command.RequestID)
	assert.Equal(t, payload, command.Payload)
	encoded, err := json.Marshal(command)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret_key")

	err = DeliverAgentResponse(7, []byte(`{
		"type":"nginx_success",
		"request_id":"request-success",
		"data":{"success":true}
	}`))
	require.NoError(t, err)

	response := receiveAgentRequestResult(t, result)
	require.NoError(t, response.err)
	assert.JSONEq(t, `{"success":true}`, string(response.data))
	assert.Zero(t, pendingAgentRequestCount())
}

func TestAgentRequestBrokerAgentError(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	sent := configureAgentRequestTestSender(t, "request-error")
	result := startAgentRequest(context.Background(), 7, nil, "nginx_success")
	receiveAgentCommand(t, sent)

	err := DeliverAgentResponse(7, []byte(`{
		"type":"nginx_error",
		"request_id":"request-error",
		"data":{"code":"nginx_config_invalid","error":"configuration rejected"}
	}`))
	require.NoError(t, err)

	response := receiveAgentRequestResult(t, result)
	var commandErr *AgentCommandError
	require.ErrorAs(t, response.err, &commandErr)
	assert.Equal(t, "nginx_config_invalid", commandErr.Code)
	assert.Equal(t, "configuration rejected", commandErr.Message)
	assert.Zero(t, pendingAgentRequestCount())
}

func TestAgentRequestBrokerContextCancellation(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	sent := configureAgentRequestTestSender(t, "request-cancel")
	ctx, cancel := context.WithCancel(context.Background())
	result := startAgentRequest(ctx, 7, nil, "nginx_success")
	receiveAgentCommand(t, sent)

	cancel()
	response := receiveAgentRequestResult(t, result)
	assert.ErrorIs(t, response.err, context.Canceled)
	assert.Zero(t, pendingAgentRequestCount())
	assert.ErrorIs(
		t,
		DeliverAgentResponse(7, []byte(`{"type":"nginx_success","request_id":"request-cancel","data":{}}`)),
		ErrAgentRequestNotFound,
	)
}

func TestAgentRequestBrokerContextTimeout(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	sent := configureAgentRequestTestSender(t, "request-timeout")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result := startAgentRequest(ctx, 7, nil, "nginx_success")
	receiveAgentCommand(t, sent)

	response := receiveAgentRequestResult(t, result)
	assert.ErrorIs(t, response.err, context.DeadlineExceeded)
	assert.Zero(t, pendingAgentRequestCount())
}

func TestAgentRequestBrokerSenderFailure(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	senderErr := errors.New("write failed")
	agentRequestIDGenerator = func() string { return "request-send-failure" }
	ConfigureAgentCommandSender(func(_ uint, _ AgentCommandEnvelope) error {
		return senderErr
	})

	_, err := SendAgentCommand(context.Background(), 7, "nginx_command", nil, "nginx_success")
	assert.ErrorIs(t, err, senderErr)
	assert.Zero(t, pendingAgentRequestCount())
}

func TestAgentRequestBrokerRejectsWrongServer(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	sent := configureAgentRequestTestSender(t, "request-server")
	result := startAgentRequest(context.Background(), 7, nil, "nginx_success")
	receiveAgentCommand(t, sent)

	message := []byte(`{"type":"nginx_success","request_id":"request-server","data":{"server":7}}`)
	assert.ErrorIs(t, DeliverAgentResponse(8, message), ErrAgentResponseServerMismatch)
	assert.Equal(t, 1, pendingAgentRequestCount())
	require.NoError(t, DeliverAgentResponse(7, message))
	require.NoError(t, receiveAgentRequestResult(t, result).err)
}

func TestAgentRequestBrokerRejectsWrongResponseType(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	sent := configureAgentRequestTestSender(t, "request-type")
	result := startAgentRequest(context.Background(), 7, nil, "nginx_success")
	receiveAgentCommand(t, sent)

	assert.ErrorIs(
		t,
		DeliverAgentResponse(7, []byte(`{"type":"docker_images","request_id":"request-type","data":{}}`)),
		ErrAgentResponseTypeNotAllowed,
	)
	assert.Equal(t, 1, pendingAgentRequestCount())
	require.NoError(t, DeliverAgentResponse(7, []byte(`{"type":"nginx_success","request_id":"request-type","data":{}}`)))
	require.NoError(t, receiveAgentRequestResult(t, result).err)
}

func TestAgentRequestBrokerRejectsDuplicateAndLateResponses(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	sent := configureAgentRequestTestSender(t, "request-duplicate")
	result := startAgentRequest(context.Background(), 7, nil, "nginx_success")
	receiveAgentCommand(t, sent)
	message := []byte(`{"type":"nginx_success","request_id":"request-duplicate","data":{}}`)

	require.NoError(t, DeliverAgentResponse(7, message))
	assert.ErrorIs(t, DeliverAgentResponse(7, message), ErrAgentRequestNotFound)
	require.NoError(t, receiveAgentRequestResult(t, result).err)
	assert.Zero(t, pendingAgentRequestCount())
	assert.ErrorIs(t, DeliverAgentResponse(7, message), ErrAgentRequestNotFound)
}

func TestAgentRequestBrokerFailsRequestsForDisconnectedServer(t *testing.T) {
	resetAgentRequestBrokerTestState(t)
	sent := configureAgentRequestTestSender(t, "request-7-a", "request-7-b", "request-8")
	request7A := startAgentRequest(context.Background(), 7, nil, "nginx_success")
	receiveAgentCommand(t, sent)
	request7B := startAgentRequest(context.Background(), 7, nil, "nginx_success")
	receiveAgentCommand(t, sent)
	request8 := startAgentRequest(context.Background(), 8, nil, "nginx_success")
	receiveAgentCommand(t, sent)
	assert.Equal(t, 3, pendingAgentRequestCount())

	disconnectErr := errors.New("Agent disconnected")
	FailAgentRequests(7, disconnectErr)
	assert.ErrorIs(t, receiveAgentRequestResult(t, request7A).err, disconnectErr)
	assert.ErrorIs(t, receiveAgentRequestResult(t, request7B).err, disconnectErr)
	assert.Equal(t, 1, pendingAgentRequestCount())

	require.NoError(t, DeliverAgentResponse(8, []byte(fmt.Sprintf(
		`{"type":"nginx_success","request_id":%q,"data":{"success":true}}`,
		"request-8",
	))))
	require.NoError(t, receiveAgentRequestResult(t, request8).err)
	assert.Zero(t, pendingAgentRequestCount())
}
