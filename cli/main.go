package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// CLI flags
	var (
		command   string
		clientID  string
		serverURL string
	)

	// Subcommands
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	downloadCmd.StringVar(&clientID, "client-id", "", "Client ID to trigger download from (required)")

	statusCmd := flag.NewFlagSet("status", flag.ExitOnError)
	statusCmd.StringVar(&clientID, "client-id", "", "Client ID to check status (required)")

	listCmd := flag.NewFlagSet("list", flag.ExitOnError)

	fmt.Println("🚀 File Download System - CLI")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command = os.Args[1]

	serverURL = os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	switch command {
	case "download":
		downloadCmd.Parse(os.Args[2:])
		if clientID == "" {
			fmt.Println("❌ Error: --client-id is required")
			downloadCmd.PrintDefaults()
			os.Exit(1)
		}
		triggerDownload(serverURL, clientID)

	case "status":
		statusCmd.Parse(os.Args[2:])
		if clientID == "" {
			fmt.Println("❌ Error: --client-id is required")
			statusCmd.PrintDefaults()
			os.Exit(1)
		}
		checkStatus(serverURL, clientID)

	case "list":
		listCmd.Parse(os.Args[2:])
		listClients(serverURL)

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("\nUsage:")
	fmt.Println("  cli download --client-id=<client-id>")
	fmt.Println("  cli status --client-id=<client-id>")
	fmt.Println("  cli list")
	fmt.Println("\nExamples:")
	fmt.Println("  cli download --client-id=restaurant-1")
	fmt.Println("  cli status --client-id=restaurant-1")
	fmt.Println("  cli list")
}

func triggerDownload(serverURL, clientID string) {
	fmt.Printf("📥 Triggering download for client: %s\n", clientID)
	fmt.Printf("🔗 Server: %s\n", serverURL)

	url := fmt.Sprintf("%s/trigger-download/%s", serverURL, clientID)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Error reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Server returned error (status %d):\n", resp.StatusCode)
		fmt.Println(string(body))
		os.Exit(1)
	}

	// Parse and pretty print response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("❌ Error parsing response: %v\n", err)
		fmt.Println(string(body))
		os.Exit(1)
	}

	fmt.Println("\n✅ Download triggered successfully!")
	fmt.Printf("   Upload ID: %v\n", result["upload_id"])
	fmt.Printf("   S3 Key: %v\n", result["s3_key"])
	fmt.Printf("   Message: %v\n", result["message"])
}

func checkStatus(serverURL, clientID string) {
	fmt.Printf("📊 Checking status for client: %s\n", clientID)
	fmt.Printf("🔗 Server: %s\n", serverURL)

	url := fmt.Sprintf("%s/status/%s", serverURL, clientID)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Error reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Server returned error (status %d):\n", resp.StatusCode)
		fmt.Println(string(body))
		os.Exit(1)
	}

	// Parse and pretty print response
	var status map[string]interface{}
	if err := json.Unmarshal(body, &status); err != nil {
		fmt.Printf("❌ Error parsing response: %v\n", err)
		fmt.Println(string(body))
		os.Exit(1)
	}

	fmt.Println("\n📊 Client Status:")
	fmt.Printf("   Client ID: %v\n", status["client_id"])
	fmt.Printf("   Connected: %v\n", status["connected"])
	if status["connected"] == true {
		if ca, ok := status["connected_at"].(string); ok {
			fmt.Printf("   Connected At: %v\n", ca)
		}
		if lh, ok := status["last_heartbeat"].(string); ok {
			fmt.Printf("   Last Heartbeat: %v\n", lh)
		}
	}
	fmt.Printf("   Total Uploads: %v\n", status["total_uploads"])
	fmt.Printf("   Success Uploads: %v\n", status["success_uploads"])
	fmt.Printf("   Failed Uploads: %v\n", status["failed_uploads"])

	if currentUpload, ok := status["current_upload"].(map[string]interface{}); ok && currentUpload != nil {
		fmt.Println("\n   Current Upload:")
		fmt.Printf("      Upload ID: %v\n", currentUpload["upload_id"])
		fmt.Printf("      File Path: %v\n", currentUpload["file_path"])
		fmt.Printf("      Status: %v\n", currentUpload["status"])
		fmt.Printf("      Progress: %.1f%%\n", currentUpload["progress"])
		fmt.Printf("      Completed Parts: %v/%v\n", currentUpload["completed_parts"], currentUpload["total_parts"])
	}
}

func listClients(serverURL string) {
	fmt.Printf("📋 Listing connected clients\n")
	fmt.Printf("🔗 Server: %s\n", serverURL)

	url := fmt.Sprintf("%s/clients", serverURL)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Error reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Server returned error (status %d):\n", resp.StatusCode)
		fmt.Println(string(body))
		os.Exit(1)
	}

	// Parse and pretty print response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("❌ Error parsing response: %v\n", err)
		fmt.Println(string(body))
		os.Exit(1)
	}

	clients, ok := result["clients"].([]interface{})
	if !ok {
		fmt.Println("❌ Invalid response format")
		os.Exit(1)
	}

	fmt.Printf("\n✅ Connected Clients: %v\n", result["count"])
	if len(clients) > 0 {
		for i, client := range clients {
			fmt.Printf("   %d. %v\n", i+1, client)
		}
	} else {
		fmt.Println("   (no clients connected)")
	}
}
