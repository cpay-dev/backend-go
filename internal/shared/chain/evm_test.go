package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRequiredCreate2AdapterCreatesCreate2Wallet(t *testing.T) {
	predicted := common.HexToAddress("0x386d43Ec19E11aE2Ad0d9aB97955c510804Fd510")
	factory := "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"
	adapter := &EVMAdapter{
		confirmations:  map[string]int{"hyperevm": 1},
		requireCreate2: true,
		create2: &Create2WalletProvider{
			rpcURLs:          map[string]string{"hyperevm": "http://127.0.0.1:8545"},
			factoryAddresses: map[string]string{"hyperevm": factory},
			predictWallet: func(_ context.Context, rpcURL string, gotFactory common.Address, salt [32]byte) (common.Address, error) {
				if rpcURL == "" {
					t.Fatalf("expected rpc url")
				}
				if gotFactory != common.HexToAddress(factory) {
					t.Fatalf("unexpected factory: %s", gotFactory.Hex())
				}
				if salt == ([32]byte{}) {
					t.Fatalf("expected non-empty salt")
				}
				return predicted, nil
			},
		},
	}

	wallet, err := adapter.GenerateDepositWallet(context.Background(), "hyperevm", "pi_123")
	if err != nil {
		t.Fatalf("generate wallet: %v", err)
	}
	if wallet.WalletType != WalletTypeCreate2 {
		t.Fatalf("expected create2 wallet type, got %q", wallet.WalletType)
	}
	if wallet.Address != predicted.Hex() {
		t.Fatalf("unexpected address: %s", wallet.Address)
	}
	if wallet.PrivateKeyHex != "" {
		t.Fatalf("expected no private key, got %q", wallet.PrivateKeyHex)
	}
	if wallet.FactoryAddress != common.HexToAddress(factory).Hex() || wallet.WalletSalt == "" {
		t.Fatalf("expected create2 metadata, got %+v", wallet)
	}
}

func TestRequiredCreate2AdapterFailsWithoutConfiguredFactories(t *testing.T) {
	adapter := NewEVMAdapterWithRequiredCreate2(map[string]int{"hyperevm": 1}, map[string]string{"hyperevm": "http://127.0.0.1:8545"}, nil)

	wallet, err := adapter.GenerateDepositWallet(context.Background(), "hyperevm", "pi_123")
	if err == nil {
		t.Fatalf("expected missing factory error, got wallet %+v", wallet)
	}
	if !strings.Contains(err.Error(), "checkout wallet factory addresses are not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequiredCreate2AdapterFailsForChainWithoutFactory(t *testing.T) {
	adapter := NewEVMAdapterWithRequiredCreate2(
		map[string]int{"hyperevm": 1, "base": 12},
		map[string]string{"hyperevm": "http://127.0.0.1:8545", "base": "http://127.0.0.1:8546"},
		map[string]string{"hyperevm": "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"},
	)

	wallet, err := adapter.GenerateDepositWallet(context.Background(), "base", "pi_123")
	if err == nil {
		t.Fatalf("expected missing chain factory error, got wallet %+v", wallet)
	}
	if !strings.Contains(err.Error(), "checkout wallet factory for base is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreate2AdapterStillAllowsEOAFallbackWhenNotRequired(t *testing.T) {
	adapter := NewEVMAdapterWithCreate2(
		map[string]int{"hyperevm": 1},
		map[string]string{"hyperevm": "http://127.0.0.1:8545"},
		map[string]string{"base": "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"},
	)

	wallet, err := adapter.GenerateDepositWallet(context.Background(), "hyperevm", "pi_123")
	if err != nil {
		t.Fatalf("generate wallet: %v", err)
	}
	if wallet.WalletType != WalletTypeEOA || wallet.PrivateKeyHex == "" {
		t.Fatalf("expected EOA fallback wallet, got %+v", wallet)
	}
}
