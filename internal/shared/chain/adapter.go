package chain

import "context"

type DepositWallet struct {
	Address       string
	PrivateKeyHex string
}

type ChainAdapter interface {
	GenerateDepositWallet(ctx context.Context, chain string) (DepositWallet, error)
	RequiredConfirmations(chain string) int
	SupportedChains() []string
}
