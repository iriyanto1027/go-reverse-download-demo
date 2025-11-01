package models

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ClientConnection represents a connected client
type ClientConnection struct {
	ClientID      string
	Connection    *websocket.Conn
	ConnectedAt   time.Time
	LastHeartbeat time.Time
	LastActivity  time.Time
	Metadata      map[string]string
	mu            sync.RWMutex
}

// NewClientConnection creates a new client connection
func NewClientConnection(clientID string, conn *websocket.Conn) *ClientConnection {
	now := time.Now()
	return &ClientConnection{
		ClientID:      clientID,
		Connection:    conn,
		ConnectedAt:   now,
		LastHeartbeat: now,
		LastActivity:  now,
		Metadata:      make(map[string]string),
	}
}

// UpdateHeartbeat updates the last heartbeat time
func (c *ClientConnection) UpdateHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastHeartbeat = time.Now()
}

// UpdateActivity updates the last activity time
func (c *ClientConnection) UpdateActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastActivity = time.Now()
}

// GetMetadata safely retrieves metadata
func (c *ClientConnection) GetMetadata(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.Metadata[key]
	return val, ok
}

// SetMetadata safely sets metadata
func (c *ClientConnection) SetMetadata(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Metadata[key] = value
}

// IsAlive checks if the client is still alive based on heartbeat
func (c *ClientConnection) IsAlive(timeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.LastHeartbeat) < timeout
}

// UploadStatus represents the status of an ongoing upload
type UploadStatus struct {
	UploadID       string
	ClientID       string
	FilePath       string
	S3Bucket       string
	S3Key          string
	FileSize       int64
	ChunkSize      int64
	TotalParts     int
	CompletedParts int
	BytesUploaded  int64
	Status         UploadState
	StartTime      time.Time
	EndTime        *time.Time
	Error          string
	ETags          map[int]string // part number -> ETag
	mu             sync.RWMutex
}

// UploadState represents the state of an upload
type UploadState string

const (
	UploadStatePending    UploadState = "pending"
	UploadStateInProgress UploadState = "in_progress"
	UploadStateCompleted  UploadState = "completed"
	UploadStateFailed     UploadState = "failed"
	UploadStateCancelled  UploadState = "cancelled"
)

// NewUploadStatus creates a new upload status
func NewUploadStatus(uploadID, clientID, filePath, bucket, key string, fileSize, chunkSize int64, totalParts int) *UploadStatus {
	return &UploadStatus{
		UploadID:       uploadID,
		ClientID:       clientID,
		FilePath:       filePath,
		S3Bucket:       bucket,
		S3Key:          key,
		FileSize:       fileSize,
		ChunkSize:      chunkSize,
		TotalParts:     totalParts,
		CompletedParts: 0,
		BytesUploaded:  0,
		Status:         UploadStatePending,
		StartTime:      time.Now(),
		ETags:          make(map[int]string),
	}
}

// UpdateProgress updates the upload progress
func (u *UploadStatus) UpdateProgress(partNumber int, etag string, bytesUploaded int64) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if _, exists := u.ETags[partNumber]; !exists {
		u.CompletedParts++
	}
	u.ETags[partNumber] = etag
	u.BytesUploaded = bytesUploaded

	if u.Status == UploadStatePending {
		u.Status = UploadStateInProgress
	}
}

// MarkCompleted marks the upload as completed
func (u *UploadStatus) MarkCompleted() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Status = UploadStateCompleted
	now := time.Now()
	u.EndTime = &now
}

// MarkFailed marks the upload as failed
func (u *UploadStatus) MarkFailed(err string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Status = UploadStateFailed
	u.Error = err
	now := time.Now()
	u.EndTime = &now
}

// MarkCancelled marks the upload as cancelled
func (u *UploadStatus) MarkCancelled() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Status = UploadStateCancelled
	now := time.Now()
	u.EndTime = &now
}

// GetProgress returns the current progress percentage
func (u *UploadStatus) GetProgress() float64 {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if u.TotalParts == 0 {
		return 0
	}
	return float64(u.CompletedParts) / float64(u.TotalParts) * 100
}

// GetETags returns a copy of the ETags map
func (u *UploadStatus) GetETags() map[int]string {
	u.mu.RLock()
	defer u.mu.RUnlock()

	etags := make(map[int]string, len(u.ETags))
	for k, v := range u.ETags {
		etags[k] = v
	}
	return etags
}

// ClientStatus represents the overall status of a client
type ClientStatus struct {
	ClientID       string      `json:"client_id"`
	Connected      bool        `json:"connected"`
	ConnectedAt    *time.Time  `json:"connected_at,omitempty"`
	LastHeartbeat  *time.Time  `json:"last_heartbeat,omitempty"`
	LastActivity   *time.Time  `json:"last_activity,omitempty"`
	CurrentUpload  *UploadInfo `json:"current_upload,omitempty"`
	TotalUploads   int         `json:"total_uploads"`
	SuccessUploads int         `json:"success_uploads"`
	FailedUploads  int         `json:"failed_uploads"`
}

// UploadInfo contains information about an upload
type UploadInfo struct {
	UploadID       string      `json:"upload_id"`
	FilePath       string      `json:"file_path"`
	S3Key          string      `json:"s3_key"`
	FileSize       int64       `json:"file_size"`
	Status         UploadState `json:"status"`
	Progress       float64     `json:"progress"`
	CompletedParts int         `json:"completed_parts"`
	TotalParts     int         `json:"total_parts"`
	BytesUploaded  int64       `json:"bytes_uploaded"`
	StartTime      time.Time   `json:"start_time"`
	EndTime        *time.Time  `json:"end_time,omitempty"`
	Error          string      `json:"error,omitempty"`
}
