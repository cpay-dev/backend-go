package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	assetmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset/model"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/middleware"
	paymentmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/payment/model"
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	"github.com/cpay-dev/backend-go/pkg/tokenmath"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
	pbpayment "github.com/cpay-dev/proto-go/api/v1/merchant/payment"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) CreateIntent(ctx context.Context, req *pbpayment.CreateIntentRequest) (*pbpayment.CreateIntentResponse, error) {
	asset, assetPrice, assetMetadata, err := s.parseCreateIntentAsset(ctx, req.AssetId)
	if err != nil {
		return nil, err
	}

	amountUSD, amountAsset, err := s.parseCreateIntentAmount(req, assetPrice, assetMetadata)
	if err != nil {
		return nil, err
	}

	wallet, err := s.walletService.GetWallet(ctx, asset.ChainID)
	if err != nil {
		return nil, fmt.Errorf("get wallet: %w", err)
	}

	merchant := middleware.MustGetMerchant(ctx)

	paymentIntent := pgpayment.Intent{
		MerchantID:  merchant.ID,
		AssetID:     req.AssetId,
		Status:      pgpayment.IntentStatusAwaitingPayment,
		AmountUSD:   amountUSD,
		AmountAsset: amountAsset,
	}

	err = s.paymentRepo.RunInTx(ctx, func(ctx context.Context) error {
		if err = s.paymentRepo.CreateIntent(ctx, paymentIntent); err != nil {
			return fmt.Errorf("create payment intent: %w", err)
		}
		intentWallet := pgpayment.IntentWallet{
			IntentID:           paymentIntent.ID,
			WalletID:           wallet.ID,
			WalletType:         pgpayment.IntentWalletTypeCustodial,
			WalletAssetAddress: wallet.PublicKey,
		}
		if err = s.paymentRepo.CreateIntentWallet(ctx, intentWallet); err != nil {
			return fmt.Errorf("create payment intent wallet: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("save payment intent: %w", err)
	}

	intentpb, err := paymentmodel.IntentToProto(paymentIntent)
	if err != nil {
		return nil, fmt.Errorf("map payment intent to proto: %w", err)
	}
	intentpb.CreatedAt = timestamppb.New(time.Now())
	intentpb.UpdatedAt = intentpb.CreatedAt

	return &pbpayment.CreateIntentResponse{Intent: intentpb}, nil
}

func (s *Service) GetIntent(ctx context.Context, req *pbpayment.GetIntentRequest) (*pbpayment.GetIntentResponse, error) {
	merchant := middleware.MustGetMerchant(ctx)

	intent, err := s.paymentRepo.GetIntent(ctx, req.Id, merchant.ID)
	if err != nil {
		return nil, fmt.Errorf("get intent: %w", err)
	}
	if intent == nil {
		return nil, status.Error(codes.NotFound, "intent not found")
	}

	intentpb, err := paymentmodel.IntentToProto(*intent)
	if err != nil {
		return nil, fmt.Errorf("map payment intent to proto: %w", err)
	}

	return &pbpayment.GetIntentResponse{Intent: intentpb}, nil
}

func (s *Service) parseCreateIntentAsset(ctx context.Context, assetID string) (asset *pgblockchain.Asset, price string, metadata *pbasset.AssetMetadata, err error) {
	asset, err = s.blockchainRepo.GetAsset(ctx, assetID)
	if err != nil {
		return nil, "", nil, fmt.Errorf("get asset: %w", err)
	} else if asset == nil {
		return nil, "", nil, status.Error(codes.NotFound, "asset not found")
	}

	assetMetadata, err := assetmodel.UnmarshalMetadata(asset.Metadata)
	if err != nil {
		return nil, "", nil, fmt.Errorf("unmarshal asset metadata: %w", err)
	}

	assetPrice, err := s.priceService.GetPrice(ctx, asset.ID)
	if errors.Is(err, apiasset.ErrAssetNotFound) {
		return nil, "", nil, status.Error(codes.NotFound, "asset not found")
	} else if errors.Is(err, apiasset.ErrPriceUnknown) {
		return nil, "", nil, status.Error(codes.Internal, "asset price unknown")
	} else if err != nil {
		return nil, "", nil, fmt.Errorf("get price: %w", err)
	}

	return asset, assetPrice, assetMetadata, nil
}

func (s *Service) parseCreateIntentAmount(req *pbpayment.CreateIntentRequest, assetPrice string, assetMetadata *pbasset.AssetMetadata) (amountUSD string, amountAsset string, err error) {
	switch amount := req.Amount.(type) {
	case *pbpayment.CreateIntentRequest_AmountUsd:
		if amount.AmountUsd == "" {
			return "", "", status.Error(codes.InvalidArgument, "amount should not be empty")
		}
		amountMant, dp, err := tokenmath.ParseDecimal(amount.AmountUsd)
		if err != nil {
			return "", "", status.Error(codes.InvalidArgument, "amount should be a valid decimal")
		}
		if dp != 2 {
			return "", "", status.Error(codes.InvalidArgument, "amount should have 2 decimal places")
		}
		if !amountMant.IsUint64() {
			return "", "", status.Error(codes.InvalidArgument, "amount should be a valid decimal")
		}
		if amountMant.IsZero() {
			return "", "", status.Error(codes.InvalidArgument, "amount should be greater than 0")
		}
		amountUSD := amountMant.Dec()
		amountAsset, err := tokenmath.TokensFromUSD_Ceil(amount.AmountUsd, assetPrice, uint(assetMetadata.Decimals))
		if err != nil {
			return "", "", fmt.Errorf("compute amount asset: %w (%s/%s, %d)", err, amount.AmountUsd, assetPrice, assetMetadata.Decimals)
		}
		return amountUSD, amountAsset.Dec(), nil
	case *pbpayment.CreateIntentRequest_AmountAsset:
		if amount.AmountAsset == "" {
			return "", "", status.Error(codes.InvalidArgument, "amount should not be empty")
		}
		amountMant, dp, err := tokenmath.ParseDecimal(amount.AmountAsset)
		if err != nil {
			return "", "", status.Error(codes.InvalidArgument, "amount should be a valid decimal")
		}
		if dp > uint(assetMetadata.Decimals) {
			return "", "", status.Error(codes.InvalidArgument, "amount should have no more decimal places than the asset")
		}
		if amountMant.IsZero() {
			return "", "", status.Error(codes.InvalidArgument, "amount should be greater than 0")
		}
		amountAsset := amountMant.Dec()
		amountUSD, err := tokenmath.ValueUSD_ScaledFloor(amount.AmountAsset, assetPrice, 2)
		if err != nil {
			return "", "", fmt.Errorf("compute amount USD: %w (%s*%s, 2)", err, amount.AmountAsset, assetPrice)
		}
		return amountUSD.Dec(), amountAsset, nil
	default:
		return "", "", status.Error(codes.InvalidArgument, "amount should be either amount_usd or amount_asset")
	}
}
