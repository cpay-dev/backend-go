package model

import (
	"fmt"

	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	pbpayment "github.com/cpay-dev/proto-go/api/v1/merchant/payment"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func IntentToProto(intent pgpayment.Intent) (*pbpayment.Intent, error) {
	status, err := IntentStatusToProto(intent.Status)
	if err != nil {
		return nil, err
	}
	return &pbpayment.Intent{
		Id:          intent.ID,
		Status:      status,
		AssetId:     intent.AssetID,
		AmountUsd:   intent.AmountUSD,
		AmountAsset: intent.AmountAsset,
		CreatedAt:   timestamppb.New(intent.CreatedAt),
		UpdatedAt:   timestamppb.New(intent.UpdatedAt),
	}, nil
}

func IntentStatusToProto(status pgpayment.IntentStatus) (pbpayment.IntentStatus, error) {
	switch status {
	case pgpayment.IntentStatusAwaitingPayment:
		return pbpayment.IntentStatus_INTENT_STATUS_AWAITING_PAYMENT, nil
	case pgpayment.IntentStatusPaid:
		return pbpayment.IntentStatus_INTENT_STATUS_PAID, nil
	case pgpayment.IntentStatusExpired:
		return pbpayment.IntentStatus_INTENT_STATUS_EXPIRED, nil
	case pgpayment.IntentStatusAMLCheckPending:
		return pbpayment.IntentStatus_INTENT_STATUS_AML_CHECK_PENDING, nil
	case pgpayment.IntentStatusAMLCheckFailed:
		return pbpayment.IntentStatus_INTENT_STATUS_AML_CHECK_FAILED, nil
	case pgpayment.IntentStatusRefundPending:
		return pbpayment.IntentStatus_INTENT_STATUS_REFUND_PENDING, nil
	case pgpayment.IntentStatusRefunded:
		return pbpayment.IntentStatus_INTENT_STATUS_REFUNDED, nil
	default:
		return pbpayment.IntentStatus_INTENT_STATUS_UNSPECIFIED, fmt.Errorf("invalid intent status: %s", status)
	}
}
