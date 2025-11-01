# File Download System (Reverse Download)

A scalable reverse file download system where the **server triggers** on-premise clients to upload files to cloud storage using WebSocket commands and AWS S3 presigned URLs.

## 🏗️ Architecture

This is a **"reverse download"** system - instead of clients requesting files from the server, the **server commands clients** to upload their local files to cloud storage:

- **Server**: Cloud-hosted application that manages client connections via WebSocket and triggers file uploads
- **Client**: On-premise agent that listens for download commands and uploads files directly to S3
- **Storage**: AWS S3 with presigned URLs for direct, secure multipart uploads

### Flow:

1. Client connects to server via WebSocket
2. Server sends "download_file" command to client
3. Server generates S3 presigned URLs for multipart upload
4. Client uploads file chunks directly to S3 (not through server)
5. Client sends completion response with ETags back to server
6. Server completes the multipart upload on S3

## 📋 Prerequisites

- **Docker** and **Docker Compose** (recommended for quick testing)
- Go 1.22+ (only if building from source)

## 🚀 Quick Start with Docker

### 1. Clone and Prepare

```bash
git clone <repository-url>
cd go-reverse-download-demo

# Create test file (10MB for quick testing)
mkdir -p test-data
dd if=/dev/urandom of=test-data/test-file.bin bs=1M count=10
```

### 2. Start All Services

```bash
# Start LocalStack (S3), Server, and Client
docker-compose up -d

# Check logs
docker-compose logs -f
```

This will start:

- **LocalStack S3** on `http://localhost:4566`
- **Server** on `http://localhost:8080`
- **Client** (connects to server automatically)

### 3. Trigger File Upload

```bash
# Trigger the client to upload test-file.bin to S3
curl -X POST http://localhost:8080/trigger-download/restaurant-1

# Expected response:
# {
#   "success": true,
#   "message": "Download triggered for client restaurant-1",
#   "upload_id": "abc123...",
#   "s3_key": "uploads/restaurant-1/20251101-123456-test-file.bin"
# }
```

### 4. Verify Upload

```bash
# List files in S3 bucket
curl "http://localhost:4566/file-download-system-uploads?list-type=2"

# You should see your uploaded file:
# <Key>uploads/restaurant-1/20251101-123456-test-file.bin</Key>
# <Size>10485760</Size>
```

### 5. Check Upload Status

```bash
# Get client status
curl http://localhost:8080/status/restaurant-1

# Get specific upload status
curl http://localhost:8080/uploads/<upload_id>

# List all connected clients
curl http://localhost:8080/clients
```

## 🧪 Testing Different File Sizes

```bash
# Small file (5MB - single part)
dd if=/dev/urandom of=test-data/test-file.bin bs=1M count=5
docker-compose restart client
curl -X POST http://localhost:8080/trigger-download/restaurant-1

# Medium file (25MB - 5 parts)
dd if=/dev/urandom of=test-data/test-file.bin bs=1M count=25
docker-compose restart client
curl -X POST http://localhost:8080/trigger-download/restaurant-1

# Large file (100MB - 20 parts)
dd if=/dev/urandom of=test-data/test-file.bin bs=1M count=100
docker-compose restart client
curl -X POST http://localhost:8080/trigger-download/restaurant-1
```

## 🛑 Stop Services

```bash
# Stop all services
docker-compose down

# Stop and remove volumes (clean slate)
docker-compose down -v
```

## 📁 Project Structure

```
.
├── server/              # Server application (cloud-hosted)
│   ├── main.go         # Entry point
│   ├── websocket/      # WebSocket connection manager
│   ├── api/            # REST API & message handlers
│   ├── s3/             # S3 client with presigned URLs
│   └── models/         # Upload status & client models
├── client/             # Client application (on-premise)
│   ├── main.go         # Entry point
│   ├── websocket/      # WebSocket client with auto-reconnect
│   ├── handler/        # Command handler for download_file
│   ├── uploader/       # S3 multipart uploader
│   └── config/         # Client configuration
├── cli/                # CLI tool for testing
│   └── main.go
├── shared/             # Shared code between server & client
│   ├── auth/          # JWT authentication utilities
│   └── models/        # WebSocket protocol definitions
├── test-data/          # Test files for upload
├── docker-compose.yml  # Docker services configuration
├── Dockerfile.server   # Server Docker image
├── Dockerfile.client   # Client Docker image
├── .env               # Environment configuration (create from .env.example)
└── README.md
```

## 🔧 Configuration

The system uses environment variables for configuration. Docker Compose automatically loads from `.env` file.

### Key Environment Variables:

**AWS/S3 Configuration:**

```bash
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=test           # For LocalStack
AWS_SECRET_ACCESS_KEY=test       # For LocalStack
S3_BUCKET_NAME=file-download-system-uploads
S3_CHUNK_SIZE=5242880            # 5MB chunks for multipart upload
AWS_ENDPOINT=http://localstack:4566  # LocalStack endpoint (remove for real AWS)
```

**Server Configuration:**

```bash
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
BASE_S3_PATH=uploads             # S3 prefix for uploaded files
# JWT_SECRET=your-secret-key     # Commented out for development (no auth)
```

**Client Configuration:**

```bash
CLIENT_ID=restaurant-1
SERVER_URL=ws://server:8080/ws/connect
FILE_PATH=/data/test-file.bin    # File to upload when triggered
```

## 📝 Development (Without Docker)

If you want to run services locally without Docker:

### 1. Install Dependencies

```bash
go mod download
```

### 2. Start LocalStack

```bash
docker-compose up -d localstack
```

### 3. Set Environment Variables

```bash
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export S3_BUCKET_NAME=file-download-system-uploads
export AWS_ENDPOINT=http://localhost:4566
export SERVER_HOST=0.0.0.0
export SERVER_PORT=8080
```

### 4. Run Server

```bash
go run ./server/main.go
```

### 5. Run Client (in another terminal)

```bash
export CLIENT_ID=restaurant-1
export SERVER_URL=ws://localhost:8080/ws/connect
export FILE_PATH=./test-data/test-file.bin
go run ./client/main.go
```

### 6. Build Binaries

```bash
# Build all binaries
go build -o bin/server ./server/main.go
go build -o bin/client ./client/main.go
go build -o bin/cli ./cli/main.go

# Run built binaries
./bin/server
./bin/client
```

## 🧪 Testing & Debugging

### View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f server
docker-compose logs -f client
docker-compose logs -f localstack
```

### Check Service Health

```bash
# Server health check
curl http://localhost:8080/health

# LocalStack S3 health
curl http://localhost:4566/_localstack/health
```

### Debug Upload Issues

```bash
# Check if client is connected
curl http://localhost:8080/clients

# Check client status
curl http://localhost:8080/status/restaurant-1

# Trigger upload with custom file path
curl -X POST http://localhost:8080/trigger-download/restaurant-1 \
  -H "Content-Type: application/json" \
  -d '{"file_path": "/data/test-file.bin"}'

# Check upload progress
curl http://localhost:8080/uploads/<upload_id>
```

### Inspect S3 Bucket

```bash
# List all files
curl "http://localhost:4566/file-download-system-uploads?list-type=2"

# Download a file from S3
curl "http://localhost:4566/file-download-system-uploads/uploads/restaurant-1/20251101-123456-test-file.bin" \
  -o downloaded-file.bin

# Verify file integrity
md5sum test-data/test-file.bin downloaded-file.bin
```

## 🔍 Troubleshooting

### Client not connecting?

```bash
# Check if server is running
docker-compose ps

# Restart client
docker-compose restart client
```

### Upload failing?

```bash
# Check server logs for errors
docker-compose logs server | grep -i error

# Check client logs
docker-compose logs client | tail -50

# Verify file exists in client container
docker exec file-download-client ls -lh /data/
```

### LocalStack issues?

```bash
# Restart LocalStack
docker-compose restart localstack

# Check LocalStack logs
docker-compose logs localstack

# Verify S3 bucket exists
docker exec localstack-s3 awslocal s3 ls
```

## 📚 API Documentation

### WebSocket Protocol

**Connection:**

- `ws://localhost:8080/ws/connect?client_id={client_id}` - Client connection endpoint

**Message Types:**

1. **Command (Server → Client):**

```json
{
  "message_id": "abc123",
  "action": "download_file",
  "payload": {
    "file_path": "/data/test-file.bin",
    "upload_config": {
      "upload_id": "upload123",
      "bucket": "file-download-system-uploads",
      "key": "uploads/restaurant-1/20251101-123456-test-file.bin",
      "chunk_size": 5242880,
      "presigned_urls": [
        { "part_number": 1, "url": "https://s3..." },
        { "part_number": 2, "url": "https://s3..." }
      ]
    }
  }
}
```

2. **Response (Client → Server):**

```json
{
  "message_id": "abc123",
  "action": "download_file",
  "status": "success",
  "payload": {
    "etags": {
      "1": "etag-for-part-1",
      "2": "etag-for-part-2"
    }
  }
}
```

3. **Status (Client → Server):**

```json
{
  "status": "uploading",
  "current_upload": {
    "upload_id": "upload123",
    "file_path": "/data/test-file.bin",
    "progress": 45.5
  }
}
```

### REST API

**Trigger Download:**

```bash
POST /trigger-download/{client_id}

# Optional body:
{
  "file_path": "/data/custom-file.bin",
  "metadata": {
    "key": "value"
  }
}

# Response:
{
  "success": true,
  "message": "Download triggered for client restaurant-1",
  "upload_id": "abc123",
  "s3_key": "uploads/restaurant-1/20251101-123456-test-file.bin"
}
```

**Get Client Status:**

```bash
GET /status/{client_id}

# Response:
{
  "client_id": "restaurant-1",
  "connected": true,
  "connected_at": "2025-11-01T10:00:00Z",
  "last_heartbeat": "2025-11-01T10:05:00Z",
  "current_upload": {
    "upload_id": "abc123",
    "file_path": "/data/test-file.bin",
    "status": "in_progress",
    "progress": 45.5
  }
}
```

**List Connected Clients:**

```bash
GET /clients

# Response:
{
  "clients": [
    {
      "client_id": "restaurant-1",
      "connected": true,
      "last_activity": "2025-11-01T10:05:00Z"
    }
  ],
  "total": 1
}
```

**Get Upload Status:**

```bash
GET /uploads/{upload_id}

# Response:
{
  "upload_id": "abc123",
  "file_path": "/data/test-file.bin",
  "s3_key": "uploads/restaurant-1/20251101-123456-test-file.bin",
  "status": "completed",
  "progress": 100.0,
  "completed_parts": 2,
  "total_parts": 2,
  "bytes_uploaded": 10485760
}
```

**Health Check:**

```bash
GET /health

# Response:
{
  "status": "ok",
  "timestamp": "2025-11-01T10:00:00Z"
}
```

## 🔒 Security

- **WebSocket Authentication**: JWT tokens (disabled in development mode)
- **S3 Presigned URLs**: Time-limited (15 minutes expiry)
- **Upload Sessions**: Timeout after 5 minutes
- **S3 Security**: Private bucket with server-side encryption
- **Network Isolation**: Docker containers on private bridge network

### Production Deployment Notes:

1. **Enable JWT Authentication:**

   ```bash
   # In .env file
   JWT_SECRET=your-strong-secret-key-here
   ```

2. **Use Real AWS S3:**

   ```bash
   # Remove AWS_ENDPOINT to use real AWS
   AWS_REGION=us-east-1
   AWS_ACCESS_KEY_ID=your-access-key
   AWS_SECRET_ACCESS_KEY=your-secret-key
   S3_BUCKET_NAME=your-production-bucket
   ```

3. **Configure HTTPS:**

   - Use reverse proxy (nginx, Traefik) for TLS termination
   - Use `wss://` for WebSocket connections

4. **Set Proper Timeouts:**
   ```bash
   CLIENT_TIMEOUT=300  # 5 minutes for large files
   PRESIGNED_URL_EXPIRY=900  # 15 minutes
   ```

## 🚀 Production Deployment

### Using Docker:

```bash
# Build production images
docker-compose -f docker-compose.prod.yml build

# Deploy to cloud
docker-compose -f docker-compose.prod.yml up -d
```

### Using Kubernetes:

See `k8s/` directory for Kubernetes manifests (coming soon).

## 📖 More Information

- **Architecture & Design**: See [MY_APPROACH.md](MY_APPROACH.md)
- **Problem Statement**: See [PROBLEM.md](PROBLEM.md)

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

---

## 💡 Quick Tips

- **Default client ID**: `restaurant-1` (configured in docker-compose.yml)
- **Default file path**: `/data/test-file.bin` (client container mounts `./test-data` to `/data`)
- **Chunk size**: 5MB (configurable via `S3_CHUNK_SIZE`)
- **Max parts**: 20 presigned URLs generated by default
- **File naming**: Server generates keys like `uploads/{client_id}/{timestamp}-{filename}`

## 🎯 Use Cases

- **Restaurant chains**: Upload daily sales reports from each branch
- **IoT devices**: Upload sensor data or logs to cloud storage
- **Edge computing**: Backup files from edge nodes to centralized storage
- **Distributed systems**: Collect files from multiple on-premise locations
