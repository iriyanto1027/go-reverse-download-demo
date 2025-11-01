package handler

import (
	"fmt"
	"log"
	"time"

	"github.com/iriyanto1027/file-download-system/client/uploader"
	"github.com/iriyanto1027/file-download-system/client/websocket"
	sharedModels "github.com/iriyanto1027/file-download-system/shared/models"
)

// CommandHandler handles commands from the server
type CommandHandler struct {
	wsClient *websocket.Client
	filePath string
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(wsClient *websocket.Client, filePath string) *CommandHandler {
	return &CommandHandler{
		wsClient: wsClient,
		filePath: filePath,
	}
}

// HandleCommand processes incoming commands from the server
func (h *CommandHandler) HandleCommand(cmd *sharedModels.CommandMessage) error {
	log.Printf("📥 Received command: %s (ID: %s)", cmd.Action, cmd.MessageID)

	switch cmd.Action {
	case sharedModels.CommandActionDownloadFile:
		return h.handleDownloadFile(cmd)

	case sharedModels.CommandActionHealthCheck:
		return h.handleHealthCheck(cmd)

	case sharedModels.CommandActionCancelUpload:
		return h.handleCancelUpload(cmd)

	default:
		log.Printf("Unknown command action: %s", cmd.Action)
		return h.sendErrorResponse(cmd.MessageID, cmd.Action, fmt.Sprintf("unknown command: %s", cmd.Action))
	}
}

// handleDownloadFile handles the download file command
func (h *CommandHandler) handleDownloadFile(cmd *sharedModels.CommandMessage) error {
	// Parse payload
	payloadMap, ok := cmd.Payload.(map[string]interface{})
	if !ok {
		return h.sendErrorResponse(cmd.MessageID, cmd.Action, "invalid payload format")
	}

	// Extract upload config
	uploadConfigMap, ok := payloadMap["upload_config"].(map[string]interface{})
	if !ok {
		return h.sendErrorResponse(cmd.MessageID, cmd.Action, "missing upload_config")
	}

	uploadConfig, err := parseUploadConfig(uploadConfigMap)
	if err != nil {
		return h.sendErrorResponse(cmd.MessageID, cmd.Action, fmt.Sprintf("failed to parse upload config: %v", err))
	}

	// Get file path (use from command or default)
	filePath := h.filePath
	if fp, ok := payloadMap["file_path"].(string); ok && fp != "" {
		filePath = fp
	}

	log.Printf("📤 Starting upload for file: %s", filePath)
	log.Printf("   Upload ID: %s", uploadConfig.UploadID)
	log.Printf("   S3 Key: %s", uploadConfig.Key)
	log.Printf("   Total parts: %d", len(uploadConfig.PresignedURLs))

	// Send in-progress response
	h.wsClient.SendResponse(
		sharedModels.ResponseStatusInProgress,
		cmd.MessageID,
		cmd.Action,
		map[string]interface{}{
			"upload_id": uploadConfig.UploadID,
			"file_path": filePath,
			"status":    "starting",
		},
		"",
	)

	// Get file size
	fileSize, err := uploader.GetFileSize(filePath)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get file size: %v", err)
		log.Printf("❌ %s", errMsg)
		return h.sendErrorResponse(cmd.MessageID, cmd.Action, errMsg)
	}

	log.Printf("📦 File size: %.2f MB", float64(fileSize)/(1024*1024))

	// Create uploader
	up := uploader.NewUploader(filePath, uploadConfig)

	// Upload with progress callback
	result, err := up.Upload(func(partNumber, totalParts int, bytesUploaded, totalBytes int64) {
		progress := float64(bytesUploaded) / float64(totalBytes) * 100

		// Send status update
		h.wsClient.SendStatus("uploading", &sharedModels.UploadStatus{
			UploadID:       uploadConfig.UploadID,
			FilePath:       filePath,
			FileSize:       totalBytes,
			TotalParts:     totalParts,
			CompletedParts: partNumber,
			BytesUploaded:  bytesUploaded,
			Progress:       progress,
			StartTime:      time.Now().Add(-time.Second * time.Duration(partNumber*2)), // Approximate
			LastUpdate:     time.Now(),
		}, nil)

		log.Printf("📊 Progress: %.1f%% (%d/%d parts)", progress, partNumber, totalParts)
	})

	if err != nil {
		errMsg := fmt.Sprintf("upload failed: %v", err)
		log.Printf("❌ %s", errMsg)
		return h.sendErrorResponse(cmd.MessageID, cmd.Action, errMsg)
	}

	// Send success response
	log.Printf("✅ Upload completed successfully")
	return h.wsClient.SendResponse(
		sharedModels.ResponseStatusSuccess,
		cmd.MessageID,
		cmd.Action,
		sharedModels.DownloadFileResponse{
			UploadID:       result.UploadID,
			FilePath:       filePath,
			FileSize:       result.FileSize,
			TotalParts:     result.TotalParts,
			CompletedParts: result.CompletedParts,
			StartTime:      time.Now().Add(-result.Duration),
			EndTime:        time.Now(),
			S3Key:          uploadConfig.Key,
		},
		"",
	)
}

// handleHealthCheck handles the health check command
func (h *CommandHandler) handleHealthCheck(cmd *sharedModels.CommandMessage) error {
	log.Printf("💓 Health check requested")

	return h.wsClient.SendResponse(
		sharedModels.ResponseStatusSuccess,
		cmd.MessageID,
		cmd.Action,
		map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now(),
		},
		"",
	)
}

// handleCancelUpload handles the cancel upload command
func (h *CommandHandler) handleCancelUpload(cmd *sharedModels.CommandMessage) error {
	log.Printf("🛑 Cancel upload requested")

	// TODO: Implement upload cancellation logic

	return h.wsClient.SendResponse(
		sharedModels.ResponseStatusCancelled,
		cmd.MessageID,
		cmd.Action,
		map[string]interface{}{
			"status": "cancelled",
		},
		"",
	)
}

// sendErrorResponse sends an error response
func (h *CommandHandler) sendErrorResponse(messageID string, action sharedModels.CommandAction, errMsg string) error {
	return h.wsClient.SendResponse(
		sharedModels.ResponseStatusError,
		messageID,
		action,
		nil,
		errMsg,
	)
}

// parseUploadConfig parses the upload config from a map
func parseUploadConfig(m map[string]interface{}) (sharedModels.UploadConfig, error) {
	config := sharedModels.UploadConfig{}

	if uploadID, ok := m["upload_id"].(string); ok {
		config.UploadID = uploadID
	}

	if bucket, ok := m["bucket"].(string); ok {
		config.Bucket = bucket
	}

	if key, ok := m["key"].(string); ok {
		config.Key = key
	}

	if region, ok := m["region"].(string); ok {
		config.Region = region
	}

	if chunkSize, ok := m["chunk_size"].(float64); ok {
		config.ChunkSize = int64(chunkSize)
	}

	// Parse presigned URLs
	if presignedURLs, ok := m["presigned_urls"].([]interface{}); ok {
		config.PresignedURLs = make([]sharedModels.PresignedURL, len(presignedURLs))
		for i, urlInterface := range presignedURLs {
			if urlMap, ok := urlInterface.(map[string]interface{}); ok {
				if partNumber, ok := urlMap["part_number"].(float64); ok {
					config.PresignedURLs[i].PartNumber = int(partNumber)
				}
				if url, ok := urlMap["url"].(string); ok {
					config.PresignedURLs[i].URL = url
				}
			}
		}
	}

	if config.UploadID == "" || len(config.PresignedURLs) == 0 {
		return config, fmt.Errorf("invalid upload config: missing required fields")
	}

	return config, nil
}
