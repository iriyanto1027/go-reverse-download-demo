package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/iriyanto1027/file-download-system/client/config"
	"github.com/iriyanto1027/file-download-system/client/handler"
	"github.com/iriyanto1027/file-download-system/client/websocket"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	fmt.Println("🚀 File Download System - Client")
	fmt.Println("================================")

	// Load configuration
	cfg := config.Load()

	fmt.Printf("🆔 Client ID: %s\n", cfg.ClientID)
	fmt.Printf("📡 Server URL: %s\n", cfg.ServerWSURL)
	fmt.Printf("📁 File Path: %s\n", cfg.FilePath)
	fmt.Println("================================")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create WebSocket client (without handler first)
	wsClient := websocket.NewClient(websocket.Config{
		ClientID:  cfg.ClientID,
		ServerURL: cfg.ServerWSURL,
		Token:     cfg.ClientToken,
	}, nil)

	// Create command handler
	commandHandler := handler.NewCommandHandler(wsClient, cfg.FilePath)

	// Set the command handler for WebSocket client
	// We need to update the client's handler
	wsClient = websocket.NewClient(websocket.Config{
		ClientID:  cfg.ClientID,
		ServerURL: cfg.ServerWSURL,
		Token:     cfg.ClientToken,
	}, commandHandler)

	// Update command handler with new client instance
	commandHandler = handler.NewCommandHandler(wsClient, cfg.FilePath)

	// Recreate client with handler
	wsClient = websocket.NewClient(websocket.Config{
		ClientID:  cfg.ClientID,
		ServerURL: cfg.ServerWSURL,
		Token:     cfg.ClientToken,
	}, commandHandler)

	fmt.Println("🔧 Starting WebSocket client...")
	fmt.Println("✅ Client ready and waiting for commands...")
	fmt.Println("Press Ctrl+C to exit")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start client in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := wsClient.Start(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-sigChan:
		fmt.Println("\n👋 Shutting down client...")
		cancel()
		wsClient.Stop()
	case err := <-errChan:
		if err != nil {
			log.Printf("❌ Client error: %v", err)
		}
	}

	fmt.Println("✅ Client stopped")
}
