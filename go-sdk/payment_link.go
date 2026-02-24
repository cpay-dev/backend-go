package cpay

import "context"

// CreatePaymentLink creates a new payment link.
func (c *Client) CreatePaymentLink(ctx context.Context, req *CreatePaymentLinkRequest) (*PaymentLink, error) {
	var l PaymentLink
	if err := c.post(ctx, "/api/payment-links", req, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// ListPaymentLinks lists all payment links for the merchant.
func (c *Client) ListPaymentLinks(ctx context.Context) ([]PaymentLink, error) {
	var links []PaymentLink
	if err := c.get(ctx, "/api/payment-links", &links); err != nil {
		return nil, err
	}
	return links, nil
}

// SetPaymentLinkActive activates or deactivates a payment link.
func (c *Client) SetPaymentLinkActive(ctx context.Context, id string, active bool) error {
	body := struct {
		Active bool `json:"active"`
	}{Active: active}
	return c.patch(ctx, "/api/payment-links/"+id+"/active", body, nil)
}

// DeletePaymentLink deletes a payment link by ID.
func (c *Client) DeletePaymentLink(ctx context.Context, id string) error {
	return c.del(ctx, "/api/payment-links/" + id)
}
