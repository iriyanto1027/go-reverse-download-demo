# File Download System

A scalable solution for downloading files from on-premise clients to a cloud server using WebSocket and AWS S3 presigned URLs.

## 🏗️ Architecture

- **Server**: Cloud-hosted application managing client connections via WebSocket
- **Client**: On-premise agent that uploads files to S3 when commanded
- **Storage**: AWS S3 with presigned URLs for direct, secure uploads

## 📋 Prerequisites

- Go 1.22 or higher
- Docker & Docker Compose (for LocalStack testing)
- AWS Account (for production) or LocalStack (for local testing)

## 🚀 Quick Start

### 1. Setup Environment

```bash
# Copy environment template
cp .env.example .env

# Edit .env with your configuration
# For local testing with LocalStack, the defaults work fine
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Run with LocalStack (Local Testing)

```bash
# Start all services (LocalStack, Server, Client)
docker-compose up

# Or run separately:

# Terminal 1: Start LocalStack
docker-compose up localstack

# Terminal 2: Run server
go run ./server/main.go

# Terminal 3: Run client
go run ./client/main.go
```

### 4. Trigger Download

```bash
# Using CLI
go run ./cli/main.go download --client-id=restaurant-1

# Or using curl
curl -X POST http://localhost:8080/trigger-download/restaurant-1
```

## 📁 Project Structure

```
.
├── server/              # Server application
│   ├── main.go
│   ├── websocket/      # WebSocket handlers
│   ├── api/            # REST API handlers
│   ├── s3/             # S3 client & presigned URLs
│   └── models/         # Data models
├── client/             # Client application
│   ├── main.go
│   ├── websocket/      # WebSocket client
│   ├── uploader/       # S3 uploader with chunking
│   └── config/         # Client configuration
├── cli/                # CLI tool
│   └── main.go
├── shared/             # Shared code
│   ├── auth/          # JWT utilities
│   └── models/        # Protocol definitions
├── docker-compose.yml
├── .env.example
└── README.md
```

## 🔧 Configuration

See `.env.example` for all configuration options.

Key settings:

- `AWS_REGION`: AWS region for S3
- `S3_BUCKET_NAME`: S3 bucket for uploads
- `S3_CHUNK_SIZE`: Chunk size for multipart upload (default: 5MB)
- `JWT_SECRET`: Secret key for JWT tokens

## 📝 Development

```bash
# Run server in development mode
go run ./server/main.go

# Run client in development mode
go run ./client/main.go

# Build binaries
go build -o bin/server ./server/main.go
go build -o bin/client ./client/main.go
go build -o bin/cli ./cli/main.go
```

## 🧪 Testing

```bash
# Create test file (100MB)
dd if=/dev/zero of=test-data/test-file.bin bs=1M count=100

# Run tests
go test ./...
```

## 📚 API Documentation

### WebSocket

- `ws://localhost:8080/ws/connect` - Client connection endpoint

### REST API

- `POST /trigger-download/{client_id}` - Trigger file download
- `GET /status/{client_id}` - Check client status
- `GET /clients` - List connected clients

## 🔒 Security

- WebSocket authentication via JWT tokens
- S3 presigned URLs with 15-minute expiry
- Time-limited upload sessions
- Private S3 bucket with encryption at rest

## 📖 More Information

See [MY_APPROACH.md](MY_APPROACH.md) for detailed architecture and design decisions.

## 📄 License

MIT
