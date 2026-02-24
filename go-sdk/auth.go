package cpay

import "context"

// GetNonce fetches a fresh SIWE nonce. No authentication required.
func (c *Client) GetNonce(ctx context.Context) (string, error) {
	var resp struct {
		Nonce string `json:"nonce"`
	}
	if err := c.post(ctx, "/api/auth/siwe/nonce", nil, &resp); err != nil {
		return "", err
	}
	return resp.Nonce, nil
}

// Verify verifies a SIWE message and signature, returning a JWT token and user.
// After success, call c.SetToken(resp.Token) to authenticate subsequent requests.
func (c *Client) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	var resp VerifyResponse
	if err := c.post(ctx, "/api/auth/siwe/verify", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
