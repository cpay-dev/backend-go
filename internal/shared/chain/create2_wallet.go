package chain

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	WalletTypeEOA     = "eoa"
	WalletTypeCreate2 = "create2"

	checkoutWalletFactoryABI = `[{"inputs":[{"internalType":"bytes32","name":"salt","type":"bytes32"}],"name":"walletAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}]`
)

var checkoutWalletFactoryParsedABI = mustParseCheckoutWalletFactoryABI()

type Create2WalletProvider struct {
	rpcURLs          map[string]string
	factoryAddresses map[string]string
	predictWallet    func(ctx context.Context, rpcURL string, factory common.Address, salt [32]byte) (common.Address, error)
}

func NewCreate2WalletProvider(rpcURLs, factoryAddresses map[string]string) *Create2WalletProvider {
	if len(factoryAddresses) == 0 {
		return nil
	}
	return &Create2WalletProvider{
		rpcURLs:          normalizeStringMap(rpcURLs),
		factoryAddresses: normalizeStringMap(factoryAddresses),
		predictWallet:    predictWithFactory,
	}
}

func (p *Create2WalletProvider) DepositWallet(ctx context.Context, chainName string, paymentIntentID string) (DepositWallet, bool, error) {
	chainName = normalizeEVMChain(chainName)
	factoryAddress := p.factoryAddresses[chainName]
	if factoryAddress == "" {
		return DepositWallet{}, false, nil
	}
	factory, err := requireHexAddress("checkout wallet factory address", factoryAddress)
	if err != nil {
		return DepositWallet{}, false, err
	}
	rpcURL := strings.TrimSpace(p.rpcURLs[chainName])
	if rpcURL == "" {
		return DepositWallet{}, false, fmt.Errorf("rpc url for %s is required to predict checkout wallet", chainName)
	}
	if strings.TrimSpace(paymentIntentID) == "" {
		return DepositWallet{}, false, fmt.Errorf("payment intent id is required to derive checkout wallet salt")
	}

	salt := CheckoutWalletSalt(paymentIntentID, chainName)
	predictWallet := p.predictWallet
	if predictWallet == nil {
		predictWallet = predictWithFactory
	}
	address, err := predictWallet(ctx, rpcURL, factory, salt)
	if err != nil {
		return DepositWallet{}, false, err
	}
	return DepositWallet{
		Address:        address.Hex(),
		WalletType:     WalletTypeCreate2,
		FactoryAddress: factory.Hex(),
		WalletSalt:     "0x" + hex.EncodeToString(salt[:]),
	}, true, nil
}

func predictWithFactory(ctx context.Context, rpcURL string, factory common.Address, salt [32]byte) (common.Address, error) {
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return common.Address{}, fmt.Errorf("dial checkout wallet factory rpc: %w", err)
	}
	defer client.Close()

	data, err := checkoutWalletFactoryParsedABI.Pack("walletAddress", salt)
	if err != nil {
		return common.Address{}, fmt.Errorf("pack walletAddress call: %w", err)
	}
	out, err := client.CallContract(ctx, ethereum.CallMsg{To: &factory, Data: data}, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("call checkout wallet factory: %w", err)
	}
	values, err := checkoutWalletFactoryParsedABI.Unpack("walletAddress", out)
	if err != nil {
		return common.Address{}, fmt.Errorf("unpack walletAddress call: %w", err)
	}
	if len(values) != 1 {
		return common.Address{}, fmt.Errorf("walletAddress returned %d values", len(values))
	}
	address, ok := values[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("walletAddress returned unexpected type %T", values[0])
	}
	return address, nil
}

func CheckoutWalletSalt(paymentIntentID, chainName string) [32]byte {
	return crypto.Keccak256Hash(
		[]byte("cpay.checkout.wallet.v1"),
		[]byte(strings.TrimSpace(paymentIntentID)),
		[]byte(normalizeEVMChain(chainName)),
	)
}

func PredictCreate2Address(factory common.Address, salt [32]byte, initCodeHash common.Hash) common.Address {
	return crypto.CreateAddress2(factory, salt, initCodeHash.Bytes())
}

func normalizeStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := normalizeEVMChain(k)
		val := strings.TrimSpace(v)
		if key != "" && val != "" {
			out[key] = val
		}
	}
	return out
}

func mustParseCheckoutWalletFactoryABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(checkoutWalletFactoryABI))
	if err != nil {
		panic(err)
	}
	return parsed
}
