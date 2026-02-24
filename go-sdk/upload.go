package cpay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

func (c *Client) uploadMultipart(ctx context.Context, path, fieldName string, reader io.Reader, filename string, extraFields map[string]string) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		return "", fmt.Errorf("cpay: create form file: %w", err)
	}
	if _, err := io.Copy(part, reader); err != nil {
		return "", fmt.Errorf("cpay: copy file data: %w", err)
	}
	for k, v := range extraFields {
		if v != "" {
			if err := w.WriteField(k, v); err != nil {
				return "", fmt.Errorf("cpay: write field %s: %w", k, err)
			}
		}
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("cpay: close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	var result uploadResponse
	if err := c.do(req, &result); err != nil {
		return "", err
	}
	return result.URL, nil
}

// UploadAvatar uploads a shop avatar image. Returns the image URL.
// Allowed types: image/png, image/jpeg, image/webp. Max 5MB.
func (c *Client) UploadAvatar(ctx context.Context, reader io.Reader, filename string) (string, error) {
	return c.uploadMultipart(ctx, "/api/upload/avatar", "file", reader, filename, nil)
}

// UploadProductImage uploads a product image. Returns the image URL.
// productID is optional; if empty, a UUID is generated server-side.
// Allowed types: image/png, image/jpeg, image/webp. Max 5MB.
func (c *Client) UploadProductImage(ctx context.Context, reader io.Reader, filename string, productID string) (string, error) {
	extra := map[string]string{}
	if productID != "" {
		extra["product_id"] = productID
	}
	return c.uploadMultipart(ctx, "/api/upload/product-image", "file", reader, filename, extra)
}
