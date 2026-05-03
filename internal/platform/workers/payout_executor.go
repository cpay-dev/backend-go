package workers

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type PayoutExecutionRequest struct {
	PayoutID             string
	PayoutItemID         string
	PaymentIntentID      string
	MerchantID           string
	Chain                string
	TokenSymbol          string
	TokenAddress         string
	AmountRaw            string
	DepositAddress       string
	DepositPrivateKeyHex string
	SettlementAddress    string
}

type PayoutExecutionResult struct {
	TxHash string
}

type PayoutExecutor interface {
	ExecutePayout(ctx context.Context, req PayoutExecutionRequest) (PayoutExecutionResult, error)
}

type MockPayoutExecutor struct{}

func (MockPayoutExecutor) ExecutePayout(context.Context, PayoutExecutionRequest) (PayoutExecutionResult, error) {
	return PayoutExecutionResult{TxHash: "0xsweep_" + strings.ToLower(ids.New())}, nil
}

type evmRPCClient interface {
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	ChainID(ctx context.Context) (*big.Int, error)
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	Close()
}

type EVMPayoutExecutor struct {
	clients map[string]evmRPCClient
}

func NewEVMPayoutExecutor(ctx context.Context, rpcURLs map[string]string) (*EVMPayoutExecutor, error) {
	clients := make(map[string]evmRPCClient, len(rpcURLs))
	for chainName, rpcURL := range rpcURLs {
		client, err := ethclient.DialContext(ctx, rpcURL)
		if err != nil {
			closeEVMClients(clients)
			return nil, fmt.Errorf("dial %s rpc: %w", chainName, err)
		}
		if _, err := client.ChainID(ctx); err != nil {
			client.Close()
			closeEVMClients(clients)
			return nil, fmt.Errorf("validate %s rpc: %w", chainName, err)
		}
		clients[strings.ToLower(strings.TrimSpace(chainName))] = client
	}
	if len(clients) == 0 {
		return nil, errors.New("at least one chain rpc url is required")
	}
	return &EVMPayoutExecutor{clients: clients}, nil
}

func (e *EVMPayoutExecutor) Close() {
	closeEVMClients(e.clients)
}

func closeEVMClients(clients map[string]evmRPCClient) {
	for _, client := range clients {
		client.Close()
	}
}

func (e *EVMPayoutExecutor) ExecutePayout(ctx context.Context, req PayoutExecutionRequest) (PayoutExecutionResult, error) {
	chainName := strings.ToLower(strings.TrimSpace(req.Chain))
	client := e.clients[chainName]
	if client == nil {
		return PayoutExecutionResult{}, fmt.Errorf("rpc client for chain %s is not configured", chainName)
	}
	if !common.IsHexAddress(req.SettlementAddress) {
		return PayoutExecutionResult{}, errors.New("settlement address is invalid")
	}

	keyHex := stripHexPrefix(req.DepositPrivateKeyHex)
	privateKey, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("parse deposit private key: %w", err)
	}
	from := crypto.PubkeyToAddress(privateKey.PublicKey)
	to := common.HexToAddress(req.SettlementAddress)

	tokenAddress := strings.TrimSpace(req.TokenAddress)
	var value *big.Int
	var txTo common.Address
	var data []byte
	if tokenAddress == "" {
		value, err = decimalToBaseUnits(req.AmountRaw, 18)
		if err != nil {
			return PayoutExecutionResult{}, err
		}
		txTo = to
	} else {
		if !common.IsHexAddress(tokenAddress) {
			return PayoutExecutionResult{}, errors.New("token address is invalid")
		}
		txTo = common.HexToAddress(tokenAddress)
		decimals, err := e.erc20Decimals(ctx, client, txTo)
		if err != nil {
			return PayoutExecutionResult{}, err
		}
		value, err = decimalToBaseUnits(req.AmountRaw, decimals)
		if err != nil {
			return PayoutExecutionResult{}, err
		}
		data = erc20TransferData(to, value)
	}

	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("load nonce: %w", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("suggest gas price: %w", err)
	}
	callValue := big.NewInt(0)
	if tokenAddress == "" {
		callValue = value
	}
	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{
		From:  from,
		To:    &txTo,
		Gas:   0,
		Value: callValue,
		Data:  data,
	})
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("estimate gas: %w", err)
	}
	if gasLimit < 21000 {
		gasLimit = 21000
	}

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("load chain id: %w", err)
	}
	tx := types.NewTransaction(nonce, txTo, callValue, gasLimit, gasPrice, data)
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), privateKey)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("sign transaction: %w", err)
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("send transaction: %w", err)
	}
	return PayoutExecutionResult{TxHash: signed.Hash().Hex()}, nil
}

func (e *EVMPayoutExecutor) erc20Decimals(ctx context.Context, client evmRPCClient, token common.Address) (int, error) {
	selector := crypto.Keccak256([]byte("decimals()"))[:4]
	out, err := client.CallContract(ctx, ethereum.CallMsg{To: &token, Data: selector}, nil)
	if err != nil {
		return 0, fmt.Errorf("read token decimals: %w", err)
	}
	if len(out) == 0 {
		return 0, errors.New("token decimals response is empty")
	}
	decimals := new(big.Int).SetBytes(out).Int64()
	if decimals < 0 || decimals > 255 {
		return 0, errors.New("token decimals response is invalid")
	}
	return int(decimals), nil
}

func erc20TransferData(to common.Address, amount *big.Int) []byte {
	selector := crypto.Keccak256([]byte("transfer(address,uint256)"))[:4]
	data := make([]byte, 0, 68)
	data = append(data, selector...)
	data = append(data, common.LeftPadBytes(to.Bytes(), 32)...)
	data = append(data, common.LeftPadBytes(amount.Bytes(), 32)...)
	return data
}

func decimalToBaseUnits(raw string, decimals int) (*big.Int, error) {
	if decimals < 0 {
		return nil, errors.New("decimals must be non-negative")
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, errors.New("amount is required")
	}
	if strings.HasPrefix(value, "-") || strings.ContainsAny(value, "eE") {
		return nil, errors.New("amount is invalid")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return nil, errors.New("amount is invalid")
	}
	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if !digitsOnly(whole) || !digitsOnly(frac) {
		return nil, errors.New("amount is invalid")
	}
	frac = strings.TrimRight(frac, "0")
	if len(frac) > decimals {
		return nil, fmt.Errorf("amount has more than %d decimals", decimals)
	}
	frac += strings.Repeat("0", decimals-len(frac))
	combined := strings.TrimLeft(whole+frac, "0")
	if combined == "" {
		return nil, errors.New("amount must be greater than zero")
	}
	out, ok := new(big.Int).SetString(combined, 10)
	if !ok {
		return nil, errors.New("amount is invalid")
	}
	return out, nil
}

func digitsOnly(v string) bool {
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func stripHexPrefix(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '0' && (v[1] == 'x' || v[1] == 'X') {
		return v[2:]
	}
	return v
}
