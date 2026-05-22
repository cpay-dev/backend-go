//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func TestKnownEVMTransactionsHaveConfirmations(t *testing.T) {
	cases := []struct {
		name         string
		rpcEnv       string
		rpcURLs      []string
		fallbackOnRL bool
		txHash       string
	}{
		{
			name:         "hyperevm",
			rpcEnv:       "HYPEREVM_RPC_URL",
			rpcURLs:      []string{"https://rpc.hypurrscan.io", "https://hyperliquid.drpc.org"},
			fallbackOnRL: true,
			txHash:       "0x8d375ef3e3097e9fd7950f86ca1b3a3a6d05b8ec501b8a51a0c3b7197f074825",
		},
		{
			name:    "arbitrum",
			rpcEnv:  "ARBITRUM_RPC_URL",
			rpcURLs: []string{"https://arb1.arbitrum.io/rpc"},
			txHash:  "0xe94b2d9d0dfd8f148d40c5f33014844e4f8b582a09cd4aaad74d39ca7b0886c3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rpcURLs := tc.rpcURLs
			if override := os.Getenv(tc.rpcEnv); override != "" {
				rpcURLs = []string{override}
			}
			assertRPCConfirmations(t, rpcURLs, tc.txHash, tc.fallbackOnRL)
		})
	}
}

func assertRPCConfirmations(t *testing.T, rpcURLs []string, txHash string, fallbackOnRateLimit bool) {
	t.Helper()

	var lastErr error
	for index, rpcURL := range rpcURLs {
		err := rpcConfirmations(rpcURL, txHash)
		if err == nil {
			if index > 0 {
				t.Logf("used fallback rpc %q", rpcURL)
			}
			return
		}
		lastErr = err
		if !fallbackOnRateLimit || !transientRPCError(err) || index == len(rpcURLs)-1 {
			break
		}
		t.Logf("rpc %q returned transient error, trying fallback: %v", rpcURL, err)
	}
	t.Fatalf("expected confirmations > 0 for %s: %v", txHash, lastErr)
}

func rpcConfirmations(rpcURL string, txHash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return fmt.Errorf("dial rpc %q: %w", rpcURL, err)
	}
	defer client.Close()

	var receiptBlock uint64
	for attempt := 0; attempt < 5; attempt++ {
		receipt, receiptErr := client.TransactionReceipt(ctx, common.HexToHash(txHash))
		if receiptErr == nil {
			if receipt.BlockNumber == nil {
				return fmt.Errorf("receipt for %s has no block number", txHash)
			}
			err = nil
			receiptBlock = receipt.BlockNumber.Uint64()
			break
		}
		err = receiptErr
		if !transientRPCError(receiptErr) {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fetch receipt for %s timed out: %w", txHash, ctx.Err())
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	if err != nil {
		return fmt.Errorf("fetch receipt for %s: %w", txHash, err)
	}

	var head uint64
	for attempt := 0; attempt < 5; attempt++ {
		head, err = client.BlockNumber(ctx)
		if err == nil {
			break
		}
		if !transientRPCError(err) {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fetch latest block timed out: %w", ctx.Err())
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	if err != nil {
		return fmt.Errorf("fetch latest block: %w", err)
	}

	if head < receiptBlock {
		return fmt.Errorf("latest block %d is behind receipt block %d", head, receiptBlock)
	}
	confirmations := head - receiptBlock + 1
	if confirmations <= 0 {
		return fmt.Errorf("got %d confirmations", confirmations)
	}
	return nil
}

func transientRPCError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "rate limit") ||
		strings.Contains(message, "rate limited") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "timeout") ||
		strings.Contains(message, "temporarily unavailable")
}
