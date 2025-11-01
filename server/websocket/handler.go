package websocket

import (
	"log"
	"net/http"
	"strings"

	"github.com/iriyanto1027/file-download-system/shared/auth"
)

// Handler handles WebSocket HTTP requests
type Handler struct {
	manager      *Manager
	tokenManager *auth.TokenManager
}

// NewHandler creates a new WebSocket HTTP handler
func NewHandler(manager *Manager, tokenManager *auth.TokenManager) *Handler {
	return &Handler{
		manager:      manager,
		tokenManager: tokenManager,
	}
}

// HandleConnect handles WebSocket connection requests at /ws/connect
func (h *Handler) HandleConnect(w http.ResponseWriter, r *http.Request) {
	// Extract client ID from query parameter or header
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = r.Header.Get("X-Client-ID")
	}

	if clientID == "" {
		http.Error(w, "Client ID is required", http.StatusBadRequest)
		return
	}

	// Validate client ID
	if !auth.ValidateClientID(clientID) {
		http.Error(w, "Invalid client ID", http.StatusBadRequest)
		return
	}

	// Optional: Validate JWT token if provided
	token := r.URL.Query().Get("token")
	if token == "" {
		// Try to get from Authorization header
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if token != "" && h.tokenManager != nil {
		claims, err := h.tokenManager.ValidateToken(token)
		if err != nil {
			log.Printf("Invalid token for client %s: %v", clientID, err)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Verify client ID matches token
		if claims.ClientID != clientID {
			log.Printf("Client ID mismatch: token=%s, query=%s", claims.ClientID, clientID)
			http.Error(w, "Client ID mismatch", http.StatusUnauthorized)
			return
		}
	}

	// Upgrade to WebSocket
	conn, err := h.manager.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection for client %s: %v", clientID, err)
		return
	}

	log.Printf("WebSocket connection established for client: %s", clientID)

	// Handle the client connection
	h.manager.HandleClient(r.Context(), clientID, conn)
}
