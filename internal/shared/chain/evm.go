package chain

import (
	"context"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

type EVMAdapter struct {
	confirmations map[string]int
}

func NewEVMAdapter(confirmations map[string]int) *EVMAdapter {
	return &EVMAdapter{confirmations: confirmations}
}

func (a *EVMAdapter) GenerateDepositWallet(_ context.Context, _ string) (DepositWallet, error) {
	pk, err := crypto.GenerateKey()
	if err != nil {
		return DepositWallet{}, err
	}
	addr := crypto.PubkeyToAddress(pk.PublicKey)
	pkBytes := crypto.FromECDSA(pk)
	return DepositWallet{Address: addr.Hex(), PrivateKeyHex: "0x" + strings.ToLower(commonBytesToHex(pkBytes))}, nil
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
