#!/bin/bash

echo "Waiting for LocalStack to be ready..."
sleep 5

echo "Creating S3 bucket: file-download-system-uploads"
aws --endpoint-url=http://localhost:4566 s3 mb s3://file-download-system-uploads 2>/dev/null || echo "Bucket already exists"

echo "Listing S3 buckets:"
aws --endpoint-url=http://localhost:4566 s3 ls

echo "LocalStack initialization complete!"
