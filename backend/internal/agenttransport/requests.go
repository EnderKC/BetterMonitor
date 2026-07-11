package agenttransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/google/uuid"
)

var (
	ErrAgentCommandSenderNotConfigured = errors.New("Agent command sender is not configured")
	ErrAgentRequestNotFound            = errors.New("Agent request not found")
	ErrAgentResponseServerMismatch     = errors.New("Agent response server mismatch")
	ErrAgentResponseTypeNotAllowed     = errors.New("Agent response type not allowed")
	ErrAgentResponseInvalid            = errors.New("invalid Agent response")
)

type AgentCommandEnvelope struct {
	Type      string      `json:"type"`
	RequestID string      `json:"request_id"`
	Payload   interface{} `json:"payload"`
}

type AgentResponseEnvelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Status    string          `json:"status,omitempty"`
	Code      string          `json:"code,omitempty"`
	Error     string          `json:"error,omitempty"`
	Message   string          `json:"message,omitempty"`
	Data      json.RawMessage `json:"data"`
}

type AgentCommandSender func(serverID uint, command AgentCommandEnvelope) error

type AgentCommandError struct {
	Code         string
	Message      string
	ResponseType string
}

func (e *AgentCommandError) Error() string {
	if e == nil {
		return "Agent command failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return "Agent command failed"
}

type agentRequestResult struct {
	data json.RawMessage
	err  error
}

type pendingAgentRequest struct {
	serverID    uint
	allowedType map[string]struct{}
	result      chan agentRequestResult
}

var agentRequests = struct {
	sync.Mutex
	sender  AgentCommandSender
	pending map[string]*pendingAgentRequest
}{
	pending: make(map[string]*pendingAgentRequest),
}

var agentRequestIDGenerator = defaultAgentRequestIDGenerator

func defaultAgentRequestIDGenerator() string {
	return uuid.NewString()
}

func ConfigureAgentCommandSender(sender AgentCommandSender) {
	agentRequests.Lock()
	agentRequests.sender = sender
	agentRequests.Unlock()
}

func SendAgentCommand(
	ctx context.Context,
	serverID uint,
	commandType string,
	payload interface{},
	allowedTypes ...string,
) (json.RawMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is nil", ErrAgentResponseInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	commandType = strings.TrimSpace(commandType)
	if commandType == "" {
		return nil, fmt.Errorf("%w: command type is empty", ErrAgentResponseInvalid)
	}

	allowed := make(map[string]struct{}, len(allowedTypes))
	for _, responseType := range allowedTypes {
		responseType = strings.TrimSpace(responseType)
		if responseType != "" {
			allowed[responseType] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("%w: no response types allowed", ErrAgentResponseTypeNotAllowed)
	}

	request := &pendingAgentRequest{
		serverID:    serverID,
		allowedType: allowed,
		result:      make(chan agentRequestResult, 1),
	}

	requestID, sender, err := registerAgentRequest(request)
	if err != nil {
		return nil, err
	}

	command := AgentCommandEnvelope{
		Type:      commandType,
		RequestID: requestID,
		Payload:   payload,
	}
	if err := sender(serverID, command); err != nil {
		removeAgentRequest(requestID, request)
		return nil, fmt.Errorf("send Agent command: %w", err)
	}

	select {
	case response := <-request.result:
		return response.data, response.err
	case <-ctx.Done():
		if removeAgentRequest(requestID, request) {
			return nil, ctx.Err()
		}
		response := <-request.result
		return response.data, response.err
	}
}

func registerAgentRequest(request *pendingAgentRequest) (string, AgentCommandSender, error) {
	agentRequests.Lock()
	defer agentRequests.Unlock()

	if agentRequests.sender == nil {
		return "", nil, ErrAgentCommandSenderNotConfigured
	}

	for range 4 {
		requestID := strings.TrimSpace(agentRequestIDGenerator())
		if requestID == "" {
			continue
		}
		if _, exists := agentRequests.pending[requestID]; exists {
			continue
		}
		agentRequests.pending[requestID] = request
		return requestID, agentRequests.sender, nil
	}

	return "", nil, fmt.Errorf("generate Agent request ID: repeated or empty identifier")
}

func removeAgentRequest(requestID string, request *pendingAgentRequest) bool {
	agentRequests.Lock()
	defer agentRequests.Unlock()

	current, exists := agentRequests.pending[requestID]
	if !exists || current != request {
		return false
	}
	delete(agentRequests.pending, requestID)
	return true
}

func DeliverAgentResponse(serverID uint, message []byte) error {
	var response AgentResponseEnvelope
	if err := json.Unmarshal(message, &response); err != nil {
		return fmt.Errorf("%w: %v", ErrAgentResponseInvalid, err)
	}
	if strings.TrimSpace(response.RequestID) == "" || strings.TrimSpace(response.Type) == "" {
		return fmt.Errorf("%w: response type or request ID is empty", ErrAgentResponseInvalid)
	}

	agentRequests.Lock()
	request, exists := agentRequests.pending[response.RequestID]
	if !exists {
		agentRequests.Unlock()
		return fmt.Errorf("%w: %s", ErrAgentRequestNotFound, response.RequestID)
	}
	if request.serverID != serverID {
		agentRequests.Unlock()
		return fmt.Errorf(
			"%w: request %s belongs to server %d",
			ErrAgentResponseServerMismatch,
			response.RequestID,
			request.serverID,
		)
	}

	isError := isAgentErrorResponse(response)
	if !isError {
		if _, allowed := request.allowedType[response.Type]; !allowed {
			agentRequests.Unlock()
			return fmt.Errorf(
				"%w: request %s does not allow %s",
				ErrAgentResponseTypeNotAllowed,
				response.RequestID,
				response.Type,
			)
		}
	}
	delete(agentRequests.pending, response.RequestID)
	agentRequests.Unlock()

	result := agentRequestResult{data: response.Data}
	if isError {
		result.err = projectAgentCommandError(response)
	}
	request.result <- result
	return nil
}

func FailAgentRequests(serverID uint, cause error) {
	if cause == nil {
		cause = errors.New("Agent connection closed")
	}

	agentRequests.Lock()
	failed := make([]*pendingAgentRequest, 0)
	for requestID, request := range agentRequests.pending {
		if request.serverID != serverID {
			continue
		}
		delete(agentRequests.pending, requestID)
		failed = append(failed, request)
	}
	agentRequests.Unlock()

	for _, request := range failed {
		request.result <- agentRequestResult{err: cause}
	}
}

func pendingAgentRequestCount() int {
	agentRequests.Lock()
	defer agentRequests.Unlock()
	return len(agentRequests.pending)
}

func isAgentErrorResponse(response AgentResponseEnvelope) bool {
	if strings.EqualFold(response.Status, "error") {
		return true
	}
	switch response.Type {
	case "error", "docker_error", "nginx_error":
		return true
	default:
		return false
	}
}

func projectAgentCommandError(response AgentResponseEnvelope) error {
	projection := struct {
		Code    string `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}{}
	if len(response.Data) > 0 && string(response.Data) != "null" {
		_ = json.Unmarshal(response.Data, &projection)
	}

	code := safeAgentErrorCode(response.Code)
	if code == "agent_error" {
		code = safeAgentErrorCode(projection.Code)
	}
	message := safeAgentErrorMessage(response.Error)
	if message == "" {
		message = safeAgentErrorMessage(response.Message)
	}
	if message == "" {
		message = safeAgentErrorMessage(projection.Error)
	}
	if message == "" {
		message = safeAgentErrorMessage(projection.Message)
	}
	if message == "" {
		message = "Agent command failed"
	}

	return &AgentCommandError{
		Code:         code,
		Message:      message,
		ResponseType: response.Type,
	}
}

func safeAgentErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 64 {
		return "agent_error"
	}
	for _, char := range code {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return "agent_error"
	}
	return code
}

func safeAgentErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" || len(message) > 512 {
		return ""
	}
	for _, char := range message {
		if unicode.IsControl(char) && char != '\n' && char != '\t' {
			return ""
		}
	}
	return message
}
