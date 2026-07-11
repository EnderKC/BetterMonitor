package controllers

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	maxDockerImageRefBytes       = 512
	maxDockerContainerRefBytes   = 128
	maxDockerComposeNameBytes    = 128
	maxDockerComposeContentBytes = 1 << 20
	maxDockerCreatePorts         = 128
	maxDockerCreateVolumes       = 128
	maxDockerCreateEnv           = 256
	maxDockerPortBytes           = 256
	maxDockerVolumeBytes         = 1024
	maxDockerEnvKeyBytes         = 256
	maxDockerEnvValueBytes       = 4096
	maxDockerCommandBytes        = 8192
)

type DockerActionSpec struct {
	Command         string
	Action          string
	AllowedResponse string
	Timeout         time.Duration
}

type DockerCreateRequest struct {
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Ports   []string          `json:"ports"`
	Volumes []string          `json:"volumes"`
	Env     map[string]string `json:"env"`
	Command string            `json:"command"`
	Restart string            `json:"restart"`
	Network string            `json:"network"`
}

type DockerCommandPayload struct {
	Command string      `json:"command"`
	Action  string      `json:"action"`
	Params  interface{} `json:"params,omitempty"`
}

type DockerContainerRefParams struct {
	ContainerID string `json:"container_id"`
}

type DockerContainerLogsParams struct {
	ContainerID string `json:"container_id"`
	Tail        int    `json:"tail"`
}

type DockerContainerTimeoutParams struct {
	ContainerID string `json:"container_id"`
	Timeout     int    `json:"timeout"`
}

type DockerContainerRemoveParams struct {
	ContainerID string `json:"container_id"`
	Force       bool   `json:"force"`
}

type DockerImagePullParams struct {
	Image string `json:"image"`
}

type DockerImageRemoveParams struct {
	ImageID string `json:"image_id"`
	Force   bool   `json:"force"`
}

type DockerComposeNameParams struct {
	Name string `json:"name"`
}

type DockerComposeCreateRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

var dockerActionSpecs = map[[2]string]DockerActionSpec{
	{"containers", "list"}:    {Command: "containers", Action: "list", AllowedResponse: "docker_containers", Timeout: 30 * time.Second},
	{"containers", "logs"}:    {Command: "containers", Action: "logs", AllowedResponse: "docker_container_logs", Timeout: 30 * time.Second},
	{"containers", "start"}:   {Command: "containers", Action: "start", AllowedResponse: "success", Timeout: 2 * time.Minute},
	{"containers", "stop"}:    {Command: "containers", Action: "stop", AllowedResponse: "success", Timeout: 2 * time.Minute},
	{"containers", "restart"}: {Command: "containers", Action: "restart", AllowedResponse: "success", Timeout: 2 * time.Minute},
	{"containers", "remove"}:  {Command: "containers", Action: "remove", AllowedResponse: "success", Timeout: 2 * time.Minute},
	{"containers", "create"}:  {Command: "containers", Action: "create", AllowedResponse: "success", Timeout: 2 * time.Minute},
	{"images", "list"}:        {Command: "images", Action: "list", AllowedResponse: "docker_images", Timeout: 30 * time.Second},
	{"images", "pull"}:        {Command: "images", Action: "pull", AllowedResponse: "success", Timeout: 10 * time.Minute},
	{"images", "remove"}:      {Command: "images", Action: "remove", AllowedResponse: "success", Timeout: 2 * time.Minute},
	{"composes", "list"}:      {Command: "composes", Action: "list", AllowedResponse: "docker_composes", Timeout: 30 * time.Second},
	{"composes", "config"}:    {Command: "composes", Action: "config", AllowedResponse: "docker_compose_config", Timeout: 30 * time.Second},
	{"composes", "up"}:        {Command: "composes", Action: "up", AllowedResponse: "success", Timeout: 10 * time.Minute},
	{"composes", "down"}:      {Command: "composes", Action: "down", AllowedResponse: "success", Timeout: 10 * time.Minute},
	{"composes", "create"}:    {Command: "composes", Action: "create", AllowedResponse: "success", Timeout: 2 * time.Minute},
	{"composes", "remove"}:    {Command: "composes", Action: "remove", AllowedResponse: "success", Timeout: 2 * time.Minute},
}

func LookupDockerActionSpec(command, action string) (DockerActionSpec, error) {
	key := [2]string{strings.TrimSpace(command), strings.TrimSpace(action)}
	spec, ok := dockerActionSpecs[key]
	if !ok {
		return DockerActionSpec{}, fmt.Errorf("unsupported Docker action %q/%q", key[0], key[1])
	}
	return spec, nil
}

func ValidateContainerRef(value string) error {
	if err := validateDockerIdentifier(value, maxDockerContainerRefBytes, false); err != nil {
		return fmt.Errorf("invalid container reference: %w", err)
	}
	return nil
}

func ValidateImageRef(value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("image reference is empty or padded")
	}
	if len(value) > maxDockerImageRefBytes {
		return fmt.Errorf("image reference exceeds %d bytes", maxDockerImageRefBytes)
	}
	if value[0] == '-' {
		return fmt.Errorf("image reference cannot start with '-'")
	}
	for _, char := range value {
		if char > unicode.MaxASCII || unicode.IsSpace(char) || unicode.IsControl(char) {
			return fmt.Errorf("image reference contains whitespace, control or non-ASCII characters")
		}
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case strings.ContainsRune("._-/:@+", char):
		default:
			return fmt.Errorf("image reference contains invalid character %q", char)
		}
	}
	return nil
}

func ValidateComposeName(value string) error {
	if err := validateDockerIdentifier(value, maxDockerComposeNameBytes, true); err != nil {
		return fmt.Errorf("invalid Compose name: %w", err)
	}
	if value == "." || value == ".." || strings.Contains(value, "..") {
		return fmt.Errorf("invalid Compose name traversal sequence")
	}
	return nil
}

func ValidateDockerTail(tail int) error {
	if tail < 1 || tail > 10_000 {
		return fmt.Errorf("tail must be between 1 and 10000")
	}
	return nil
}

func ValidateDockerTimeout(timeout int) error {
	if timeout < 1 || timeout > 300 {
		return fmt.Errorf("timeout must be between 1 and 300 seconds")
	}
	return nil
}

func ValidateComposeContent(content string) error {
	if len(content) > maxDockerComposeContentBytes {
		return fmt.Errorf("Compose content exceeds %d bytes", maxDockerComposeContentBytes)
	}
	return nil
}

func ValidateDockerCreate(request DockerCreateRequest) error {
	if err := ValidateContainerRef(request.Name); err != nil {
		return err
	}
	if err := ValidateImageRef(request.Image); err != nil {
		return err
	}
	if len(request.Ports) > maxDockerCreatePorts {
		return fmt.Errorf("too many port mappings")
	}
	for _, port := range request.Ports {
		if err := validateDockerTextField("port mapping", port, maxDockerPortBytes, false); err != nil {
			return err
		}
	}
	if len(request.Volumes) > maxDockerCreateVolumes {
		return fmt.Errorf("too many volume mappings")
	}
	for _, volume := range request.Volumes {
		if err := validateDockerTextField("volume mapping", volume, maxDockerVolumeBytes, false); err != nil {
			return err
		}
	}
	if len(request.Env) > maxDockerCreateEnv {
		return fmt.Errorf("too many environment variables")
	}
	for key, value := range request.Env {
		if err := validateDockerEnvKey(key); err != nil {
			return err
		}
		if err := validateDockerTextField("environment value", value, maxDockerEnvValueBytes, true); err != nil {
			return err
		}
	}
	if err := validateDockerTextField("command", request.Command, maxDockerCommandBytes, true); err != nil {
		return err
	}
	if err := validateDockerRestartPolicy(request.Restart); err != nil {
		return err
	}
	if request.Network != "" {
		if err := validateDockerIdentifier(request.Network, maxDockerContainerRefBytes, false); err != nil {
			return fmt.Errorf("invalid network: %w", err)
		}
	}
	return nil
}

func validateDockerIdentifier(value string, maxBytes int, allowDot bool) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("value is empty or padded")
	}
	if len(value) > maxBytes {
		return fmt.Errorf("value exceeds %d bytes", maxBytes)
	}
	for index, char := range value {
		if char > unicode.MaxASCII || unicode.IsSpace(char) || unicode.IsControl(char) {
			return fmt.Errorf("value contains whitespace, control or non-ASCII characters")
		}
		if index == 0 && !isDockerASCIIAlphanumeric(char) {
			return fmt.Errorf("value must start with an ASCII letter or digit")
		}
		if isDockerASCIIAlphanumeric(char) || char == '_' || char == '-' || (allowDot && char == '.') || (!allowDot && char == '.') {
			continue
		}
		return fmt.Errorf("value contains invalid character %q", char)
	}
	return nil
}

func isDockerASCIIAlphanumeric(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9')
}

func validateDockerTextField(name, value string, maxBytes int, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("%s is empty", name)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%s exceeds %d bytes", name, maxBytes)
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}

func validateDockerEnvKey(key string) error {
	if key == "" || len(key) > maxDockerEnvKeyBytes {
		return fmt.Errorf("invalid environment key length")
	}
	for index, char := range key {
		if index == 0 {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' {
				continue
			}
			return fmt.Errorf("invalid environment key")
		}
		if isDockerASCIIAlphanumeric(char) || char == '_' {
			continue
		}
		return fmt.Errorf("invalid environment key")
	}
	return nil
}

func validateDockerRestartPolicy(policy string) error {
	if policy == "" || policy == "no" || policy == "always" || policy == "unless-stopped" {
		return nil
	}
	prefix, attempts, found := strings.Cut(policy, ":")
	if !found || prefix != "on-failure" || attempts == "" {
		return fmt.Errorf("invalid restart policy")
	}
	count, err := strconv.Atoi(attempts)
	if err != nil || count < 1 || count > 100 {
		return fmt.Errorf("invalid restart retry count")
	}
	return nil
}
