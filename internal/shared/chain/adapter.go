package chain

import "context"

type DepositWallet struct {
	Address        string
	PrivateKeyHex  string
	WalletType     string
	FactoryAddress string
	WalletSalt     string
	InitCodeHash   string
}

type ChainAdapter interface {
	GenerateDepositWallet(ctx context.Context, chain string, paymentIntentID string) (DepositWallet, error)
	RequiredConfirmations(chain string) int
	SupportedChains() []string
}
