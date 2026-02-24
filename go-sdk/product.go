package cpay

import "context"

// CreateProduct creates a new product.
func (c *Client) CreateProduct(ctx context.Context, req *CreateProductRequest) (*Product, error) {
	var p Product
	if err := c.post(ctx, "/api/products", req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProducts lists all products for the authenticated merchant.
func (c *Client) ListProducts(ctx context.Context) ([]Product, error) {
	var products []Product
	if err := c.get(ctx, "/api/products", &products); err != nil {
		return nil, err
	}
	return products, nil
}

// GetProduct returns a product by ID.
func (c *Client) GetProduct(ctx context.Context, id string) (*Product, error) {
	var p Product
	if err := c.get(ctx, "/api/products/"+id, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProduct updates a product by ID.
func (c *Client) UpdateProduct(ctx context.Context, id string, req *UpdateProductRequest) (*Product, error) {
	var p Product
	if err := c.put(ctx, "/api/products/"+id, req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// SetProductActive activates or deactivates a product.
func (c *Client) SetProductActive(ctx context.Context, id string, active bool) error {
	body := struct {
		Active bool `json:"active"`
	}{Active: active}
	return c.patch(ctx, "/api/products/"+id+"/active", body, nil)
}

// DeleteProduct deletes a product by ID.
func (c *Client) DeleteProduct(ctx context.Context, id string) error {
	return c.del(ctx, "/api/products/"+id)
}

// ListProductPayments lists payments for a specific product.
func (c *Client) ListProductPayments(ctx context.Context, productID string) ([]Payment, error) {
	var payments []Payment
	if err := c.get(ctx, "/api/products/"+productID+"/payments", &payments); err != nil {
		return nil, err
	}
	return payments, nil
}
