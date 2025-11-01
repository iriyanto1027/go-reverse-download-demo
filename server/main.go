package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/iriyanto1027/file-download-system/server/api"
	"github.com/iriyanto1027/file-download-system/server/s3"
	"github.com/iriyanto1027/file-download-system/server/websocket"
	"github.com/iriyanto1027/file-download-system/shared/auth"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	fmt.Println("🚀 File Download System - Server")
	fmt.Println("================================")

	// Parse configuration
	cfg := loadConfig()

	fmt.Printf("📡 Server Host: %s\n", cfg.ServerHost)
	fmt.Printf("📡 Server Port: %s\n", cfg.ServerPort)
	fmt.Printf("☁️  AWS Region: %s\n", cfg.AWSRegion)
	fmt.Printf("🪣  S3 Bucket: %s\n", cfg.S3Bucket)
	if cfg.AWSEndpointURL != "" {
		fmt.Printf("🔧 AWS Endpoint: %s (LocalStack mode)\n", cfg.AWSEndpointURL)
	}
	fmt.Println("================================")

	ctx := context.Background()

	// Initialize S3 client
	fmt.Println("� Initializing S3 client...")
	s3Client, err := s3.NewClient(ctx, s3.Config{
		Region:             cfg.AWSRegion,
		Bucket:             cfg.S3Bucket,
		EndpointURL:        cfg.AWSEndpointURL,
		AccessKeyID:        cfg.AWSAccessKeyID,
		SecretAccessKey:    cfg.AWSSecretAccessKey,
		PresignedURLExpiry: cfg.PresignedURLExpiry,
	})
	if err != nil {
		log.Fatalf("❌ Failed to initialize S3 client: %v", err)
	}
	fmt.Println("✅ S3 client initialized")

	// Initialize token manager (optional)
	var tokenManager *auth.TokenManager
	if cfg.JWTSecret != "" {
		tokenManager = auth.NewTokenManager(cfg.JWTSecret, "file-download-system")
		fmt.Println("✅ JWT token manager initialized")
	}

	// Initialize WebSocket manager
	fmt.Println("🔧 Initializing WebSocket manager...")
	wsManager := websocket.NewManager(websocket.Config{
		PingInterval:  30 * time.Second,
		ClientTimeout: 90 * time.Second,
		ReadLimit:     1024 * 1024, // 1MB
	}, nil) // Handler will be set later
	fmt.Println("✅ WebSocket manager initialized")

	// Initialize API handler (also acts as message handler for WebSocket)
	fmt.Println("🔧 Initializing API handler...")
	apiHandler := api.NewHandler(wsManager, s3Client, api.Config{
		ChunkSize:  cfg.ChunkSize,
		BaseS3Path: "uploads",
	})
	fmt.Println("✅ API handler initialized")

	// Set the message handler for WebSocket manager
	wsManager.SetMessageHandler(apiHandler)

	// Initialize WebSocket HTTP handler
	wsHandler := websocket.NewHandler(wsManager, tokenManager)

	// Setup routes
	fmt.Println("🔧 Setting up routes...")

	// WebSocket endpoint
	http.HandleFunc("/ws/connect", wsHandler.HandleConnect)

	// API endpoints
	http.HandleFunc("/trigger-download/", apiHandler.TriggerDownload)
	http.HandleFunc("/status/", apiHandler.GetStatus)
	http.HandleFunc("/uploads/", apiHandler.GetUploadStatus)
	http.HandleFunc("/clients", apiHandler.ListClients)
	http.HandleFunc("/health", apiHandler.HealthCheck)

	// Root endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"running","service":"file-download-system","version":"1.0.0"}`)
	})

	fmt.Println("✅ Routes configured")
	fmt.Println("\n📍 Available Endpoints:")
	fmt.Println("   WebSocket:  /ws/connect")
	fmt.Println("   API:        POST /trigger-download/{client_id}")
	fmt.Println("   API:        GET  /status/{client_id}")
	fmt.Println("   API:        GET  /uploads/{upload_id}")
	fmt.Println("   API:        GET  /clients")
	fmt.Println("   API:        GET  /health")

	addr := cfg.ServerHost + ":" + cfg.ServerPort
	fmt.Printf("\n✅ Server ready at http://%s\n", addr)
	fmt.Println("Press Ctrl+C to stop")

	log.Fatal(http.ListenAndServe(addr, nil))
}

// Config holds the server configuration
type Config struct {
	ServerHost         string
	ServerPort         string
	AWSRegion          string
	AWSEndpointURL     string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	S3Bucket           string
	PresignedURLExpiry time.Duration
	ChunkSize          int64
	JWTSecret          string
}

// loadConfig loads configuration from environment variables
func loadConfig() Config {
	cfg := Config{
		ServerHost:         getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort:         getEnv("SERVER_PORT", "8080"),
		AWSRegion:          getEnv("AWS_REGION", "us-east-1"),
		AWSEndpointURL:     getEnv("AWS_ENDPOINT_URL", ""),
		AWSAccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", ""),
		AWSSecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
		S3Bucket:           getEnv("S3_BUCKET_NAME", "file-download-system-uploads"),
		JWTSecret:          getEnv("JWT_SECRET", ""),
	}

	// Parse presigned URL expiry
	expiryStr := getEnv("S3_PRESIGNED_URL_EXPIRY", "15m")
	expiry, err := time.ParseDuration(expiryStr)
	if err != nil {
		log.Printf("Warning: Invalid S3_PRESIGNED_URL_EXPIRY '%s', using default 15m", expiryStr)
		expiry = 15 * time.Minute
	}
	cfg.PresignedURLExpiry = expiry

	// Parse chunk size
	chunkSizeStr := getEnv("S3_CHUNK_SIZE", "5242880") // 5MB default
	chunkSize, err := strconv.ParseInt(chunkSizeStr, 10, 64)
	if err != nil {
		log.Printf("Warning: Invalid S3_CHUNK_SIZE '%s', using default 5MB", chunkSizeStr)
		chunkSize = 5 * 1024 * 1024
	}
	cfg.ChunkSize = chunkSize

	return cfg
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
