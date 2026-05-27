package workers

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/cpay-dev/cpay/internal/shared/chain"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
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
	WalletType           string
	FactoryAddress       string
	WalletSalt           string
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
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	ChainID(ctx context.Context) (*big.Int, error)
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	Close()
}

type EVMPayoutExecutor struct {
	clients          map[string]evmRPCClient
	hotWalletKey     *ecdsa.PrivateKey
	gasBufferPercent float64
}

type EVMPayoutExecutorConfig struct {
	HotWalletPrivateKey string
	GasBufferPercent    float64
}

const checkoutWalletSweepABI = `[{"inputs":[{"internalType":"bytes32","name":"salt","type":"bytes32"},{"internalType":"address","name":"token","type":"address"},{"internalType":"address","name":"recipient","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"}],"name":"deployAndSweep","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

var checkoutWalletSweepParsedABI = mustParsePayoutABI()

func NewEVMPayoutExecutor(ctx context.Context, rpcURLs map[string]string, cfg EVMPayoutExecutorConfig) (*EVMPayoutExecutor, error) {
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
		clients[chain.NormalizeEVMChain(chainName)] = client
	}
	if len(clients) == 0 {
		return nil, errors.New("at least one chain rpc url is required")
	}
	var hotWalletKey *ecdsa.PrivateKey
	if strings.TrimSpace(cfg.HotWalletPrivateKey) != "" {
		key, err := crypto.HexToECDSA(stripHexPrefix(cfg.HotWalletPrivateKey))
		if err != nil {
			closeEVMClients(clients)
			return nil, fmt.Errorf("parse hot wallet private key: %w", err)
		}
		hotWalletKey = key
	}
	return &EVMPayoutExecutor{
		clients:          clients,
		hotWalletKey:     hotWalletKey,
		gasBufferPercent: cfg.GasBufferPercent,
	}, nil
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
	chainName := chain.NormalizeEVMChain(req.Chain)
	client := e.clients[chainName]
	if client == nil {
		return PayoutExecutionResult{}, fmt.Errorf("rpc client for chain %s is not configured", chainName)
	}
	if !common.IsHexAddress(req.SettlementAddress) {
		return PayoutExecutionResult{}, errors.New("settlement address is invalid")
	}
	walletType := strings.ToLower(strings.TrimSpace(req.WalletType))
	if walletType == "" {
		walletType = chain.WalletTypeEOA
	}
	switch walletType {
	case chain.WalletTypeEOA:
		return e.executeEOAPayout(ctx, client, req)
	case chain.WalletTypeCreate2:
		return e.executeCreate2Payout(ctx, client, req)
	default:
		return PayoutExecutionResult{}, fmt.Errorf("unsupported payout wallet type %s", walletType)
	}
}

func (e *EVMPayoutExecutor) executeEOAPayout(ctx context.Context, client evmRPCClient, req PayoutExecutionRequest) (PayoutExecutionResult, error) {
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

func (e *EVMPayoutExecutor) executeCreate2Payout(ctx context.Context, client evmRPCClient, req PayoutExecutionRequest) (PayoutExecutionResult, error) {
	if e.hotWalletKey == nil {
		return PayoutExecutionResult{}, errors.New("hot wallet private key is not configured")
	}
	if !common.IsHexAddress(req.FactoryAddress) {
		return PayoutExecutionResult{}, errors.New("checkout wallet factory address is invalid")
	}
	salt, err := decodeBytes32(req.WalletSalt)
	if err != nil {
		return PayoutExecutionResult{}, err
	}
	settlement := common.HexToAddress(req.SettlementAddress)
	token := common.Address{}
	decimals := 18
	if strings.TrimSpace(req.TokenAddress) != "" {
		if !common.IsHexAddress(req.TokenAddress) {
			return PayoutExecutionResult{}, errors.New("token address is invalid")
		}
		token = common.HexToAddress(req.TokenAddress)
		decimals, err = e.erc20Decimals(ctx, client, token)
		if err != nil {
			return PayoutExecutionResult{}, err
		}
	}
	amount, err := decimalToBaseUnits(req.AmountRaw, decimals)
	if err != nil {
		return PayoutExecutionResult{}, err
	}
	data, err := checkoutWalletSweepParsedABI.Pack("deployAndSweep", salt, token, settlement, amount)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("pack deployAndSweep call: %w", err)
	}
	from := crypto.PubkeyToAddress(e.hotWalletKey.PublicKey)
	factory := common.HexToAddress(req.FactoryAddress)
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("load nonce: %w", err)
	}
	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &factory, Value: big.NewInt(0), Data: data})
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("estimate deployAndSweep gas: %w", err)
	}
	gasLimit = applyGasBuffer(gasLimit, e.gasBufferPercent)
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("load chain id: %w", err)
	}
	tx, err := e.buildHotWalletTx(ctx, client, chainID, nonce, factory, gasLimit, data)
	if err != nil {
		return PayoutExecutionResult{}, err
	}
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), e.hotWalletKey)
	if err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("sign deployAndSweep transaction: %w", err)
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return PayoutExecutionResult{}, fmt.Errorf("send deployAndSweep transaction: %w", err)
	}
	return PayoutExecutionResult{TxHash: signed.Hash().Hex()}, nil
}

func (e *EVMPayoutExecutor) buildHotWalletTx(ctx context.Context, client evmRPCClient, chainID *big.Int, nonce uint64, to common.Address, gasLimit uint64, data []byte) (*types.Transaction, error) {
	header, headerErr := client.HeaderByNumber(ctx, nil)
	tip, tipErr := client.SuggestGasTipCap(ctx)
	if headerErr == nil && tipErr == nil && header != nil && header.BaseFee != nil && tip != nil {
		feeCap := new(big.Int).Mul(header.BaseFee, big.NewInt(2))
		feeCap.Add(feeCap, tip)
		return types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			GasTipCap: tip,
			GasFeeCap: feeCap,
			Gas:       gasLimit,
			To:        &to,
			Value:     big.NewInt(0),
			Data:      data,
		}), nil
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("suggest gas price: %w", err)
	}
	return types.NewTransaction(nonce, to, big.NewInt(0), gasLimit, gasPrice, data), nil
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

func applyGasBuffer(gas uint64, percent float64) uint64 {
	if gas < 21000 {
		gas = 21000
	}
	if percent <= 0 {
		return gas
	}
	buffered := float64(gas) * (1 + percent/100)
	if buffered > float64(math.MaxUint64) {
		return math.MaxUint64
	}
	return uint64(math.Ceil(buffered))
}

func decodeBytes32(raw string) ([32]byte, error) {
	var out [32]byte
	value := stripHexPrefix(raw)
	if len(value) != 64 {
		return out, fmt.Errorf("wallet salt must be 32 bytes")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return out, fmt.Errorf("wallet salt is invalid: %w", err)
	}
	if len(decoded) != 32 {
		return out, fmt.Errorf("wallet salt must be 32 bytes")
	}
	copy(out[:], decoded)
	return out, nil
}

func mustParsePayoutABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(checkoutWalletSweepABI))
	if err != nil {
		panic(err)
	}
	return parsed
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
