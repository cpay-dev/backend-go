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
	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	"github.com/cpay-dev/backend-go/pkg/tokenmath"
	pbpayment "github.com/cpay-dev/proto-go/api/v1/merchant/payment"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) CreateIntent(ctx context.Context, req *pbpayment.CreateIntentRequest) (*pbpayment.CreateIntentResponse, error) {
	asset, err := s.blockchainRepo.GetAsset(ctx, req.AssetId)
	if err != nil {
		return nil, fmt.Errorf("get asset: %w", err)
	}
	if asset == nil {
		return nil, status.Error(codes.NotFound, "asset not found")
	}
	assetMetadata, err := assetmodel.UnmarshalMetadata(asset.Metadata)
	if err != nil {
		return nil, fmt.Errorf("unmarshal asset metadata: %w", err)
	}

	assetPrice, err := s.priceService.GetPrice(ctx, asset.ID)
	if errors.Is(err, apiasset.ErrAssetNotFound) {
		return nil, status.Error(codes.NotFound, "asset not found")
	} else if errors.Is(err, apiasset.ErrPriceUnknown) {
		return nil, status.Error(codes.Internal, "asset price unknown")
	} else if err != nil {
		return nil, fmt.Errorf("get price: %w", err)
	}

	merchant := middleware.MustGetMerchant(ctx)

	paymentIntent := pgpayment.Intent{
		MerchantID: merchant.ID,
		AssetID:    asset.ID,
		Status:     pgpayment.IntentStatusAwaitingPayment,
	}

	switch amount := req.Amount.(type) {
	case *pbpayment.CreateIntentRequest_AmountUsd:
		if amount.AmountUsd == "" {
			return nil, status.Error(codes.InvalidArgument, "amount should not be empty")
		}
		amountMant, dp, err := tokenmath.ParseDecimal(amount.AmountUsd)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "amount should be a valid decimal")
		}
		if dp != 2 {
			return nil, status.Error(codes.InvalidArgument, "amount should have 2 decimal places")
		}
		if !amountMant.IsUint64() {
			return nil, status.Error(codes.InvalidArgument, "amount should be a valid decimal")
		}
		if amountMant.IsZero() {
			return nil, status.Error(codes.InvalidArgument, "amount should be greater than 0")
		}
		paymentIntent.AmountUSD = amountMant.Dec()
		amountAsset, err := tokenmath.TokensFromUSD_Ceil(amount.AmountUsd, assetPrice, uint(assetMetadata.Decimals))
		if err != nil {
			return nil, fmt.Errorf("compute amount asset: %w (%s/%s, %d)", err, amount.AmountUsd, assetPrice, assetMetadata.Decimals)
		}
		paymentIntent.AmountAsset = amountAsset.Dec()
	case *pbpayment.CreateIntentRequest_AmountAsset:
		if amount.AmountAsset == "" {
			return nil, status.Error(codes.InvalidArgument, "amount should not be empty")
		}
		amountMant, dp, err := tokenmath.ParseDecimal(amount.AmountAsset)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "amount should be a valid decimal")
		}
		if dp > uint(assetMetadata.Decimals) {
			return nil, status.Error(codes.InvalidArgument, "amount should have no more decimal places than the asset")
		}
		if amountMant.IsZero() {
			return nil, status.Error(codes.InvalidArgument, "amount should be greater than 0")
		}
		paymentIntent.AmountAsset = amountMant.Dec()
		amountUSD, err := tokenmath.ValueUSD_ScaledFloor(amount.AmountAsset, assetPrice, 2)
		if err != nil {
			return nil, fmt.Errorf("compute amount USD: %w (%s*%s, 2)", err, amount.AmountAsset, assetPrice)
		}
		paymentIntent.AmountUSD = amountUSD.Dec()
	}

	if err = s.paymentRepo.CreateIntent(ctx, paymentIntent); err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
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
	return &pbpayment.GetIntentResponse{Intent: nil}, nil
}
