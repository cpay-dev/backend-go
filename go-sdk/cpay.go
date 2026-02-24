// Package cpay provides a Go client for the CPay.dev crypto payment platform API.
//
// Create a client with New, optionally passing WithBaseURL, WithToken, or WithHTTPClient.
// After authenticating via Verify, call SetToken to enable authenticated endpoints.
//
//	client := cpay.New(cpay.WithBaseURL("https://cpay.dev"))
//	nonce, _ := client.GetNonce(ctx)
//	// ... sign SIWE message with wallet ...
//	resp, _ := client.Verify(ctx, &cpay.VerifyRequest{Message: msg, Signature: sig})
//	client.SetToken(resp.Token)
package cpay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultBaseURL = "https://cpay.dev"

// Client is the CPay API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets a custom API base URL.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the default http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithToken sets the JWT bearer token for authenticated requests.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// New creates a new CPay API client.
func New(opts ...Option) *Client {
	c := &Client{
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetToken sets or replaces the JWT bearer token.
func (c *Client) SetToken(token string) {
	c.token = token
}

// Error represents an API error response.
type Error struct {
	StatusCode int    `json:"-"`
	Message    string `json:"error"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("cpay: %d %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether err is a 404 API error.
func IsNotFound(err error) bool {
	e, ok := err.(*Error)
	return ok && e.StatusCode == http.StatusNotFound
}

// IsForbidden reports whether err is a 403 API error.
func IsForbidden(err error) bool {
	e, ok := err.(*Error)
	return ok && e.StatusCode == http.StatusForbidden
}

// IsConflict reports whether err is a 409 API error.
func IsConflict(err error) bool {
	e, ok := err.(*Error)
	return ok && e.StatusCode == http.StatusConflict
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("cpay: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

func (c *Client) do(req *http.Request, result any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cpay: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr Error
		apiErr.StatusCode = resp.StatusCode
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			apiErr.Message = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		}
		return &apiErr
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("cpay: decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, result any) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *Client) post(ctx context.Context, path string, body, result any) error {
	req, err := c.newRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *Client) put(ctx context.Context, path string, body, result any) error {
	req, err := c.newRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *Client) patch(ctx context.Context, path string, body, result any) error {
	req, err := c.newRequest(ctx, http.MethodPatch, path, body)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *Client) del(ctx context.Context, path string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}
