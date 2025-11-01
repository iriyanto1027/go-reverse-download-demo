package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iriyanto1027/file-download-system/server/models"
	sharedModels "github.com/iriyanto1027/file-download-system/shared/models"
)

// Manager manages WebSocket connections
type Manager struct {
	clients        map[string]*models.ClientConnection
	uploads        map[string]*models.UploadStatus
	mu             sync.RWMutex
	upgrader       websocket.Upgrader
	pingInterval   time.Duration
	clientTimeout  time.Duration
	messageHandler MessageHandler
}

// MessageHandler handles incoming WebSocket messages
type MessageHandler interface {
	HandleResponse(clientID string, msg *sharedModels.ResponseMessage) error
	HandleStatus(clientID string, msg *sharedModels.StatusMessage) error
}

// Config contains the manager configuration
type Config struct {
	PingInterval  time.Duration
	ClientTimeout time.Duration
	ReadLimit     int64
}

// NewManager creates a new WebSocket manager
func NewManager(cfg Config, handler MessageHandler) *Manager {
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 30 * time.Second
	}
	if cfg.ClientTimeout == 0 {
		cfg.ClientTimeout = 90 * time.Second
	}
	if cfg.ReadLimit == 0 {
		cfg.ReadLimit = 1024 * 1024 // 1MB
	}

	return &Manager{
		clients: make(map[string]*models.ClientConnection),
		uploads: make(map[string]*models.UploadStatus),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Implement proper origin checking in production
				return true
			},
		},
		pingInterval:   cfg.PingInterval,
		clientTimeout:  cfg.ClientTimeout,
		messageHandler: handler,
	}
}

// SetMessageHandler sets the message handler
func (m *Manager) SetMessageHandler(handler MessageHandler) {
	m.messageHandler = handler
}

// RegisterClient registers a new client connection
func (m *Manager) RegisterClient(clientID string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing connection if any
	if existingConn, exists := m.clients[clientID]; exists {
		existingConn.Connection.Close()
		log.Printf("Closed existing connection for client %s", clientID)
	}

	client := models.NewClientConnection(clientID, conn)
	m.clients[clientID] = client

	log.Printf("Client registered: %s", clientID)
}

// UnregisterClient removes a client connection
func (m *Manager) UnregisterClient(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if client, exists := m.clients[clientID]; exists {
		client.Connection.Close()
		delete(m.clients, clientID)
		log.Printf("Client unregistered: %s", clientID)
	}
}

// GetClient retrieves a client connection
func (m *Manager) GetClient(clientID string) (*models.ClientConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, exists := m.clients[clientID]
	return client, exists
}

// GetAllClients returns a list of all connected clients
func (m *Manager) GetAllClients() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clientIDs := make([]string, 0, len(m.clients))
	for clientID := range m.clients {
		clientIDs = append(clientIDs, clientID)
	}
	return clientIDs
}

// IsClientConnected checks if a client is connected
func (m *Manager) IsClientConnected(clientID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.clients[clientID]
	return exists
}

// SendCommand sends a command to a specific client
func (m *Manager) SendCommand(clientID string, cmd *sharedModels.CommandMessage) error {
	client, exists := m.GetClient(clientID)
	if !exists {
		return fmt.Errorf("client %s not connected", clientID)
	}

	// Set timestamp and message type
	cmd.Timestamp = time.Now()
	cmd.Type = sharedModels.MessageTypeCommand

	// Send message
	if err := client.Connection.WriteJSON(cmd); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	client.UpdateActivity()
	return nil
}

// SendPing sends a ping message to a client
func (m *Manager) SendPing(clientID string) error {
	client, exists := m.GetClient(clientID)
	if !exists {
		return fmt.Errorf("client %s not connected", clientID)
	}

	ping := &sharedModels.PingMessage{
		WebSocketMessage: sharedModels.WebSocketMessage{
			Type:      sharedModels.MessageTypePing,
			Timestamp: time.Now(),
		},
	}

	if err := client.Connection.WriteJSON(ping); err != nil {
		return fmt.Errorf("failed to send ping: %w", err)
	}

	return nil
}

// HandleClient handles a client WebSocket connection
func (m *Manager) HandleClient(ctx context.Context, clientID string, conn *websocket.Conn) {
	m.RegisterClient(clientID, conn)
	defer m.UnregisterClient(clientID)

	// Set read limit and deadline
	conn.SetReadLimit(1024 * 1024) // 1MB
	conn.SetReadDeadline(time.Now().Add(m.clientTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(m.clientTimeout))
		if client, exists := m.GetClient(clientID); exists {
			client.UpdateHeartbeat()
		}
		return nil
	})

	// Start ping routine
	pingDone := make(chan struct{})
	go m.pingRoutine(ctx, clientID, pingDone)
	defer close(pingDone)

	// Read messages
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error for client %s: %v", clientID, err)
				}
				return
			}

			if err := m.handleMessage(clientID, message); err != nil {
				log.Printf("Error handling message from client %s: %v", clientID, err)
			}
		}
	}
}

// handleMessage processes incoming messages from clients
func (m *Manager) handleMessage(clientID string, message []byte) error {
	// Parse base message to determine type
	var baseMsg sharedModels.WebSocketMessage
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Update client activity
	if client, exists := m.GetClient(clientID); exists {
		client.UpdateActivity()
	}

	// Handle based on type
	switch baseMsg.Type {
	case sharedModels.MessageTypeResponse:
		var responseMsg sharedModels.ResponseMessage
		if err := json.Unmarshal(message, &responseMsg); err != nil {
			return fmt.Errorf("failed to parse response message: %w", err)
		}
		if m.messageHandler != nil {
			return m.messageHandler.HandleResponse(clientID, &responseMsg)
		}

	case sharedModels.MessageTypeStatus:
		var statusMsg sharedModels.StatusMessage
		if err := json.Unmarshal(message, &statusMsg); err != nil {
			return fmt.Errorf("failed to parse status message: %w", err)
		}
		if m.messageHandler != nil {
			return m.messageHandler.HandleStatus(clientID, &statusMsg)
		}

	case sharedModels.MessageTypePong:
		// Pong received, already handled by SetPongHandler
		return nil

	default:
		log.Printf("Unknown message type from client %s: %s", clientID, baseMsg.Type)
	}

	return nil
}

// pingRoutine sends periodic ping messages to keep the connection alive
func (m *Manager) pingRoutine(ctx context.Context, clientID string, done chan struct{}) {
	ticker := time.NewTicker(m.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			if err := m.SendPing(clientID); err != nil {
				log.Printf("Failed to send ping to client %s: %v", clientID, err)
				return
			}
		}
	}
}

// RegisterUpload registers a new upload
func (m *Manager) RegisterUpload(upload *models.UploadStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploads[upload.UploadID] = upload
	log.Printf("Upload registered: %s for client %s", upload.UploadID, upload.ClientID)
}

// GetUpload retrieves an upload status
func (m *Manager) GetUpload(uploadID string) (*models.UploadStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	upload, exists := m.uploads[uploadID]
	return upload, exists
}

// GetClientUploads returns all uploads for a specific client
func (m *Manager) GetClientUploads(clientID string) []*models.UploadStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	uploads := make([]*models.UploadStatus, 0)
	for _, upload := range m.uploads {
		if upload.ClientID == clientID {
			uploads = append(uploads, upload)
		}
	}
	return uploads
}

// GetClientStatus returns the status of a client
func (m *Manager) GetClientStatus(clientID string) *models.ClientStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := &models.ClientStatus{
		ClientID:  clientID,
		Connected: false,
	}

	// Check connection
	if client, exists := m.clients[clientID]; exists {
		status.Connected = true
		status.ConnectedAt = &client.ConnectedAt
		status.LastHeartbeat = &client.LastHeartbeat
		status.LastActivity = &client.LastActivity
	}

	// Count uploads
	var currentUpload *models.UploadStatus
	for _, upload := range m.uploads {
		if upload.ClientID != clientID {
			continue
		}

		status.TotalUploads++
		switch upload.Status {
		case models.UploadStateCompleted:
			status.SuccessUploads++
		case models.UploadStateFailed:
			status.FailedUploads++
		case models.UploadStateInProgress:
			currentUpload = upload
		}
	}

	// Set current upload info
	if currentUpload != nil {
		status.CurrentUpload = &models.UploadInfo{
			UploadID:       currentUpload.UploadID,
			FilePath:       currentUpload.FilePath,
			S3Key:          currentUpload.S3Key,
			FileSize:       currentUpload.FileSize,
			Status:         currentUpload.Status,
			Progress:       currentUpload.GetProgress(),
			CompletedParts: currentUpload.CompletedParts,
			TotalParts:     currentUpload.TotalParts,
			BytesUploaded:  currentUpload.BytesUploaded,
			StartTime:      currentUpload.StartTime,
			EndTime:        currentUpload.EndTime,
			Error:          currentUpload.Error,
		}
	}

	return status
}
