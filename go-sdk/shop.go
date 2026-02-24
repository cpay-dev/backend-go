package cpay

import "context"

// CreateShop creates a new shop for the authenticated merchant.
func (c *Client) CreateShop(ctx context.Context, req *CreateShopRequest) (*Shop, error) {
	var s Shop
	if err := c.post(ctx, "/api/shops", req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetMyShop returns the authenticated merchant's shop.
func (c *Client) GetMyShop(ctx context.Context) (*Shop, error) {
	var s Shop
	if err := c.get(ctx, "/api/shops/me", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateShop updates a shop by ID.
func (c *Client) UpdateShop(ctx context.Context, id string, req *UpdateShopRequest) (*Shop, error) {
	var s Shop
	if err := c.put(ctx, "/api/shops/"+id, req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
