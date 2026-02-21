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

// CFWalletInfo holds info returned by DeployAndWithdraw for frontend use.
type CFWalletInfo struct {
	CFAddress  string // CF wallet address
	Balance    string // raw token balance (U256 decimal string)
	IsDeployed bool   // whether MinimalWallet is already deployed at CFAddress
}

// GetCFWalletInfo returns the CF wallet address, token balance, and deployment status.
// The actual deploy+withdraw transactions are signed by the merchant in the frontend.
func (c *ChainClient) GetCFWalletInfo(ctx context.Context, chainID uint64, merchantWallet, tokenAddress string) (*CFWalletInfo, error) {
	resp, err := c.client.DeployAndWithdraw(ctx, &chainv1.DeployAndWithdrawRequest{
		ChainId:        chainID,
		MerchantWallet: merchantWallet,
		TokenAddress:   tokenAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("GetCFWalletInfo: %w", err)
	}
	// resp.TxHash holds the CF address, resp.Amount holds "balance:deployed|not_deployed"
	balance, deployStatus := splitLast(resp.Amount, ':')
	return &CFWalletInfo{
		CFAddress:  resp.TxHash,
		Balance:    balance,
		IsDeployed: deployStatus == "deployed",
	}, nil
}

// VerifyTransaction checks an on-chain tx receipt and returns whether it succeeded.
func (c *ChainClient) VerifyTransaction(ctx context.Context, chainID uint64, txHash string) (success bool, from, to string, err error) {
	resp, err := c.client.VerifyTransaction(ctx, &chainv1.VerifyTransactionRequest{
		ChainId: chainID,
		TxHash:  txHash,
	})
	if err != nil {
		return false, "", "", fmt.Errorf("VerifyTransaction: %w", err)
	}
	return resp.Success, resp.FromAddress, resp.ToAddress, nil
}

// CheckAllowance returns the ERC-20 allowance(owner, spender) as a decimal string.
func (c *ChainClient) CheckAllowance(ctx context.Context, chainID uint64, tokenAddress, owner, spender string) (string, error) {
	resp, err := c.client.CheckAllowance(ctx, &chainv1.CheckAllowanceRequest{
		ChainId:      chainID,
		TokenAddress: tokenAddress,
		Owner:        owner,
		Spender:      spender,
	})
	if err != nil {
		return "", fmt.Errorf("CheckAllowance: %w", err)
	}
	return resp.Allowance, nil
}

// ExecuteTransferFrom signs and submits a transferFrom via the relayer. Returns tx hash.
func (c *ChainClient) ExecuteTransferFrom(ctx context.Context, chainID uint64, tokenAddress, from, to, amount string) (string, error) {
	resp, err := c.client.ExecuteTransferFrom(ctx, &chainv1.ExecuteTransferFromRequest{
		ChainId:      chainID,
		TokenAddress: tokenAddress,
		From:         from,
		To:           to,
		Amount:       amount,
	})
	if err != nil {
		return "", fmt.Errorf("ExecuteTransferFrom: %w", err)
	}
	return resp.TxHash, nil
}

// GetRelayerAddress returns the relayer EOA address (for frontend approve target).
func (c *ChainClient) GetRelayerAddress(ctx context.Context) (string, error) {
	resp, err := c.client.GetRelayerAddress(ctx, &chainv1.GetRelayerAddressRequest{})
	if err != nil {
		return "", fmt.Errorf("GetRelayerAddress: %w", err)
	}
	return resp.Address, nil
}

func splitLast(s string, sep byte) (string, string) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
