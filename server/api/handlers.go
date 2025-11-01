package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"sort"
	"time"

	"github.com/iriyanto1027/file-download-system/server/models"
	"github.com/iriyanto1027/file-download-system/server/s3"
	"github.com/iriyanto1027/file-download-system/server/websocket"
	sharedModels "github.com/iriyanto1027/file-download-system/shared/models"
)

// Handler handles API requests
type Handler struct {
	wsManager  *websocket.Manager
	s3Client   *s3.Client
	chunkSize  int64
	baseS3Path string
}

// Config contains the API handler configuration
type Config struct {
	ChunkSize  int64
	BaseS3Path string
}

// NewHandler creates a new API handler
func NewHandler(wsManager *websocket.Manager, s3Client *s3.Client, cfg Config) *Handler {
	if cfg.ChunkSize == 0 {
		cfg.ChunkSize = 5 * 1024 * 1024 // 5MB default
	}
	if cfg.BaseS3Path == "" {
		cfg.BaseS3Path = "uploads"
	}

	return &Handler{
		wsManager:  wsManager,
		s3Client:   s3Client,
		chunkSize:  cfg.ChunkSize,
		baseS3Path: cfg.BaseS3Path,
	}
}

// TriggerDownloadRequest is the request body for triggering a download
type TriggerDownloadRequest struct {
	FilePath string            `json:"file_path,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// TriggerDownloadResponse is the response for triggering a download
type TriggerDownloadResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	UploadID string `json:"upload_id,omitempty"`
	S3Key    string `json:"s3_key,omitempty"`
}

// ErrorResponse is the standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// TriggerDownload handles POST /trigger-download/{client_id}
func (h *Handler) TriggerDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract client ID from URL path
	clientID := path.Base(r.URL.Path)
	if clientID == "" || clientID == "trigger-download" {
		h.sendError(w, http.StatusBadRequest, "Client ID is required")
		return
	}

	// Check if client is connected
	if !h.wsManager.IsClientConnected(clientID) {
		h.sendError(w, http.StatusNotFound, fmt.Sprintf("Client %s is not connected", clientID))
		return
	}

	// Parse request body (optional)
	var req TriggerDownloadRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Ignore parse errors, use defaults
			log.Printf("Warning: Failed to parse request body: %v", err)
		}
	}

	// Set default file path if not provided
	if req.FilePath == "" {
		req.FilePath = "/data/test-file.bin"
	}

	// Generate upload ID
	uploadID, err := generateUploadID()
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "Failed to generate upload ID")
		return
	}

	// Generate S3 key
	timestamp := time.Now().Format("20060102-150405")
	s3Key := fmt.Sprintf("%s/%s/%s-%s", h.baseS3Path, clientID, timestamp, path.Base(req.FilePath))

	// Assume file size for multipart upload (will be adjusted by client)
	// We'll create presigned URLs for a 100MB file by default
	estimatedFileSize := int64(100 * 1024 * 1024) // 100MB

	// Initiate multipart upload and generate presigned URLs
	multipartUpload, err := h.s3Client.InitiateMultipartUpload(r.Context(), s3.MultipartUploadConfig{
		Key:       s3Key,
		FileSize:  estimatedFileSize,
		ChunkSize: h.chunkSize,
		Metadata:  req.Metadata,
	})

	if err != nil {
		log.Printf("Failed to initiate multipart upload: %v", err)
		h.sendError(w, http.StatusInternalServerError, "Failed to initiate upload")
		return
	}

	// Create upload status
	uploadStatus := models.NewUploadStatus(
		uploadID,
		clientID,
		req.FilePath,
		multipartUpload.Bucket,
		multipartUpload.Key,
		estimatedFileSize,
		h.chunkSize,
		multipartUpload.TotalParts,
	)
	// Set the S3 multipart upload ID
	uploadStatus.SetS3UploadID(multipartUpload.UploadID)
	h.wsManager.RegisterUpload(uploadStatus)

	// Prepare download command
	presignedURLs := make([]sharedModels.PresignedURL, len(multipartUpload.PresignedURLs))
	for i, url := range multipartUpload.PresignedURLs {
		presignedURLs[i] = sharedModels.PresignedURL{
			PartNumber: url.PartNumber,
			URL:        url.URL,
		}
	}

	command := &sharedModels.CommandMessage{
		Action: sharedModels.CommandActionDownloadFile,
		Payload: sharedModels.DownloadFilePayload{
			FilePath: req.FilePath,
			UploadConfig: sharedModels.UploadConfig{
				UploadID:      uploadID,
				Bucket:        multipartUpload.Bucket,
				Key:           multipartUpload.Key,
				ChunkSize:     h.chunkSize,
				PresignedURLs: presignedURLs,
			},
			Metadata: req.Metadata,
		},
	}
	command.MessageID = uploadID

	// Send command to client
	if err := h.wsManager.SendCommand(clientID, command); err != nil {
		log.Printf("Failed to send command to client %s: %v", clientID, err)
		h.sendError(w, http.StatusInternalServerError, "Failed to send command to client")
		return
	}

	// Send success response
	h.sendJSON(w, http.StatusOK, TriggerDownloadResponse{
		Success:  true,
		Message:  fmt.Sprintf("Download triggered for client %s", clientID),
		UploadID: uploadID,
		S3Key:    s3Key,
	})
}

// GetStatus handles GET /status/{client_id}
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract client ID from URL path
	clientID := path.Base(r.URL.Path)
	if clientID == "" || clientID == "status" {
		h.sendError(w, http.StatusBadRequest, "Client ID is required")
		return
	}

	// Get client status
	status := h.wsManager.GetClientStatus(clientID)

	h.sendJSON(w, http.StatusOK, status)
}

// GetUploadStatus handles GET /uploads/{upload_id}
func (h *Handler) GetUploadStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract upload ID from URL path
	uploadID := path.Base(r.URL.Path)
	if uploadID == "" || uploadID == "uploads" {
		h.sendError(w, http.StatusBadRequest, "Upload ID is required")
		return
	}

	// Get upload status
	upload, exists := h.wsManager.GetUpload(uploadID)
	if !exists {
		h.sendError(w, http.StatusNotFound, fmt.Sprintf("Upload %s not found", uploadID))
		return
	}

	// Convert to response format
	response := models.UploadInfo{
		UploadID:       upload.UploadID,
		FilePath:       upload.FilePath,
		S3Key:          upload.S3Key,
		FileSize:       upload.FileSize,
		Status:         upload.Status,
		Progress:       upload.GetProgress(),
		CompletedParts: upload.CompletedParts,
		TotalParts:     upload.TotalParts,
		BytesUploaded:  upload.BytesUploaded,
		StartTime:      upload.StartTime,
		EndTime:        upload.EndTime,
		Error:          upload.Error,
	}

	h.sendJSON(w, http.StatusOK, response)
}

// ListClients handles GET /clients
func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientIDs := h.wsManager.GetAllClients()

	response := struct {
		Clients []string `json:"clients"`
		Count   int      `json:"count"`
	}{
		Clients: clientIDs,
		Count:   len(clientIDs),
	}

	h.sendJSON(w, http.StatusOK, response)
}

// HealthCheck handles GET /health
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	response := struct {
		Status  string    `json:"status"`
		Time    time.Time `json:"time"`
		Clients int       `json:"clients"`
	}{
		Status:  "healthy",
		Time:    time.Now(),
		Clients: len(h.wsManager.GetAllClients()),
	}

	h.sendJSON(w, http.StatusOK, response)
}

// HandleResponse implements websocket.MessageHandler
func (h *Handler) HandleResponse(clientID string, msg *sharedModels.ResponseMessage) error {
	log.Printf("Received response from client %s: status=%s, action=%s", clientID, msg.Status, msg.Action)

	// Handle download file response
	if msg.Action == sharedModels.CommandActionDownloadFile {
		if payload, ok := msg.Payload.(map[string]interface{}); ok {
			uploadID, _ := payload["upload_id"].(string)

			if upload, exists := h.wsManager.GetUpload(uploadID); exists {
				switch msg.Status {
				case sharedModels.ResponseStatusSuccess:
					upload.MarkCompleted()
					log.Printf("Upload %s completed successfully", uploadID)

					// Extract ETags from payload
					var etags map[int]string
					if etagsRaw, ok := payload["etags"].(map[string]interface{}); ok {
						etags = make(map[int]string)
						for k, v := range etagsRaw {
							// Convert string key to int
							var partNum int
							if _, err := fmt.Sscanf(k, "%d", &partNum); err == nil {
								if etag, ok := v.(string); ok {
									etags[partNum] = etag
								}
							}
						}
					}

					// Complete the multipart upload on S3
					if len(etags) > 0 {
						parts := make([]s3.CompletedPart, 0, len(etags))
						for partNum, etag := range etags {
							parts = append(parts, s3.CompletedPart{
								PartNumber: partNum,
								ETag:       etag,
							})
						}

						// CRITICAL: Sort parts by PartNumber to ensure correct order for S3
						sort.Slice(parts, func(i, j int) bool {
							return parts[i].PartNumber < parts[j].PartNumber
						})

						log.Printf("Completing multipart upload %s with %d parts", uploadID, len(parts))

						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()

						// Use S3UploadID from upload status, not internal uploadID
						s3UploadID := upload.GetS3UploadID()
						if err := h.s3Client.CompleteMultipartUpload(ctx, upload.S3Key, s3UploadID, parts); err != nil {
							log.Printf("❌ Failed to complete multipart upload: %v", err)
							upload.MarkFailed(err.Error())
						} else {
							log.Printf("✅ Multipart upload %s completed on S3", s3UploadID)
						}
					} else {
						log.Printf("⚠️ No ETags found in payload, cannot complete multipart upload")
					}

				case sharedModels.ResponseStatusError:
					upload.MarkFailed(msg.Error)
					log.Printf("Upload %s failed: %s", uploadID, msg.Error)

					// Abort the multipart upload
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					h.s3Client.AbortMultipartUpload(ctx, upload.S3Key, upload.GetS3UploadID())

				case sharedModels.ResponseStatusCancelled:
					upload.MarkCancelled()
					log.Printf("Upload %s cancelled", uploadID)

					// Abort the multipart upload
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					h.s3Client.AbortMultipartUpload(ctx, upload.S3Key, upload.GetS3UploadID())
				}
			}
		}
	}

	return nil
}

// HandleStatus implements websocket.MessageHandler
func (h *Handler) HandleStatus(clientID string, msg *sharedModels.StatusMessage) error {
	log.Printf("Received status from client %s: %s", clientID, msg.Status)

	// Update upload progress if available
	if msg.CurrentUpload != nil {
		if upload, exists := h.wsManager.GetUpload(msg.CurrentUpload.UploadID); exists {
			// Update progress based on completed parts
			// Note: The client will send ETags separately in response messages
			upload.BytesUploaded = msg.CurrentUpload.BytesUploaded
			upload.CompletedParts = msg.CurrentUpload.CompletedParts
		}
	}

	return nil
}

// sendJSON sends a JSON response
func (h *Handler) sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// sendError sends an error response
func (h *Handler) sendError(w http.ResponseWriter, status int, message string) {
	h.sendJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

// generateUploadID generates a random upload ID
func generateUploadID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
