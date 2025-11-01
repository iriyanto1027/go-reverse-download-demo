package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	sharedModels "github.com/iriyanto1027/file-download-system/shared/models"
)

// Client represents a WebSocket client
type Client struct {
	clientID       string
	serverURL      string
	token          string
	conn           *websocket.Conn
	mu             sync.RWMutex
	reconnectDelay time.Duration
	maxReconnect   time.Duration
	messageHandler MessageHandler
	connected      bool
	stopChan       chan struct{}
	doneChan       chan struct{}
}

// MessageHandler handles incoming messages from the server
type MessageHandler interface {
	HandleCommand(cmd *sharedModels.CommandMessage) error
}

// Config contains the client configuration
type Config struct {
	ClientID       string
	ServerURL      string
	Token          string
	ReconnectDelay time.Duration
	MaxReconnect   time.Duration
}

// NewClient creates a new WebSocket client
func NewClient(cfg Config, handler MessageHandler) *Client {
	if cfg.ReconnectDelay == 0 {
		cfg.ReconnectDelay = 5 * time.Second
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 5 * time.Minute
	}

	return &Client{
		clientID:       cfg.ClientID,
		serverURL:      cfg.ServerURL,
		token:          cfg.Token,
		reconnectDelay: cfg.ReconnectDelay,
		maxReconnect:   cfg.MaxReconnect,
		messageHandler: handler,
		connected:      false,
		stopChan:       make(chan struct{}),
		doneChan:       make(chan struct{}),
	}
}

// Connect establishes a connection to the server
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// Parse and build URL with query parameters
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	q := u.Query()
	q.Set("client_id", c.clientID)
	if c.token != "" {
		q.Set("token", c.token)
	}
	u.RawQuery = q.Encode()

	// Connect to WebSocket
	log.Printf("Connecting to %s", u.String())
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	log.Printf("✅ Connected to server as client: %s", c.clientID)

	// Start read and write routines
	go c.readPump(ctx)
	go c.writePump(ctx)

	return nil
}

// Start starts the client with auto-reconnect
func (c *Client) Start(ctx context.Context) error {
	defer close(c.doneChan)

	reconnectDelay := c.reconnectDelay

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping client")
			return ctx.Err()
		case <-c.stopChan:
			log.Println("Stop signal received, stopping client")
			return nil
		default:
			if err := c.Connect(ctx); err != nil {
				log.Printf("Connection failed: %v. Retrying in %v...", err, reconnectDelay)

				select {
				case <-time.After(reconnectDelay):
					// Exponential backoff
					reconnectDelay *= 2
					if reconnectDelay > c.maxReconnect {
						reconnectDelay = c.maxReconnect
					}
					continue
				case <-ctx.Done():
					return ctx.Err()
				case <-c.stopChan:
					return nil
				}
			}

			// Reset reconnect delay on successful connection
			reconnectDelay = c.reconnectDelay

			// Wait for disconnection
			<-c.doneChan
			c.doneChan = make(chan struct{})

			// Check if we should stop
			select {
			case <-c.stopChan:
				return nil
			default:
				log.Printf("Disconnected. Reconnecting in %v...", reconnectDelay)
				time.Sleep(reconnectDelay)
			}
		}
	}
}

// Stop stops the client
func (c *Client) Stop() {
	close(c.stopChan)
	c.disconnect()
}

// disconnect closes the connection
func (c *Client) disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connected = false
}

// readPump reads messages from the server
func (c *Client) readPump(ctx context.Context) {
	defer func() {
		c.disconnect()
		close(c.doneChan)
	}()

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return
	}

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}

			if err := c.handleMessage(message); err != nil {
				log.Printf("Error handling message: %v", err)
			}
		}
	}
}

// writePump sends periodic ping messages
func (c *Client) writePump(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.doneChan:
			return
		case <-ticker.C:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Failed to send ping: %v", err)
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *Client) handleMessage(message []byte) error {
	// Parse base message to determine type
	var baseMsg sharedModels.WebSocketMessage
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	switch baseMsg.Type {
	case sharedModels.MessageTypeCommand:
		var cmdMsg sharedModels.CommandMessage
		if err := json.Unmarshal(message, &cmdMsg); err != nil {
			return fmt.Errorf("failed to parse command message: %w", err)
		}
		if c.messageHandler != nil {
			return c.messageHandler.HandleCommand(&cmdMsg)
		}

	case sharedModels.MessageTypePing:
		// Respond with pong
		return c.SendPong()

	default:
		log.Printf("Received unknown message type: %s", baseMsg.Type)
	}

	return nil
}

// SendResponse sends a response message to the server
func (c *Client) SendResponse(status sharedModels.ResponseStatus, commandID string, action sharedModels.CommandAction, payload interface{}, errMsg string) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	response := &sharedModels.ResponseMessage{
		WebSocketMessage: sharedModels.WebSocketMessage{
			Type:      sharedModels.MessageTypeResponse,
			Timestamp: time.Now(),
			MessageID: commandID,
		},
		Status:    status,
		CommandID: commandID,
		Action:    action,
		Payload:   payload,
		Error:     errMsg,
	}

	return conn.WriteJSON(response)
}

// SendStatus sends a status message to the server
func (c *Client) SendStatus(status string, currentUpload *sharedModels.UploadStatus, systemInfo *sharedModels.SystemInfo) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	statusMsg := &sharedModels.StatusMessage{
		WebSocketMessage: sharedModels.WebSocketMessage{
			Type:      sharedModels.MessageTypeStatus,
			Timestamp: time.Now(),
		},
		ClientID:      c.clientID,
		Status:        status,
		CurrentUpload: currentUpload,
		SystemInfo:    systemInfo,
	}

	return conn.WriteJSON(statusMsg)
}

// SendPong sends a pong message
func (c *Client) SendPong() error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	pong := &sharedModels.PongMessage{
		WebSocketMessage: sharedModels.WebSocketMessage{
			Type:      sharedModels.MessageTypePong,
			Timestamp: time.Now(),
		},
	}

	return conn.WriteJSON(pong)
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}
