package wallet

import (
	"context"
	"fmt"

	chainmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain/model"
	blockchainmodel "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	pgwallet "github.com/cpay-dev/backend-go/internal/api/repo/pg/wallet"
	pbwallet "github.com/cpay-dev/proto-go/api/v1/wallet"
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

func (s *Service) GetWallet(ctx context.Context, chainId blockchainmodel.Chain) (*Wallet, error) {
	chain, err := chainmodel.ChainToProto(chainId)
	if err != nil {
		return nil, fmt.Errorf("convert chain: %w", err)
	}

	acquiredWallet, err := s.walletRepo.AcquireAvailableWallet(ctx, chainId)
	if err != nil {
		return nil, fmt.Errorf("acquire wallet: %w", err)
	}

	if acquiredWallet == nil {
		createWalletResp, err := s.walletClient.CreateWallet(ctx, &pbwallet.CreateWalletRequest{Chain: chain})
		if err != nil {
			return nil, fmt.Errorf("create wallet via grpc: %w", err)
		}

		newWallet := pgwallet.Wallet{
			ChainID:             chainId,
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
