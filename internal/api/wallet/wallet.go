package wallet

import pgwallet "github.com/cpay-dev/backend-go/internal/api/repo/pg/wallet"

type Wallet struct {
	ID        string
	PublicKey string
}

func WalletFromRepo(wallet pgwallet.Wallet) Wallet {
	return Wallet{
		ID:        wallet.ID,
		PublicKey: wallet.PublicKey,
	}
}
