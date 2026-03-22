package controllers

import "github.com/user/server-ops-backend/services"

func init() {
	services.InitUploadService(services.AgentSender{
		SendHostFile:      uploadFileViaWebSocket,
		SendContainerFile: uploadContainerFileViaWebSocket,
		ValidateFilePath:  isValidFilePath,
	})
}
