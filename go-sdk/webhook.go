package cpay

import "context"

// CreateWebhook creates a new webhook. The signing secret is only returned on creation.
func (c *Client) CreateWebhook(ctx context.Context, req *CreateWebhookRequest) (*CreateWebhookResponse, error) {
	var resp CreateWebhookResponse
	if err := c.post(ctx, "/api/webhooks", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListWebhooks lists all webhooks for the merchant.
func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	var hooks []Webhook
	if err := c.get(ctx, "/api/webhooks", &hooks); err != nil {
		return nil, err
	}
	return hooks, nil
}

// UpdateWebhook updates a webhook by ID.
func (c *Client) UpdateWebhook(ctx context.Context, id string, req *UpdateWebhookRequest) (*Webhook, error) {
	var w Webhook
	if err := c.put(ctx, "/api/webhooks/"+id, req, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// DeleteWebhook deletes a webhook by ID.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.del(ctx, "/api/webhooks/" + id)
}

// ListWebhookDeliveries lists delivery attempts for a specific webhook.
func (c *Client) ListWebhookDeliveries(ctx context.Context, webhookID string) ([]WebhookDelivery, error) {
	var deliveries []WebhookDelivery
	if err := c.get(ctx, "/api/webhooks/"+webhookID+"/deliveries", &deliveries); err != nil {
		return nil, err
	}
	return deliveries, nil
}
