package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Client wraps the AWS S3 client
type Client struct {
	s3Client           *s3.Client
	bucket             string
	region             string
	presignedURLExpiry time.Duration
}

// Config contains the S3 client configuration
type Config struct {
	Region             string
	Bucket             string
	EndpointURL        string // For LocalStack
	AccessKeyID        string
	SecretAccessKey    string
	PresignedURLExpiry time.Duration
}

// NewClient creates a new S3 client
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	var awsCfg aws.Config
	var err error

	// Load configuration
	if cfg.EndpointURL != "" {
		// For LocalStack or custom endpoint
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               cfg.EndpointURL,
				HostnameImmutable: true,
				SigningRegion:     cfg.Region,
			}, nil
		})

		awsCfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Region),
			config.WithEndpointResolverWithOptions(customResolver),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"",
			)),
		)
	} else {
		// For production AWS
		if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
			awsCfg, err = config.LoadDefaultConfig(ctx,
				config.WithRegion(cfg.Region),
				config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
					cfg.AccessKeyID,
					cfg.SecretAccessKey,
					"",
				)),
			)
		} else {
			awsCfg, err = config.LoadDefaultConfig(ctx,
				config.WithRegion(cfg.Region),
			)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := &Client{
		s3Client:           s3.NewFromConfig(awsCfg),
		bucket:             cfg.Bucket,
		region:             cfg.Region,
		presignedURLExpiry: cfg.PresignedURLExpiry,
	}

	// Ensure bucket exists
	if err := client.ensureBucket(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket exists: %w", err)
	}

	return client, nil
}

// ensureBucket ensures the S3 bucket exists, creates it if not
func (c *Client) ensureBucket(ctx context.Context) error {
	// Check if bucket exists
	_, err := c.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.bucket),
	})

	if err == nil {
		// Bucket exists
		return nil
	}

	// Try to create bucket
	_, err = c.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(c.bucket),
	})

	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	return nil
}

// MultipartUploadConfig contains configuration for multipart upload
type MultipartUploadConfig struct {
	Key       string
	FileSize  int64
	ChunkSize int64
	Metadata  map[string]string
}

// InitiateMultipartUpload starts a multipart upload and generates presigned URLs
func (c *Client) InitiateMultipartUpload(ctx context.Context, cfg MultipartUploadConfig) (*MultipartUpload, error) {
	// Initiate multipart upload
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(cfg.Key),
	}

	// Add metadata if provided
	if len(cfg.Metadata) > 0 {
		input.Metadata = cfg.Metadata
	}

	output, err := c.s3Client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate multipart upload: %w", err)
	}

	uploadID := aws.ToString(output.UploadId)

	// Calculate number of parts
	totalParts := int(cfg.FileSize / cfg.ChunkSize)
	if cfg.FileSize%cfg.ChunkSize != 0 {
		totalParts++
	}

	// Generate presigned URLs for each part
	presignClient := s3.NewPresignClient(c.s3Client)
	presignedURLs := make([]PresignedURL, totalParts)

	for i := 0; i < totalParts; i++ {
		partNumber := i + 1
		request, err := presignClient.PresignUploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(c.bucket),
			Key:        aws.String(cfg.Key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(int32(partNumber)),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = c.presignedURLExpiry
		})

		if err != nil {
			// Abort the multipart upload if we fail to generate presigned URLs
			c.AbortMultipartUpload(ctx, cfg.Key, uploadID)
			return nil, fmt.Errorf("failed to generate presigned URL for part %d: %w", partNumber, err)
		}

		presignedURLs[i] = PresignedURL{
			PartNumber: partNumber,
			URL:        request.URL,
		}
	}

	return &MultipartUpload{
		UploadID:      uploadID,
		Bucket:        c.bucket,
		Key:           cfg.Key,
		TotalParts:    totalParts,
		ChunkSize:     cfg.ChunkSize,
		PresignedURLs: presignedURLs,
	}, nil
}

// CompleteMultipartUpload completes a multipart upload
func (c *Client) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
	completedParts := make([]types.CompletedPart, len(parts))
	for i, part := range parts {
		completedParts[i] = types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(int32(part.PartNumber)),
		}
	}

	_, err := c.s3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return nil
}

// AbortMultipartUpload aborts a multipart upload
func (c *Client) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	_, err := c.s3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})

	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	return nil
}

// GetObjectMetadata retrieves metadata for an S3 object
func (c *Client) GetObjectMetadata(ctx context.Context, key string) (*ObjectMetadata, error) {
	output, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get object metadata: %w", err)
	}

	return &ObjectMetadata{
		Key:          key,
		Size:         aws.ToInt64(output.ContentLength),
		LastModified: aws.ToTime(output.LastModified),
		ETag:         aws.ToString(output.ETag),
		ContentType:  aws.ToString(output.ContentType),
		Metadata:     output.Metadata,
	}, nil
}

// DeleteObject deletes an object from S3
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	_, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// MultipartUpload contains information about a multipart upload
type MultipartUpload struct {
	UploadID      string
	Bucket        string
	Key           string
	TotalParts    int
	ChunkSize     int64
	PresignedURLs []PresignedURL
}

// PresignedURL contains a presigned URL for a specific part
type PresignedURL struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

// CompletedPart represents a completed upload part
type CompletedPart struct {
	PartNumber int
	ETag       string
}

// ObjectMetadata contains metadata about an S3 object
type ObjectMetadata struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
	ContentType  string
	Metadata     map[string]string
}
