package cpay

import "context"

// RecordWithdrawal records a completed withdrawal transaction.
func (c *Client) RecordWithdrawal(ctx context.Context, req *RecordWithdrawalRequest) (*WithdrawalResponse, error) {
	var resp WithdrawalResponse
	if err := c.post(ctx, "/api/withdrawals", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
