package swap

import "context"

type QuoteRequest struct {
	Chain           string
	FromToken       string
	ToToken         string
	FromAmount      string
	SlippageBps     int
	MerchantID      string
	PaymentIntentID string
}

type QuoteResponse struct {
	QuoteID   string
	ToAmount  string
	RouteHint string
	ExpiresAt int64
}

type ExecutionResponse struct {
	ExecutionID string
	Status      string
	TxHash      string
}

type Adapter interface {
	Quote(ctx context.Context, req QuoteRequest) (QuoteResponse, error)
	Execute(ctx context.Context, quoteID string) (ExecutionResponse, error)
	Status(ctx context.Context, executionID string) (ExecutionResponse, error)
}
