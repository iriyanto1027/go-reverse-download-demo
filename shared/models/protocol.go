package models

import "time"

// MessageType defines the type of WebSocket message
type MessageType string

const (
	MessageTypeCommand  MessageType = "command"
	MessageTypeResponse MessageType = "response"
	MessageTypeStatus   MessageType = "status"
	MessageTypePing     MessageType = "ping"
	MessageTypePong     MessageType = "pong"
)

// CommandAction defines the action to be performed by the client
type CommandAction string

const (
	CommandActionDownloadFile CommandAction = "download_file"
	CommandActionCancelUpload CommandAction = "cancel_upload"
	CommandActionHealthCheck  CommandAction = "health_check"
)

// ResponseStatus defines the status of a command execution
type ResponseStatus string

const (
	ResponseStatusSuccess    ResponseStatus = "success"
	ResponseStatusError      ResponseStatus = "error"
	ResponseStatusInProgress ResponseStatus = "in_progress"
	ResponseStatusCancelled  ResponseStatus = "cancelled"
)

// WebSocketMessage is the base message structure for WebSocket communication
type WebSocketMessage struct {
	Type      MessageType `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	MessageID string      `json:"message_id,omitempty"`
}

// CommandMessage is sent from server to client
type CommandMessage struct {
	WebSocketMessage
	Action  CommandAction `json:"action"`
	Payload interface{}   `json:"payload"`
}

// DownloadFilePayload contains the details for a download command
type DownloadFilePayload struct {
	FilePath     string            `json:"file_path"`
	UploadConfig UploadConfig      `json:"upload_config"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// UploadConfig contains S3 upload configuration
type UploadConfig struct {
	UploadID      string         `json:"upload_id"`
	Bucket        string         `json:"bucket"`
	Key           string         `json:"key"`
	Region        string         `json:"region"`
	ChunkSize     int64          `json:"chunk_size"`
	PresignedURLs []PresignedURL `json:"presigned_urls"`
}

// PresignedURL contains a presigned URL for uploading a specific part
type PresignedURL struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

// ResponseMessage is sent from client to server
type ResponseMessage struct {
	WebSocketMessage
	Status    ResponseStatus `json:"status"`
	CommandID string         `json:"command_id,omitempty"`
	Action    CommandAction  `json:"action,omitempty"`
	Payload   interface{}    `json:"payload,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// DownloadFileResponse is the payload for a download file response
type DownloadFileResponse struct {
	UploadID       string    `json:"upload_id"`
	FilePath       string    `json:"file_path"`
	FileSize       int64     `json:"file_size"`
	TotalParts     int       `json:"total_parts"`
	CompletedParts int       `json:"completed_parts"`
	StartTime      time.Time `json:"start_time,omitempty"`
	EndTime        time.Time `json:"end_time,omitempty"`
	S3Key          string    `json:"s3_key,omitempty"`
}

// StatusMessage is sent periodically from client to server for progress updates
type StatusMessage struct {
	WebSocketMessage
	ClientID      string        `json:"client_id"`
	Status        string        `json:"status"`
	CurrentUpload *UploadStatus `json:"current_upload,omitempty"`
	SystemInfo    *SystemInfo   `json:"system_info,omitempty"`
}

// UploadStatus contains the current upload progress
type UploadStatus struct {
	UploadID       string    `json:"upload_id"`
	FilePath       string    `json:"file_path"`
	FileSize       int64     `json:"file_size"`
	TotalParts     int       `json:"total_parts"`
	CompletedParts int       `json:"completed_parts"`
	BytesUploaded  int64     `json:"bytes_uploaded"`
	Progress       float64   `json:"progress"`
	StartTime      time.Time `json:"start_time"`
	LastUpdate     time.Time `json:"last_update"`
}

// SystemInfo contains system information from the client
type SystemInfo struct {
	Hostname     string  `json:"hostname,omitempty"`
	OS           string  `json:"os,omitempty"`
	Architecture string  `json:"architecture,omitempty"`
	CPUUsage     float64 `json:"cpu_usage,omitempty"`
	MemoryUsage  float64 `json:"memory_usage,omitempty"`
	DiskUsage    float64 `json:"disk_usage,omitempty"`
}

// PingMessage is sent to keep the connection alive
type PingMessage struct {
	WebSocketMessage
}

// PongMessage is the response to a ping
type PongMessage struct {
	WebSocketMessage
}
