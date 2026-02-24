package cpay

import "context"

// CreateInvoice creates a new invoice.
func (c *Client) CreateInvoice(ctx context.Context, req *CreateInvoiceRequest) (*Invoice, error) {
	var inv Invoice
	if err := c.post(ctx, "/api/invoices", req, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// ListInvoices lists all invoices for the authenticated merchant.
func (c *Client) ListInvoices(ctx context.Context) ([]Invoice, error) {
	var invoices []Invoice
	if err := c.get(ctx, "/api/invoices", &invoices); err != nil {
		return nil, err
	}
	return invoices, nil
}

// GetInvoice returns an invoice by ID.
func (c *Client) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	var inv Invoice
	if err := c.get(ctx, "/api/invoices/"+id, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// UpdateInvoice updates an invoice by ID (only draft invoices).
func (c *Client) UpdateInvoice(ctx context.Context, id string, req *UpdateInvoiceRequest) (*Invoice, error) {
	var inv Invoice
	if err := c.put(ctx, "/api/invoices/"+id, req, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// SendInvoice transitions an invoice from draft to sent.
func (c *Client) SendInvoice(ctx context.Context, id string) (*Invoice, error) {
	var inv Invoice
	if err := c.post(ctx, "/api/invoices/"+id+"/send", nil, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// CancelInvoice transitions an invoice from sent to cancelled.
func (c *Client) CancelInvoice(ctx context.Context, id string) (*Invoice, error) {
	var inv Invoice
	if err := c.post(ctx, "/api/invoices/"+id+"/cancel", nil, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// DeleteInvoice deletes an invoice by ID (only draft invoices).
func (c *Client) DeleteInvoice(ctx context.Context, id string) error {
	return c.del(ctx, "/api/invoices/"+id)
}
