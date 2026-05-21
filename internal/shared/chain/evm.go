package chain

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type EVMAdapter struct {
	confirmations map[string]int
	create2       *Create2WalletProvider
}

func NewEVMAdapter(confirmations map[string]int) *EVMAdapter {
	return &EVMAdapter{confirmations: confirmations}
}

func NewEVMAdapterWithCreate2(confirmations map[string]int, rpcURLs, factoryAddresses map[string]string) *EVMAdapter {
	return &EVMAdapter{
		confirmations: confirmations,
		create2:       NewCreate2WalletProvider(rpcURLs, factoryAddresses),
	}
}

func (a *EVMAdapter) GenerateDepositWallet(ctx context.Context, chainName string, paymentIntentID string) (DepositWallet, error) {
	if a.create2 != nil {
		wallet, ok, err := a.create2.DepositWallet(ctx, chainName, paymentIntentID)
		if err != nil {
			return DepositWallet{}, err
		}
		if ok {
			return wallet, nil
		}
	}
	return generateEOADepositWallet()
}

func generateEOADepositWallet() (DepositWallet, error) {
	pk, err := crypto.GenerateKey()
	if err != nil {
		return DepositWallet{}, err
	}
	addr := crypto.PubkeyToAddress(pk.PublicKey)
	pkBytes := crypto.FromECDSA(pk)
	return DepositWallet{
		Address:       addr.Hex(),
		PrivateKeyHex: "0x" + strings.ToLower(commonBytesToHex(pkBytes)),
		WalletType:    "eoa",
	}, nil
}

func (a *EVMAdapter) RequiredConfirmations(chain string) int {
	if v, ok := a.confirmations[strings.ToLower(chain)]; ok {
		return v
	}
	return 12
}

func (a *EVMAdapter) SupportedChains() []string {
	out := make([]string, 0, len(a.confirmations))
	for k := range a.confirmations {
		out = append(out, k)
	}
	if len(out) == 0 {
		out = append(out, "polygon")
	}
	return out
}

func commonBytesToHex(b []byte) string {
	const hexdigits = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = hexdigits[v>>4]
		dst[i*2+1] = hexdigits[v&0x0f]
	}
	return string(dst)
}

func normalizeEVMChain(chain string) string {
	return strings.ToLower(strings.TrimSpace(chain))
}

func requireHexAddress(label, address string) (common.Address, error) {
	if !common.IsHexAddress(address) {
		return common.Address{}, fmt.Errorf("%s is invalid", label)
	}
	return common.HexToAddress(address), nil
}
