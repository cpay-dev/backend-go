package cpay

import "context"

// ListPayments lists all payments (products, links, subscriptions) for the merchant's shop.
func (c *Client) ListPayments(ctx context.Context) ([]Payment, error) {
	var payments []Payment
	if err := c.get(ctx, "/api/payments", &payments); err != nil {
		return nil, err
	}
	return payments, nil
}
