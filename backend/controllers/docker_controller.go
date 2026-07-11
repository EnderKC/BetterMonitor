package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
)

func parseServerId(idText string) (uint, error) {
	id, err := strconv.ParseUint(idText, 10, 32)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid server ID")
	}
	return uint(id), nil
}

func parseIntParam(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer parameter")
	}
	return parsed, nil
}

func parseDockerBoolQuery(c *gin.Context, name string, defaultValue bool) (bool, error) {
	value, exists := c.GetQuery(name)
	if !exists {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func loadDockerServer(c *gin.Context, serverID uint) (*models.Server, bool) {
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "server_not_found", "error": "服务器不存在"})
		return nil, false
	}
	return server, true
}

func rejectDockerRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{
		"code":  "invalid_docker_request",
		"error": err.Error(),
	})
}

func sendDockerAgentRequest(
	ctx context.Context,
	serverID uint,
	payload DockerCommandPayload,
) (map[string]interface{}, error) {
	spec, err := LookupDockerActionSpec(payload.Command, payload.Action)
	if err != nil {
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	response, err := utils.SendAgentCommand(
		requestCtx,
		serverID,
		"docker_command",
		payload,
		spec.AllowedResponse,
	)
	if err != nil {
		return nil, err
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(response, &decoded); err != nil {
		return nil, fmt.Errorf("decode Docker Agent response: %w", err)
	}
	if decoded == nil {
		decoded = map[string]interface{}{}
	}
	return decoded, nil
}

func respondDockerAgent(c *gin.Context, serverID uint, payload DockerCommandPayload) {
	response, err := sendDockerAgentRequest(c.Request.Context(), serverID, payload)
	if err == nil {
		c.JSON(http.StatusOK, response)
		return
	}

	status := http.StatusBadGateway
	code := "docker_agent_failed"
	message := "Docker 操作失败"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
		code = "docker_agent_timeout"
		message = "Docker 操作超时"
	case errors.Is(err, context.Canceled):
		status = http.StatusRequestTimeout
		code = "docker_request_canceled"
		message = "Docker 请求已取消"
	case errors.Is(err, utils.ErrAgentCommandSenderNotConfigured):
		status = http.StatusServiceUnavailable
		code = "agent_unavailable"
		message = "服务器 Agent 不可用"
	default:
		var commandErr *utils.AgentCommandError
		if errors.As(err, &commandErr) {
			code = commandErr.Code
		}
	}
	c.JSON(status, gin.H{"code": code, "error": message})
}

func GetContainers(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{Command: "containers", Action: "list"})
}

func GetContainerLogs(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	containerID := c.Param("container_id")
	if err := ValidateContainerRef(containerID); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	tail := 100
	if value, exists := c.GetQuery("tail"); exists {
		tail, err = parseIntParam(value)
		if err != nil {
			rejectDockerRequest(c, err)
			return
		}
	}
	if err := ValidateDockerTail(tail); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "containers",
		Action:  "logs",
		Params:  DockerContainerLogsParams{ContainerID: containerID, Tail: tail},
	})
}

func StartContainer(c *gin.Context) {
	handleContainerRefAction(c, "start")
}

func StopContainer(c *gin.Context) {
	handleContainerTimeoutAction(c, "stop")
}

func RestartContainer(c *gin.Context) {
	handleContainerTimeoutAction(c, "restart")
}

func handleContainerRefAction(c *gin.Context, action string) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	containerID := c.Param("container_id")
	if err := ValidateContainerRef(containerID); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "containers",
		Action:  action,
		Params:  DockerContainerRefParams{ContainerID: containerID},
	})
}

func handleContainerTimeoutAction(c *gin.Context, action string) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	containerID := c.Param("container_id")
	if err := ValidateContainerRef(containerID); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	timeout := 10
	if value, exists := c.GetQuery("timeout"); exists {
		timeout, err = parseIntParam(value)
		if err != nil {
			rejectDockerRequest(c, err)
			return
		}
	}
	if err := ValidateDockerTimeout(timeout); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "containers",
		Action:  action,
		Params:  DockerContainerTimeoutParams{ContainerID: containerID, Timeout: timeout},
	})
}

func RemoveContainer(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	containerID := c.Param("container_id")
	if err := ValidateContainerRef(containerID); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	force, err := parseDockerBoolQuery(c, "force", false)
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "containers",
		Action:  "remove",
		Params:  DockerContainerRemoveParams{ContainerID: containerID, Force: force},
	})
}

func CreateContainer(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	var request DockerCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		rejectDockerRequest(c, fmt.Errorf("invalid request body"))
		return
	}
	if err := ValidateDockerCreate(request); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "containers",
		Action:  "create",
		Params:  request,
	})
}

func GetImages(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{Command: "images", Action: "list"})
}

func PullImage(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	var request DockerImagePullParams
	if err := c.ShouldBindJSON(&request); err != nil {
		rejectDockerRequest(c, fmt.Errorf("invalid request body"))
		return
	}
	if err := ValidateImageRef(request.Image); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "images",
		Action:  "pull",
		Params:  request,
	})
}

func RemoveImage(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	imageID := c.Param("image_id")
	if err := ValidateImageRef(imageID); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	force, err := parseDockerBoolQuery(c, "force", false)
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "images",
		Action:  "remove",
		Params:  DockerImageRemoveParams{ImageID: imageID, Force: force},
	})
}

func GetComposes(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{Command: "composes", Action: "list"})
}

func GetComposeConfig(c *gin.Context) {
	handleComposeNameAction(c, "config")
}

func ComposeUp(c *gin.Context) {
	handleComposeNameAction(c, "up")
}

func ComposeDown(c *gin.Context) {
	handleComposeNameAction(c, "down")
}

func RemoveCompose(c *gin.Context) {
	handleComposeNameAction(c, "remove")
}

func handleComposeNameAction(c *gin.Context, action string) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	name := c.Param("name")
	if err := ValidateComposeName(name); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "composes",
		Action:  action,
		Params:  DockerComposeNameParams{Name: name},
	})
}

func CreateCompose(c *gin.Context) {
	serverID, err := parseServerId(c.Param("id"))
	if err != nil {
		rejectDockerRequest(c, err)
		return
	}
	var request DockerComposeCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		rejectDockerRequest(c, fmt.Errorf("invalid request body"))
		return
	}
	if err := ValidateComposeName(request.Name); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if err := ValidateComposeContent(request.Content); err != nil {
		rejectDockerRequest(c, err)
		return
	}
	if _, ok := loadDockerServer(c, serverID); !ok {
		return
	}
	respondDockerAgent(c, serverID, DockerCommandPayload{
		Command: "composes",
		Action:  "create",
		Params:  request,
	})
}
