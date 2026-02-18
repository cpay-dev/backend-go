package grpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	chainv1 "github.com/cpay-dev/backend/internal/grpc/gen/chainv1/proto/chain/v1"
)

type ChainClient struct {
	conn   *grpc.ClientConn
	client chainv1.ChainServiceClient
}

func NewChainClient(addr string) (*ChainClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("connect to chain service: %w", err)
	}
	return &ChainClient{
		conn:   conn,
		client: chainv1.NewChainServiceClient(conn),
	}, nil
}

func (c *ChainClient) Conn() *grpc.ClientConn {
	return c.conn
}

func (c *ChainClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// GetCounterfactualAddress derives the CREATE2 counterfactual address for a merchant.
func (c *ChainClient) GetCounterfactualAddress(ctx context.Context, chainID uint64, merchantWallet string) (string, error) {
	resp, err := c.client.GetCounterfactualAddress(ctx, &chainv1.GetCounterfactualAddressRequest{
		ChainId:        chainID,
		MerchantWallet: merchantWallet,
	})
	if err != nil {
		return "", fmt.Errorf("GetCounterfactualAddress: %w", err)
	}
	return resp.Address, nil
}

// GetTokenBalance returns the ERC-20 token balance of walletAddress as a decimal string.
func (c *ChainClient) GetTokenBalance(ctx context.Context, chainID uint64, tokenAddress, walletAddress string) (string, error) {
	resp, err := c.client.GetTokenBalance(ctx, &chainv1.GetTokenBalanceRequest{
		ChainId:       chainID,
		TokenAddress:  tokenAddress,
		WalletAddress: walletAddress,
	})
	if err != nil {
		return "", fmt.Errorf("GetTokenBalance: %w", err)
	}
	return resp.Balance, nil
}
