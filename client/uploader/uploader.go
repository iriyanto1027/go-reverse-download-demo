package uploader

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	sharedModels "github.com/iriyanto1027/file-download-system/shared/models"
)

// Uploader handles file uploads to S3 using presigned URLs
type Uploader struct {
	filePath     string
	uploadConfig sharedModels.UploadConfig
	client       *http.Client
	mu           sync.RWMutex
}

// UploadResult contains the result of an upload
type UploadResult struct {
	Success        bool
	UploadID       string
	FileSize       int64
	TotalParts     int
	CompletedParts int
	ETags          map[int]string
	Error          error
	Duration       time.Duration
}

// ProgressCallback is called during upload progress
type ProgressCallback func(partNumber, totalParts int, bytesUploaded, totalBytes int64)

// NewUploader creates a new uploader
func NewUploader(filePath string, uploadConfig sharedModels.UploadConfig) *Uploader {
	return &Uploader{
		filePath:     filePath,
		uploadConfig: uploadConfig,
		client: &http.Client{
			Timeout: 10 * time.Minute, // Long timeout for large files
		},
	}
}

// Upload uploads the file using multipart upload with presigned URLs
func (u *Uploader) Upload(progressCallback ProgressCallback) (*UploadResult, error) {
	startTime := time.Now()

	// Open file
	file, err := os.Open(u.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := fileInfo.Size()

	log.Printf("Uploading file: %s (%.2f MB)", u.filePath, float64(fileSize)/(1024*1024))
	log.Printf("Upload ID: %s", u.uploadConfig.UploadID)
	log.Printf("Total parts: %d", len(u.uploadConfig.PresignedURLs))

	// Upload each part
	etags := make(map[int]string)
	var bytesUploaded int64
	var mu sync.Mutex

	totalParts := len(u.uploadConfig.PresignedURLs)

	for _, presignedURL := range u.uploadConfig.PresignedURLs {
		partNumber := presignedURL.PartNumber

		// Calculate part size
		partSize := u.uploadConfig.ChunkSize
		offset := int64(partNumber-1) * u.uploadConfig.ChunkSize

		// Adjust for last part
		if offset+partSize > fileSize {
			partSize = fileSize - offset
		}

		// Read part data
		partData := make([]byte, partSize)
		if _, err := file.ReadAt(partData, offset); err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read part %d: %w", partNumber, err)
		}

		// Upload part
		log.Printf("Uploading part %d/%d (%.2f MB)", partNumber, totalParts, float64(partSize)/(1024*1024))

		etag, err := u.uploadPart(presignedURL.URL, partData)
		if err != nil {
			return nil, fmt.Errorf("failed to upload part %d: %w", partNumber, err)
		}

		// Store ETag
		mu.Lock()
		etags[partNumber] = etag
		bytesUploaded += partSize
		mu.Unlock()

		log.Printf("✅ Part %d/%d uploaded (ETag: %s)", partNumber, totalParts, etag)

		// Call progress callback
		if progressCallback != nil {
			progressCallback(partNumber, totalParts, bytesUploaded, fileSize)
		}
	}

	duration := time.Since(startTime)
	log.Printf("✅ Upload completed in %v (%.2f MB/s)", duration, float64(fileSize)/(1024*1024)/duration.Seconds())

	return &UploadResult{
		Success:        true,
		UploadID:       u.uploadConfig.UploadID,
		FileSize:       fileSize,
		TotalParts:     totalParts,
		CompletedParts: len(etags),
		ETags:          etags,
		Duration:       duration,
	}, nil
}

// uploadPart uploads a single part using a presigned URL
func (u *Uploader) uploadPart(presignedURL string, data []byte) (string, error) {
	req, err := http.NewRequest(http.MethodPut, presignedURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := u.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Get ETag from response header
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", fmt.Errorf("no ETag in response")
	}

	return etag, nil
}

// GetFileSize returns the size of the file to upload
func GetFileSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// CalculateParts calculates the number of parts needed for a file
func CalculateParts(fileSize, chunkSize int64) int {
	parts := int(fileSize / chunkSize)
	if fileSize%chunkSize != 0 {
		parts++
	}
	return parts
}
