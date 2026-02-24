package cpay

import "context"

// GetProfile returns the authenticated user's profile.
func (c *Client) GetProfile(ctx context.Context) (*User, error) {
	var u User
	if err := c.get(ctx, "/api/user/profile", &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// SetRole sets the user's role ("merchant" or "user"). Returns a new JWT token.
// This can only be called once per user; subsequent calls return a 409 Conflict.
func (c *Client) SetRole(ctx context.Context, role string) (*VerifyResponse, error) {
	var resp VerifyResponse
	body := struct {
		Role string `json:"role"`
	}{Role: role}
	if err := c.put(ctx, "/api/user/role", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateEmail updates the authenticated user's email address.
func (c *Client) UpdateEmail(ctx context.Context, email string) (*User, error) {
	var u User
	body := struct {
		Email string `json:"email"`
	}{Email: email}
	if err := c.put(ctx, "/api/user/email", body, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
