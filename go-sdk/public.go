package cpay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// GetPublicProduct fetches a public product by ID. No authentication required.
func (c *Client) GetPublicProduct(ctx context.Context, id string) (*PublicProduct, error) {
	var p PublicProduct
	if err := c.get(ctx, "/api/p/"+id, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// CheckProductPayment checks if a payer has already paid for a product.
// Returns nil, nil if no payment is found.
func (c *Client) CheckProductPayment(ctx context.Context, productID, payerAddress string) (*Payment, error) {
	path := fmt.Sprintf("/api/p/%s/payment?payer=%s", productID, url.QueryEscape(payerAddress))
	return c.checkPayment(ctx, path)
}

// RecordProductPayment records a payment for a product. No authentication required.
func (c *Client) RecordProductPayment(ctx context.Context, productID string, req *RecordPaymentRequest) (*Payment, error) {
	var p Payment
	if err := c.post(ctx, "/api/p/"+productID+"/payment", req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPublicPaymentLink fetches a public payment link by ID. No authentication required.
func (c *Client) GetPublicPaymentLink(ctx context.Context, id string) (*PublicPaymentLink, error) {
	var l PublicPaymentLink
	if err := c.get(ctx, "/api/pay/"+id, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// CheckPaymentLinkPayment checks if a payer has already used a payment link.
// Returns nil, nil if no payment is found.
func (c *Client) CheckPaymentLinkPayment(ctx context.Context, linkID, payerAddress string) (*Payment, error) {
	path := fmt.Sprintf("/api/pay/%s/payment?payer=%s", linkID, url.QueryEscape(payerAddress))
	return c.checkPayment(ctx, path)
}

// RecordPaymentLinkPayment records a payment for a payment link. No authentication required.
func (c *Client) RecordPaymentLinkPayment(ctx context.Context, linkID string, req *RecordPaymentRequest) (*Payment, error) {
	var p Payment
	if err := c.post(ctx, "/api/pay/"+linkID+"/payment", req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPublicSubscription fetches a public subscription by ID. No authentication required.
func (c *Client) GetPublicSubscription(ctx context.Context, id string) (*PublicSubscription, error) {
	var s PublicSubscription
	if err := c.get(ctx, "/api/sub/"+id, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// CheckSubscriptionPayment checks if a payer has already paid a subscription.
// Returns nil, nil if no payment is found.
func (c *Client) CheckSubscriptionPayment(ctx context.Context, subID, payerAddress string) (*SubscriptionPayment, error) {
	path := fmt.Sprintf("/api/sub/%s/payment?payer=%s", subID, url.QueryEscape(payerAddress))
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cpay: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr Error
		apiErr.StatusCode = resp.StatusCode
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			apiErr.Message = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		}
		return nil, &apiErr
	}

	var payment *SubscriptionPayment
	if err := json.NewDecoder(resp.Body).Decode(&payment); err != nil {
		return nil, fmt.Errorf("cpay: decode response: %w", err)
	}
	return payment, nil
}

// RecordSubscriptionPayment records a subscription payment. No authentication required.
func (c *Client) RecordSubscriptionPayment(ctx context.Context, subID string, req *RecordSubscriptionPaymentRequest) (*SubscriptionPayment, error) {
	var p SubscriptionPayment
	if err := c.post(ctx, "/api/sub/"+subID+"/payment", req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateSubscriber creates a new subscriber for a subscription. No authentication required.
func (c *Client) CreateSubscriber(ctx context.Context, subID string, req *CreateSubscriberRequest) (*Subscriber, error) {
	var s Subscriber
	if err := c.post(ctx, "/api/sub/"+subID+"/subscribe", req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// CancelSubscriber cancels a subscriber. No authentication required.
func (c *Client) CancelSubscriber(ctx context.Context, subID string, req *CancelSubscriberRequest) (*Subscriber, error) {
	var s Subscriber
	if err := c.post(ctx, "/api/sub/"+subID+"/cancel", req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSubscriber fetches a subscriber by subscription ID and payer address.
// Returns nil, nil if no subscriber is found.
func (c *Client) GetSubscriber(ctx context.Context, subID, payerAddress string) (*Subscriber, error) {
	path := fmt.Sprintf("/api/sub/%s/subscriber?payer=%s", subID, url.QueryEscape(payerAddress))
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cpay: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr Error
		apiErr.StatusCode = resp.StatusCode
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			apiErr.Message = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		}
		return nil, &apiErr
	}

	var subscriber *Subscriber
	if err := json.NewDecoder(resp.Body).Decode(&subscriber); err != nil {
		return nil, fmt.Errorf("cpay: decode response: %w", err)
	}
	return subscriber, nil
}

// GetPublicInvoice fetches a public invoice by ID. No authentication required.
func (c *Client) GetPublicInvoice(ctx context.Context, id string) (*PublicInvoice, error) {
	var inv PublicInvoice
	if err := c.get(ctx, "/api/inv/"+id, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// RecordInvoicePayment records a payment for an invoice. No authentication required.
func (c *Client) RecordInvoicePayment(ctx context.Context, invoiceID string, req *RecordInvoicePaymentRequest) (*Invoice, error) {
	var inv Invoice
	if err := c.post(ctx, "/api/inv/"+invoiceID+"/payment", req, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// GetRelayerAddress returns the relayer contract address. No authentication required.
func (c *Client) GetRelayerAddress(ctx context.Context) (string, error) {
	var resp struct {
		Address string `json:"address"`
	}
	if err := c.get(ctx, "/api/relayer-address", &resp); err != nil {
		return "", err
	}
	return resp.Address, nil
}

// GetCFAddress returns the counterfactual account address for the authenticated merchant.
func (c *Client) GetCFAddress(ctx context.Context) (string, error) {
	var resp struct {
		Address string `json:"address"`
	}
	if err := c.get(ctx, "/api/cf-address", &resp); err != nil {
		return "", err
	}
	return resp.Address, nil
}

// GetCFBalance returns the token balance for the merchant's counterfactual account.
// Pass an empty string for token to use the default (USDC).
func (c *Client) GetCFBalance(ctx context.Context, token string) (*CFBalanceResponse, error) {
	path := "/api/cf-balance"
	if token != "" {
		path += "?token=" + url.QueryEscape(token)
	}
	var resp CFBalanceResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetCFWithdrawInfo returns CF wallet info including deployment status.
// Pass an empty string for token to use the default (USDC).
func (c *Client) GetCFWithdrawInfo(ctx context.Context, token string) (*CFWithdrawInfoResponse, error) {
	path := "/api/cf-withdraw-info"
	if token != "" {
		path += "?token=" + url.QueryEscape(token)
	}
	var resp CFWithdrawInfoResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// checkPayment is a helper for check-payment endpoints that return null when not found.
func (c *Client) checkPayment(ctx context.Context, path string) (*Payment, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cpay: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr Error
		apiErr.StatusCode = resp.StatusCode
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			apiErr.Message = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		}
		return nil, &apiErr
	}

	var payment *Payment
	if err := json.NewDecoder(resp.Body).Decode(&payment); err != nil {
		return nil, fmt.Errorf("cpay: decode response: %w", err)
	}
	return payment, nil // payment may be nil if backend returned null
}
