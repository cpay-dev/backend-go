package scheduler

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cpay-dev/backend/internal/billing"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
	grpcclient "github.com/cpay-dev/backend/internal/grpc"
)

type Scheduler struct {
	queries  *db.Queries
	chain    *grpcclient.ChainClient
	eventPub *events.Publisher
}

func New(queries *db.Queries, chain *grpcclient.ChainClient, eventPub *events.Publisher) *Scheduler {
	return &Scheduler{
		queries:  queries,
		chain:    chain,
		eventPub: eventPub,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run immediately on start
	s.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("scheduler stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	log.Info().Msg("scheduler tick starting")
	s.processDueCharges(ctx)
	s.sendReminders(ctx)
	s.expirePastDue(ctx)
	log.Info().Msg("scheduler tick completed")
}

func (s *Scheduler) processDueCharges(ctx context.Context) {
	now := time.Now()
	dueSubscribers, err := s.queries.ListDueSubscribers(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if err != nil {
		log.Error().Err(err).Msg("scheduler: list due subscribers")
		return
	}

	for _, sub := range dueSubscribers {
		switch sub.Method {
		case db.PaymentMethodPrepaid:
			s.processPrepaid(ctx, sub)
		case db.PaymentMethodApproved:
			s.processApproved(ctx, sub)
		}
	}
}

func (s *Scheduler) processPrepaid(ctx context.Context, sub db.ListDueSubscribersRow) {
	if sub.PeriodsUsed >= sub.PeriodsPaid {
		// All periods used — expire
		_, _ = s.queries.ExpireSubscriber(ctx, sub.ID)
		s.publishExpired(ctx, sub)
		log.Info().Str("id", sub.ID).Msg("subscriber expired (prepaid exhausted)")
		return
	}

	// Increment periods used
	updated, err := s.queries.IncrementPeriodsUsed(ctx, sub.ID)
	if err != nil {
		log.Error().Err(err).Str("id", sub.ID).Msg("scheduler: increment periods_used")
		return
	}

	// Check if this was the last period
	if updated.PeriodsUsed >= updated.PeriodsPaid {
		_, _ = s.queries.ExpireSubscriber(ctx, sub.ID)
		s.publishExpired(ctx, sub)
		log.Info().Str("id", sub.ID).Msg("subscriber expired (all periods used)")
		return
	}

	// Advance next_due
	nextDue := billing.NextDueFrom(sub.NextDue.Time, sub.SubPeriod)
	_, _ = s.queries.SetNextDue(ctx, db.SetNextDueParams{
		ID:      sub.ID,
		NextDue: pgtype.Timestamptz{Time: nextDue, Valid: true},
	})
	log.Info().Str("id", sub.ID).Int32("used", updated.PeriodsUsed).Int32("paid", updated.PeriodsPaid).Msg("prepaid period consumed")
}

func (s *Scheduler) processApproved(ctx context.Context, sub db.ListDueSubscribersRow) {
	// Get relayer address for allowance check
	relayerAddr, err := s.chain.GetRelayerAddress(ctx)
	if err != nil {
		log.Error().Err(err).Msg("scheduler: get relayer address")
		return
	}

	// Check allowance
	allowance, err := s.chain.CheckAllowance(ctx, uint64(sub.SubChainID), sub.SubToken, sub.PayerAddress, relayerAddr)
	if err != nil {
		log.Error().Err(err).Str("id", sub.ID).Msg("scheduler: check allowance")
		return
	}

	// Parse amounts for comparison
	amountStr := numericToString(sub.SubAmount)
	amountBig, ok := new(big.Int).SetString(amountStr, 10)
	if !ok {
		log.Error().Str("amount", amountStr).Str("id", sub.ID).Msg("scheduler: parse sub amount")
		return
	}
	allowanceBig, ok := new(big.Int).SetString(allowance, 10)
	if !ok {
		log.Error().Str("allowance", allowance).Str("id", sub.ID).Msg("scheduler: parse allowance")
		return
	}

	if allowanceBig.Cmp(amountBig) < 0 {
		// Insufficient allowance
		_, _ = s.queries.SetSubscriberPastDue(ctx, sub.ID)
		s.publishFailed(ctx, sub, "Insufficient token allowance")
		log.Warn().Str("id", sub.ID).Msg("subscriber past_due: insufficient allowance")
		return
	}

	// Get CF address for merchant
	subWithMerchant, err := s.queries.GetSubscriptionWithMerchant(ctx, sub.SubscriptionID)
	if err != nil {
		log.Error().Err(err).Msg("scheduler: get subscription with merchant")
		return
	}

	merchantCF := ""
	if subWithMerchant.MerchantCfAddress != nil {
		merchantCF = *subWithMerchant.MerchantCfAddress
	}
	if merchantCF == "" {
		// Derive CF address
		merchantCF, err = s.chain.GetCounterfactualAddress(ctx, uint64(sub.SubChainID), subWithMerchant.MerchantWallet)
		if err != nil {
			log.Error().Err(err).Msg("scheduler: derive cf address")
			return
		}
	}

	// Execute transferFrom
	txHash, err := s.chain.ExecuteTransferFrom(ctx, uint64(sub.SubChainID), sub.SubToken, sub.PayerAddress, merchantCF, amountStr)
	if err != nil {
		_, _ = s.queries.SetSubscriberPastDue(ctx, sub.ID)
		s.publishFailed(ctx, sub, fmt.Sprintf("Transfer failed: %v", err))
		log.Error().Err(err).Str("id", sub.ID).Msg("scheduler: transferFrom failed")
		return
	}

	// Record payment
	amount, _ := parsePriceToNumericStr(amountStr)
	_, err = s.queries.CreateSubscriptionPayment(ctx, db.CreateSubscriptionPaymentParams{
		SubscriptionID: sub.SubscriptionID,
		ShopID:         sub.ShopID,
		PayerAddress:   sub.PayerAddress,
		Amount:         amount,
		TokenAddress:   sub.SubToken,
		ChainID:        sub.SubChainID,
		TxHash:         txHash,
		SubscriberID:   &sub.ID,
		Verified:       true, // we just executed it
		Method:         db.PaymentMethodApproved,
	})
	if err != nil {
		log.Error().Err(err).Str("id", sub.ID).Msg("scheduler: record payment")
	}

	// Update spent amount
	spentBig := new(big.Int)
	if sub.SpentAmount != nil {
		spentBig.SetString(*sub.SpentAmount, 10)
	}
	spentBig.Add(spentBig, amountBig)
	spentStr := spentBig.String()
	_, _ = s.queries.UpdateSpentAmount(ctx, db.UpdateSpentAmountParams{
		ID:          sub.ID,
		SpentAmount: &spentStr,
	})

	// Advance next_due
	nextDue := billing.NextDueFrom(sub.NextDue.Time, sub.SubPeriod)
	_, _ = s.queries.SetNextDue(ctx, db.SetNextDueParams{
		ID:      sub.ID,
		NextDue: pgtype.Timestamptz{Time: nextDue, Valid: true},
	})

	// Publish payment event
	if evt, err := events.NewEvent(events.EventSubscriptionPayment, events.SubjectSubscriptions, map[string]interface{}{
		"id": "", "subscription_id": sub.SubscriptionID, "tx_hash": txHash,
		"chain_id": sub.SubChainID, "payer_address": sub.PayerAddress, "shop_id": sub.ShopID,
		"amount": amountStr,
	}); err == nil {
		_ = s.eventPub.Publish(ctx, evt)
	}

	log.Info().Str("id", sub.ID).Str("tx_hash", txHash).Msg("auto-charge successful")
}

func (s *Scheduler) sendReminders(ctx context.Context) {
	now := time.Now()
	horizon := now.Add(3 * 24 * time.Hour)

	upcoming, err := s.queries.ListUpcomingDueSubscribers(ctx, db.ListUpcomingDueSubscribersParams{
		NextDue:   pgtype.Timestamptz{Time: now, Valid: true},
		NextDue_2: pgtype.Timestamptz{Time: horizon, Valid: true},
	})
	if err != nil {
		log.Error().Err(err).Msg("scheduler: list upcoming")
		return
	}

	for _, sub := range upcoming {
		if sub.PayerEmail != "" && sub.NextDue.Valid {
			amountStr := numericToString(sub.SubAmount)
			evt, err := events.NewEvent(events.EventEmailQueued, events.SubjectEmails, events.EmailQueuedData{
				EmailType: events.EmailTypeUpcomingPayment,
				To:        sub.PayerEmail,
				Title:     sub.SubTitle,
				Amount:    amountStr,
				NextDue:   sub.NextDue.Time.Format("2006-01-02T15:04:05Z07:00"),
			})
			if err != nil {
				log.Error().Err(err).Str("to", sub.PayerEmail).Msg("scheduler: failed to create upcoming email event")
				continue
			}
			if err := s.eventPub.Publish(ctx, evt); err != nil {
				log.Error().Err(err).Str("to", sub.PayerEmail).Msg("scheduler: failed to publish upcoming email event")
			}
		}
	}
}

func (s *Scheduler) expirePastDue(ctx context.Context) {
	threshold := time.Now().Add(-7 * 24 * time.Hour)
	pastDue, err := s.queries.ListPastDueSubscribers(ctx, threshold)
	if err != nil {
		log.Error().Err(err).Msg("scheduler: list past_due")
		return
	}

	for _, sub := range pastDue {
		_, _ = s.queries.ExpireSubscriber(ctx, sub.ID)
		log.Info().Str("id", sub.ID).Msg("past_due subscriber expired")
	}
}

func (s *Scheduler) publishExpired(ctx context.Context, sub db.ListDueSubscribersRow) {
	if evt, err := events.NewEvent(events.EventSubscriberExpired, events.SubjectSubscribers, map[string]string{
		"id": sub.ID, "subscription_id": sub.SubscriptionID, "email": sub.PayerEmail,
	}); err == nil {
		_ = s.eventPub.Publish(ctx, evt)
	}
}

func (s *Scheduler) publishFailed(ctx context.Context, sub db.ListDueSubscribersRow, reason string) {
	amountStr := numericToString(sub.SubAmount)
	if evt, err := events.NewEvent(events.EventSubscriberFailed, events.SubjectSubscribers, map[string]string{
		"id": sub.ID, "subscription_id": sub.SubscriptionID,
		"email": sub.PayerEmail, "amount": amountStr, "reason": reason,
	}); err == nil {
		_ = s.eventPub.Publish(ctx, evt)
	}
}

// numericToString converts pgtype.Numeric to string (duplicated from api package for scheduler use).
func numericToString(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0"
	}
	return n.Int.String()
}

// parsePriceToNumericStr parses a string amount into pgtype.Numeric.
func parsePriceToNumericStr(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return n, err
	}
	return n, nil
}
