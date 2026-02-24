package cpay

import "context"

// CreateSubscription creates a new subscription plan.
func (c *Client) CreateSubscription(ctx context.Context, req *CreateSubscriptionRequest) (*Subscription, error) {
	var s Subscription
	if err := c.post(ctx, "/api/subscriptions", req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSubscriptions lists all subscriptions for the merchant.
func (c *Client) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	var subs []Subscription
	if err := c.get(ctx, "/api/subscriptions", &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

// SetSubscriptionActive activates or deactivates a subscription.
func (c *Client) SetSubscriptionActive(ctx context.Context, id string, active bool) (*Subscription, error) {
	var s Subscription
	body := struct {
		Active bool `json:"active"`
	}{Active: active}
	if err := c.patch(ctx, "/api/subscriptions/"+id+"/active", body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteSubscription deletes a subscription by ID.
func (c *Client) DeleteSubscription(ctx context.Context, id string) error {
	return c.del(ctx, "/api/subscriptions/" + id)
}

// ListSubscriptionPayments lists payments for a specific subscription.
func (c *Client) ListSubscriptionPayments(ctx context.Context, subID string) ([]SubscriptionPayment, error) {
	var payments []SubscriptionPayment
	if err := c.get(ctx, "/api/subscriptions/"+subID+"/payments", &payments); err != nil {
		return nil, err
	}
	return payments, nil
}

// ListSubscribers lists all subscribers for a specific subscription.
func (c *Client) ListSubscribers(ctx context.Context, subID string) ([]Subscriber, error) {
	var subs []Subscriber
	if err := c.get(ctx, "/api/subscriptions/"+subID+"/subscribers", &subs); err != nil {
		return nil, err
	}
	return subs, nil
}
