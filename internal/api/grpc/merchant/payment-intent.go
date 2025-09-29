package merchant

import (
	"context"
	"fmt"
	"time"

	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/model"
	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	"github.com/cpay-dev/backend-go/pkg/tokenmath"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) CreatePaymentIntent(ctx context.Context, req *pbmerchant.CreatePaymentIntentRequest) (*pbmerchant.CreatePaymentIntentResponse, error) {
	asset, err := s.blockchainRepo.GetAsset(ctx, req.AssetId)
	if err != nil {
		return nil, fmt.Errorf("get asset: %w", err)
	}
	if asset == nil {
		return nil, status.Error(codes.NotFound, "asset not found")
	}
	assetMetadata, err := model.UnmarshalMetadata(asset.Metadata)
	if err != nil {
		return nil, fmt.Errorf("unmarshal asset metadata: %w", err)
	}

	assetPriceResp, err := s.GetAssetPrice(ctx, &pbmerchant.GetAssetPriceRequest{AssetId: asset.ID})
	if err != nil {
		return nil, fmt.Errorf("get asset price: %w", err)
	}

	merchant := MustGetMerchant(ctx)

	paymentIntent := pgpayment.Intent{
		MerchantID: merchant.ID,
		AssetID:    asset.ID,
		Status:     pgpayment.IntentStatusAwaitingPayment,
	}

	switch amount := req.Amount.(type) {
	case *pbmerchant.CreatePaymentIntentRequest_AmountUsd:
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
		amountAsset, err := tokenmath.TokensFromUSD_Ceil(amount.AmountUsd, assetPriceResp.Price, uint(assetMetadata.Decimals))
		if err != nil {
			return nil, fmt.Errorf("compute amount asset: %w (%s/%s, %d)", err, amount.AmountUsd, assetPriceResp.Price, assetMetadata.Decimals)
		}
		paymentIntent.AmountAsset = amountAsset.Dec()
	case *pbmerchant.CreatePaymentIntentRequest_AmountAsset:
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
		amountUSD, err := tokenmath.ValueUSD_ScaledFloor(amount.AmountAsset, assetPriceResp.Price, 2)
		if err != nil {
			return nil, fmt.Errorf("compute amount USD: %w (%s*%s, 2)", err, amount.AmountAsset, assetPriceResp.Price)
		}
		paymentIntent.AmountUSD = amountUSD.Dec()
	}

	if err = s.paymentRepo.CreateIntent(ctx, paymentIntent); err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
	}

	intentpb, err := model.PaymentIntentToProto(paymentIntent)
	if err != nil {
		return nil, fmt.Errorf("map payment intent to proto: %w", err)
	}
	intentpb.CreatedAt = timestamppb.New(time.Now())
	intentpb.UpdatedAt = intentpb.CreatedAt

	return &pbmerchant.CreatePaymentIntentResponse{PaymentIntent: intentpb}, nil
}
