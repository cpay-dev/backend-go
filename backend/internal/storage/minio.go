package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOService struct {
	client    *minio.Client
	bucket    string
	publicURL string
}

func NewMinIOService(endpoint, accessKey, secretKey, bucket, publicURL string, useSSL bool) (*MinIOService, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &MinIOService{
		client:    client,
		bucket:    bucket,
		publicURL: publicURL,
	}, nil
}

func (s *MinIOService) UploadAvatar(ctx context.Context, userID, filename string, reader io.Reader, size int64, contentType string) (string, error) {
	objectName := fmt.Sprintf("avatars/%s/%s", userID, filename)
	_, err := s.client.PutObject(ctx, s.bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("upload avatar: %w", err)
	}

	url := fmt.Sprintf("%s/%s/%s", s.publicURL, s.bucket, objectName)
	return url, nil
}

func (s *MinIOService) UploadProductImage(ctx context.Context, shopID, filename string, reader io.Reader, size int64, contentType string) (string, error) {
	objectName := fmt.Sprintf("products/%s/%s", shopID, filename)
	_, err := s.client.PutObject(ctx, s.bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("upload product image: %w", err)
	}

	url := fmt.Sprintf("%s/%s/%s", s.publicURL, s.bucket, objectName)
	return url, nil
}
