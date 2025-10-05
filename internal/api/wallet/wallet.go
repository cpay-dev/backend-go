package wallet

import pgwallet "github.com/cpay-dev/backend-go/internal/api/repo/pg/wallet"

type Wallet struct {
	PublicKey string
}

func WalletFromRepo(wallet pgwallet.Wallet) Wallet {
	return Wallet{
		PublicKey: wallet.PublicKey,
	}
}
