package wallet

import (
	"context"
	"fmt"

	chainmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain/model"
	pgwallet "github.com/cpay-dev/backend-go/internal/api/repo/pg/wallet"
	pbwallet "github.com/cpay-dev/proto-go/api/v1/wallet"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
)

type Service struct {
	walletRepo   *pgwallet.PostgresRepo
	walletClient pbwallet.WalletServiceClient
}

func NewService(walletRepo *pgwallet.PostgresRepo, walletClient pbwallet.WalletServiceClient) *Service {
	return &Service{
		walletRepo:   walletRepo,
		walletClient: walletClient,
	}
}

func (s *Service) GetWallet(ctx context.Context, chain pbblockchain.Chain) (*Wallet, error) {
	chainID, err := chainmodel.ChainToRepo(chain)
	if err != nil {
		return nil, fmt.Errorf("convert chain: %w", err)
	}

	acquiredWallet, err := s.walletRepo.AcquireAvailableWallet(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("acquire wallet: %w", err)
	}

	if acquiredWallet == nil {
		createWalletResp, err := s.walletClient.CreateWallet(ctx, &pbwallet.CreateWalletRequest{Chain: chain})
		if err != nil {
			return nil, fmt.Errorf("create wallet via grpc: %w", err)
		}

		newWallet := pgwallet.Wallet{
			ChainID:             chainID,
			Status:              pgwallet.WalletStatusInUse,
			KekVersion:          createWalletResp.KekVersion,
			PublicKey:           createWalletResp.PublicKey,
			EncryptedPrivateKey: createWalletResp.EncryptedPrivateKey,
		}

		if err = s.walletRepo.CreateWallet(ctx, newWallet); err != nil {
			return nil, fmt.Errorf("save wallet: %w", err)
		}

		acquiredWallet = &newWallet
	}

	wallet := WalletFromRepo(*acquiredWallet)
	return &wallet, nil
}
