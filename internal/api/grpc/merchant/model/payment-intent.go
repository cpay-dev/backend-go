package model

import (
	"fmt"

	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	pbapimerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func PaymentIntentToProto(intent pgpayment.Intent) (*pbapimerchant.PaymentIntent, error) {
	status, err := PaymentIntentStatusToProto(intent.Status)
	if err != nil {
		return nil, err
	}
	return &pbapimerchant.PaymentIntent{
		Id:          intent.ID,
		Status:      status,
		AssetId:     intent.AssetID,
		AmountUsd:   intent.AmountUSD,
		AmountAsset: intent.AmountAsset,
		CreatedAt:   timestamppb.New(intent.CreatedAt),
		UpdatedAt:   timestamppb.New(intent.UpdatedAt),
	}, nil
}

func PaymentIntentStatusToProto(status pgpayment.IntentStatus) (pbapimerchant.PaymentIntentStatus, error) {
	switch status {
	case pgpayment.IntentStatusAwaitingPayment:
		return pbapimerchant.PaymentIntentStatus_PAYMENT_INTENT_STATUS_AWAITING_PAYMENT, nil
	case pgpayment.IntentStatusPaid:
		return pbapimerchant.PaymentIntentStatus_PAYMENT_INTENT_STATUS_PAID, nil
	case pgpayment.IntentStatusExpired:
		return pbapimerchant.PaymentIntentStatus_PAYMENT_INTENT_STATUS_EXPIRED, nil
	case pgpayment.IntentStatusAMLCheckPending:
		return pbapimerchant.PaymentIntentStatus_PAYMENT_INTENT_STATUS_AML_CHECK_PENDING, nil
	case pgpayment.IntentStatusAMLCheckFailed:
		return pbapimerchant.PaymentIntentStatus_PAYMENT_INTENT_STATUS_AML_CHECK_FAILED, nil
	case pgpayment.IntentStatusRefundPending:
		return pbapimerchant.PaymentIntentStatus_PAYMENT_INTENT_STATUS_REFUND_PENDING, nil
	case pgpayment.IntentStatusRefunded:
		return pbapimerchant.PaymentIntentStatus_PAYMENT_INTENT_STATUS_REFUNDED, nil
	default:
		return pbapimerchant.PaymentIntentStatus_PAYMENT_INTENT_STATUS_UNSPECIFIED, fmt.Errorf("invalid payment intent status: %s", status)
	}
}
