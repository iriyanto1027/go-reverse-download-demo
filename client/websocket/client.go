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
	writeMu        sync.Mutex // Protects concurrent writes
	handlersWg     sync.WaitGroup // Tracks active message handlers
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
	log.Printf("Connect: conn set, conn=%v, connected=%v, client=%p", c.conn != nil, c.connected, c)
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

// SetMessageHandler sets the message handler for the client
func (c *Client) SetMessageHandler(handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageHandler = handler
}

// disconnect closes the connection
func (c *Client) disconnect() {
	log.Printf("disconnect: called, waiting for active handlers...")
	// Wait for all active message handlers to complete
	c.handlersWg.Wait()
	log.Printf("disconnect: all handlers done, proceeding with disconnect")
	
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		log.Printf("disconnect: closing connection")
		c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	log.Printf("disconnect: done, connected=false")
}

// readPump reads messages from the server
func (c *Client) readPump(ctx context.Context) {
	log.Printf("readPump: started")
	defer func() {
		log.Printf("readPump: exiting and disconnecting")
		c.disconnect()
		close(c.doneChan)
	}()

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		log.Printf("readPump: connection is nil at start, exiting")
		return
	}

	log.Printf("readPump: setting read deadline and pong handler")

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			log.Printf("readPump: context done")
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				} else {
					log.Printf("readPump: read error: %v", err)
				}
				return
			}

			// Handle message in a separate goroutine to avoid blocking readPump
			// This prevents connection timeout when processing long-running tasks
			c.handlersWg.Add(1)
			go func(msg []byte) {
				defer c.handlersWg.Done()
				if err := c.handleMessage(msg); err != nil {
					log.Printf("Error handling message: %v", err)
				}
			}(message)
		}
	}
}

// writePump sends periodic ping messages
func (c *Client) writePump(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Printf("writePump: started")
	defer log.Printf("writePump: exiting")

	for {
		select {
		case <-ctx.Done():
			log.Printf("writePump: context done")
			return
		case <-c.doneChan:
			log.Printf("writePump: done channel closed")
			return
		case <-ticker.C:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				log.Printf("writePump: connection is nil, exiting")
				return
			}

			log.Printf("📡 Sending ping to server")
			c.writeMu.Lock()
			err := conn.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()

			if err != nil {
				log.Printf("Failed to send ping: %v", err)
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *Client) handleMessage(message []byte) error {
	c.mu.RLock()
	connStatus := c.conn != nil
	connectedStatus := c.connected
	c.mu.RUnlock()
	log.Printf("handleMessage: start, conn=%v, connected=%v, client=%p", connStatus, connectedStatus, c)
	
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
	connected := c.connected
	c.mu.RUnlock()

	log.Printf("📤 SendResponse: conn=%v, connected=%v, client=%p", conn != nil, connected, c)
	
	if conn == nil {
		log.Printf("❌ SendResponse failed: connection is nil")
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

	log.Printf("📤 Sending response: status=%s, action=%s", status, action)
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	err := conn.WriteJSON(response)
	if err != nil {
		log.Printf("❌ Failed to send response: %v", err)
	} else {
		log.Printf("✅ Response sent successfully")
	}
	return err
}

// SendStatus sends a status message to the server
func (c *Client) SendStatus(status string, currentUpload *sharedModels.UploadStatus, systemInfo *sharedModels.SystemInfo) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		log.Printf("❌ SendStatus failed: connection is nil")
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

	log.Printf("📤 Sending status: %s", status)
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	err := conn.WriteJSON(statusMsg)
	if err != nil {
		log.Printf("❌ Failed to send status: %v", err)
	}
	return err
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

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteJSON(pong)
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}
