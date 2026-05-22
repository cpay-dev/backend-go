package workers

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/cpay-dev/cpay/internal/domain/payment"
	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/chain"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type ChainObserver struct {
	DB             *pgxpool.Pool
	CheckoutClient cpayv1.CheckoutServiceClient
	ChainRPCURLs   map[string]string
	Log            zerolog.Logger
	Interval       time.Duration
	Source         string

	clients map[string]*ethclient.Client
}

func (w *ChainObserver) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = 15 * time.Second
	}
	if w.Source == "" {
		w.Source = "chain-observer-service"
	}
	w.openRPCClients(ctx)
	defer w.closeRPCClients()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		w.runRecordedTransactionLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		w.runDepositObserverLoop(ctx)
	}()
	wg.Wait()
}

func (w *ChainObserver) runRecordedTransactionLoop(ctx context.Context) {
	interval := w.Interval
	if interval > 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.updateRecordedTransactions(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.updateRecordedTransactions(ctx)
		}
	}
}

func (w *ChainObserver) runDepositObserverLoop(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	w.expireOldIntents(ctx)
	w.observeDeposits(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.expireOldIntents(ctx)
			w.observeDeposits(ctx)
		}
	}
}

func (w *ChainObserver) openRPCClients(ctx context.Context) {
	w.clients = make(map[string]*ethclient.Client, len(w.ChainRPCURLs))
	for chainName, rpcURL := range w.ChainRPCURLs {
		chainName = strings.ToLower(strings.TrimSpace(chainName))
		if chainName == "" || strings.TrimSpace(rpcURL) == "" {
			continue
		}
		client, err := ethclient.DialContext(ctx, rpcURL)
		if err != nil {
			w.Log.Warn().Err(err).Str("chain", chainName).Msg("chain observer: rpc unavailable")
			continue
		}
		w.clients[chainName] = client
	}
	if len(w.clients) == 0 {
		w.Log.Warn().Msg("chain observer: no chain rpc urls configured; real deposits will not be detected")
	}
}

func (w *ChainObserver) closeRPCClients() {
	for _, client := range w.clients {
		client.Close()
	}
}

type pendingDepositIntent struct {
	SessionID             string
	MerchantID            string
	Chain                 string
	TokenSymbol           string
	TokenAddress          string
	DepositAddress        string
	PaymentIntentID       string
	ExpectedAmountRaw     string
	TolerancePercentRaw   string
	RequiredConfirmations int
}

type recordedChainTransaction struct {
	SessionID             string
	MerchantID            string
	PaymentIntentID       string
	Chain                 string
	TxHash                string
	AmountRaw             string
	FromAddress           string
	ToAddress             string
	TokenSymbol           string
	RequiredConfirmations int
}

func (w *ChainObserver) updateRecordedTransactions(ctx context.Context) {
	if w.CheckoutClient == nil || len(w.clients) == 0 {
		return
	}
	rows, err := w.DB.Query(ctx, `
		SELECT c.id::text, i.merchant_id::text, i.id::text, ct.chain, ct.tx_hash, ct.amount::text,
			COALESCE(ct.from_address, ''), COALESCE(ct.to_address, ''), ct.token_symbol,
			i.required_confirmations
		FROM checkout.chain_transactions ct
		JOIN checkout.payment_intents i ON i.id=ct.payment_intent_id
		JOIN checkout.checkout_sessions c ON c.id=i.checkout_session_id
		WHERE i.status IN ('created', 'awaiting_funds', 'partial')
			AND ct.status='detected'
			AND ct.confirmations < i.required_confirmations
			AND i.created_at > NOW() - INTERVAL '36 hours'
		ORDER BY ct.observed_at ASC
		LIMIT 100
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("chain observer: recorded transaction query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var item recordedChainTransaction
		if err := rows.Scan(
			&item.SessionID, &item.MerchantID, &item.PaymentIntentID, &item.Chain, &item.TxHash, &item.AmountRaw,
			&item.FromAddress, &item.ToAddress, &item.TokenSymbol, &item.RequiredConfirmations,
		); err != nil {
			continue
		}
		w.updateRecordedTransaction(ctx, item)
	}
	if err := rows.Err(); err != nil {
		w.Log.Error().Err(err).Msg("chain observer: recorded transaction rows failed")
	}
}

func (w *ChainObserver) updateRecordedTransaction(ctx context.Context, item recordedChainTransaction) {
	client := w.clientForChain(item.Chain)
	if client == nil || !isHexHash(item.TxHash) {
		return
	}
	receipt, err := client.TransactionReceipt(ctx, common.HexToHash(item.TxHash))
	if err != nil {
		w.Log.Warn().Err(err).Str("chain", item.Chain).Str("tx_hash", item.TxHash).Msg("chain observer: receipt lookup failed")
		return
	}
	if receipt == nil || receipt.BlockNumber == nil {
		return
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		w.Log.Warn().Str("chain", item.Chain).Str("tx_hash", item.TxHash).Msg("chain observer: recorded transaction failed on chain")
		return
	}
	latest, err := client.BlockNumber(ctx)
	if err != nil {
		w.Log.Warn().Err(err).Str("chain", item.Chain).Msg("chain observer: block number failed")
		return
	}
	receiptBlock := receipt.BlockNumber.Uint64()
	if latest < receiptBlock {
		return
	}
	confirmations := int(latest - receiptBlock + 1)
	w.Log.Info().
		Str("session_id", item.SessionID).
		Str("payment_intent_id", item.PaymentIntentID).
		Str("chain", item.Chain).
		Str("tx_hash", item.TxHash).
		Int("confirmations", confirmations).
		Int("required_confirmations", item.RequiredConfirmations).
		Uint64("receipt_block", receiptBlock).
		Uint64("latest_block", latest).
		Msg("chain observer: recorded transaction observed")
	rawPayload, _ := json.Marshal(map[string]any{
		"source": "chain_observer_receipt",
	})
	resp, err := w.CheckoutClient.ConfirmCheckoutSession(ctx, &cpayv1.ConfirmCheckoutSessionRequest{
		MerchantId:     item.MerchantID,
		SessionId:      item.SessionID,
		TxHash:         item.TxHash,
		ReceivedAmount: parseFloat(item.AmountRaw),
		Confirmations:  int32(confirmations),
		HasBlockNumber: true,
		BlockNumber:    int64(receiptBlock),
		FromAddress:    item.FromAddress,
		ToAddress:      item.ToAddress,
		RawPayloadJson: string(rawPayload),
	})
	if err != nil {
		w.Log.Warn().Err(err).Str("session_id", item.SessionID).Str("tx_hash", item.TxHash).Msg("chain observer: recorded transaction confirm failed")
		return
	}
	event := w.Log.Info().
		Str("session_id", item.SessionID).
		Str("payment_intent_id", item.PaymentIntentID).
		Str("chain", item.Chain).
		Str("tx_hash", item.TxHash).
		Str("payment_status", resp.GetStatus()).
		Int("confirmations", int(resp.GetConfirmations())).
		Int("required_confirmations", int(resp.GetRequiredConfirmations()))
	if confirmations >= item.RequiredConfirmations {
		event.Msg("chain observer: recorded transaction reached required confirmations; removed from future polling")
	} else {
		event.Msg("chain observer: recorded transaction confirmations updated")
	}
}

func (w *ChainObserver) observeDeposits(ctx context.Context) {
	if w.CheckoutClient == nil || len(w.clients) == 0 {
		return
	}
	rows, err := w.DB.Query(ctx, `
		SELECT c.id::text, i.merchant_id::text, i.chain, i.token_symbol, COALESCE(i.token_address, ''), d.address,
			i.id::text, i.expected_amount::text, i.tolerance_percent::text, i.required_confirmations
		FROM checkout.payment_intents i
		JOIN checkout.checkout_sessions c ON c.id=i.checkout_session_id
		JOIN checkout.deposit_addresses d ON d.payment_intent_id=i.id
		WHERE i.status IN ('created', 'awaiting_funds', 'partial', 'expired')
			AND i.received_amount = 0
			AND i.created_at > NOW() - INTERVAL '36 hours'
		ORDER BY i.created_at ASC
		LIMIT 100
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("chain observer: deposit query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var item pendingDepositIntent
		if err := rows.Scan(
			&item.SessionID, &item.MerchantID, &item.Chain, &item.TokenSymbol, &item.TokenAddress, &item.DepositAddress,
			&item.PaymentIntentID, &item.ExpectedAmountRaw, &item.TolerancePercentRaw, &item.RequiredConfirmations,
		); err != nil {
			continue
		}
		w.observeERC20Deposit(ctx, item)
	}
	if err := rows.Err(); err != nil {
		w.Log.Error().Err(err).Msg("chain observer: deposit rows failed")
	}
}

func (w *ChainObserver) observeERC20Deposit(ctx context.Context, item pendingDepositIntent) {
	tokenAddress := strings.TrimSpace(item.TokenAddress)
	decimals := 18
	if contract, ok := chain.KnownEVMTokenContract(item.Chain, item.TokenSymbol); ok {
		decimals = contract.Decimals
		if tokenAddress == "" {
			tokenAddress = contract.Address
		}
	}
	if !common.IsHexAddress(tokenAddress) || !common.IsHexAddress(item.DepositAddress) {
		return
	}
	client := w.clientForChain(item.Chain)
	if client == nil {
		return
	}
	if observed, err := w.observedERC20Balance(ctx, client, common.HexToAddress(tokenAddress), common.HexToAddress(item.DepositAddress), decimals); err == nil && observed > 0 {
		if w.confirmObservedBalance(ctx, item, observed) {
			return
		}
	}

	latest, err := client.BlockNumber(ctx)
	if err != nil {
		w.Log.Warn().Err(err).Str("chain", item.Chain).Msg("chain observer: block number failed")
		return
	}
	deposit := common.HexToAddress(item.DepositAddress)
	logs, err := w.filterERC20TransferLogs(ctx, client, item.Chain, common.HexToAddress(tokenAddress), deposit, latest)
	if err != nil {
		w.Log.Warn().Err(err).Str("chain", item.Chain).Str("token", item.TokenSymbol).Msg("chain observer: transfer log scan failed")
		return
	}
	if len(logs) == 0 {
		return
	}

	total := big.NewInt(0)
	confirmations := math.MaxInt
	txHash := logs[len(logs)-1].TxHash.Hex()
	for _, transfer := range logs {
		if len(transfer.Data) == 0 {
			continue
		}
		total.Add(total, new(big.Int).SetBytes(transfer.Data))
		confs := int(latest - transfer.BlockNumber + 1)
		if confs < confirmations {
			confirmations = confs
		}
	}
	if total.Sign() <= 0 || confirmations == math.MaxInt {
		return
	}

	amount := baseUnitsToFloat(total, decimals)
	rawPayload, _ := json.Marshal(map[string]any{
		"source":        "chain_observer",
		"token_address": tokenAddress,
		"deposit":       item.DepositAddress,
	})
	_, err = w.CheckoutClient.ConfirmCheckoutSession(ctx, &cpayv1.ConfirmCheckoutSessionRequest{
		MerchantId:     item.MerchantID,
		SessionId:      item.SessionID,
		TxHash:         txHash,
		ReceivedAmount: amount,
		Confirmations:  int32(confirmations),
		HasBlockNumber: true,
		BlockNumber:    int64(latest),
		ToAddress:      item.DepositAddress,
		RawPayloadJson: string(rawPayload),
	})
	if err != nil {
		w.Log.Warn().Err(err).Str("session_id", item.SessionID).Msg("chain observer: confirm checkout failed")
	}
}

func (w *ChainObserver) filterERC20TransferLogs(ctx context.Context, client *ethclient.Client, chainName string, token, deposit common.Address, latest uint64) ([]types.Log, error) {
	scanDepth := uint64(50000)
	chunkSize := uint64(10000)
	if strings.Contains(strings.ToLower(chainName), "hyper") {
		chunkSize = 1000
	}
	fromFloor := uint64(0)
	if latest > scanDepth {
		fromFloor = latest - scanDepth
	}
	to := latest
	for {
		from := fromFloor
		if to > chunkSize && to-chunkSize+1 > fromFloor {
			from = to - chunkSize + 1
		}
		logs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(from),
			ToBlock:   new(big.Int).SetUint64(to),
			Addresses: []common.Address{token},
			Topics: [][]common.Hash{
				{erc20TransferTopic()},
				nil,
				{common.BytesToHash(deposit.Bytes())},
			},
		})
		if err != nil {
			return nil, err
		}
		if len(logs) > 0 || from == fromFloor {
			return logs, nil
		}
		to = from - 1
	}
}

func (w *ChainObserver) observedERC20Balance(ctx context.Context, client *ethclient.Client, token, deposit common.Address, decimals int) (float64, error) {
	selector := crypto.Keccak256([]byte("balanceOf(address)"))[:4]
	data := append([]byte{}, selector...)
	data = append(data, common.BytesToHash(deposit.Bytes()).Bytes()...)
	out, err := client.CallContract(ctx, ethereum.CallMsg{To: &token, Data: data}, nil)
	if err != nil {
		return 0, err
	}
	if len(out) == 0 {
		return 0, nil
	}
	return baseUnitsToFloat(new(big.Int).SetBytes(out), decimals), nil
}

func (w *ChainObserver) confirmObservedBalance(ctx context.Context, item pendingDepositIntent, receivedAmount float64) bool {
	expected := parseFloat(item.ExpectedAmountRaw)
	tolerance := parseFloat(item.TolerancePercentRaw)
	status := payment.ResolveIntentStatus(expected, receivedAmount, tolerance, item.RequiredConfirmations, item.RequiredConfirmations)
	statusIsPaid := payment.IntentStatusIsPaid(status)
	sessionStatus := "awaiting_funds"
	if statusIsPaid {
		sessionStatus = "paid"
	} else if status == "expired" || status == "failed" {
		sessionStatus = "failed"
	}

	tx, err := w.DB.Begin(ctx)
	if err != nil {
		w.Log.Warn().Err(err).Str("payment_intent_id", item.PaymentIntentID).Msg("chain observer: balance confirm begin failed")
		return false
	}
	defer tx.Rollback(ctx)

	var confirmedAt any
	if statusIsPaid {
		confirmedAt = time.Now().UTC()
	}
	if _, err = tx.Exec(ctx, `
		UPDATE checkout.payment_intents
		SET status=$1, received_amount=$2, confirmations=$3, confirmed_at=COALESCE(confirmed_at, $4), updated_at=NOW()
		WHERE id::text=$5
	`, status, receivedAmount, item.RequiredConfirmations, confirmedAt, item.PaymentIntentID); err != nil {
		w.Log.Warn().Err(err).Str("payment_intent_id", item.PaymentIntentID).Msg("chain observer: balance confirm intent update failed")
		return false
	}
	if _, err = tx.Exec(ctx, `UPDATE checkout.checkout_sessions SET status=$1, updated_at=NOW() WHERE id::text=$2`, sessionStatus, item.SessionID); err != nil {
		w.Log.Warn().Err(err).Str("session_id", item.SessionID).Msg("chain observer: balance confirm session update failed")
		return false
	}
	_ = enqueueOutboxTx(ctx, tx, w.Source, "payment_intent", item.PaymentIntentID, item.MerchantID, "payment.detected", map[string]any{
		"payment_intent_id": item.PaymentIntentID,
		"received_amount":   receivedAmount,
		"confirmations":     item.RequiredConfirmations,
		"status":            status,
		"source":            "balance",
	})
	if statusIsPaid {
		_ = enqueueOutboxTx(ctx, tx, w.Source, "payment_intent", item.PaymentIntentID, item.MerchantID, "payment.confirmed", map[string]any{
			"payment_intent_id": item.PaymentIntentID,
			"received_amount":   receivedAmount,
			"source":            "balance",
		})
	}
	if err = tx.Commit(ctx); err != nil {
		w.Log.Warn().Err(err).Str("payment_intent_id", item.PaymentIntentID).Msg("chain observer: balance confirm commit failed")
		return false
	}
	return statusIsPaid
}

func (w *ChainObserver) clientForChain(chainName string) *ethclient.Client {
	chainName = strings.ToLower(strings.TrimSpace(chainName))
	if client := w.clients[chainName]; client != nil {
		return client
	}
	switch {
	case strings.Contains(chainName, "arbitrum"):
		return w.clients["arbitrum"]
	case strings.Contains(chainName, "ethereum") || chainName == "mainnet":
		return w.clients["ethereum"]
	case strings.Contains(chainName, "bnb"):
		if client := w.clients["bsc"]; client != nil {
			return client
		}
		return w.clients["bnb"]
	case strings.Contains(chainName, "hyper"):
		return w.clients["hyperevm"]
	default:
		return nil
	}
}

func erc20TransferTopic() common.Hash {
	return crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
}

func isHexHash(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 66 || !strings.HasPrefix(value, "0x") {
		return false
	}
	for _, c := range value[2:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func baseUnitsToFloat(value *big.Int, decimals int) float64 {
	if decimals < 0 {
		decimals = 0
	}
	f := new(big.Float).SetInt(value)
	scale := new(big.Float).SetFloat64(math.Pow10(decimals))
	out, _ := new(big.Float).Quo(f, scale).Float64()
	return out
}

func (w *ChainObserver) expireOldIntents(ctx context.Context) {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		w.Log.Error().Err(err).Msg("chain observer: begin tx failed")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id::text, merchant_id::text, checkout_session_id::text
		FROM checkout.payment_intents
		WHERE status IN ('created', 'awaiting_funds', 'partial') AND expires_at < NOW()
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("chain observer: query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var intentID, merchantID, sessionID string
		if err := rows.Scan(&intentID, &merchantID, &sessionID); err != nil {
			continue
		}
		_, _ = tx.Exec(ctx, `UPDATE checkout.payment_intents SET status='expired', updated_at=NOW() WHERE id=$1`, intentID)
		_, _ = tx.Exec(ctx, `UPDATE checkout.checkout_sessions SET status='expired', updated_at=NOW() WHERE id=$1`, sessionID)
		_ = enqueueOutboxTx(ctx, tx, w.Source, "payment_intent", intentID, merchantID, "payment.expired", map[string]any{"payment_intent_id": intentID})
	}

	if err := tx.Commit(ctx); err != nil {
		w.Log.Error().Err(err).Msg("chain observer: commit failed")
	}
}
