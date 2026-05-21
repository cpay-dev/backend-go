package chain

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestCheckoutWalletSaltIsStable(t *testing.T) {
	got := CheckoutWalletSalt("01KQVSMQQ91EAZTX21N3JVQT8X", "HyperEVM")
	if common.Bytes2Hex(got[:]) != "c804a85e6a6a2948f45a593451abbb91ebd8731e83ec51fc1dcf6406ff22b355" {
		t.Fatalf("unexpected salt: 0x%x", got)
	}
}

func TestPredictCreate2AddressIsStable(t *testing.T) {
	factory := common.HexToAddress("0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe")
	salt := CheckoutWalletSalt("01KQVSMQQ91EAZTX21N3JVQT8X", "hyperevm")
	initCodeHash := common.HexToHash("0x4a07d2fd2f98bb68f0a57f9bdb6a170d9fbdc7c4ca011d2ad4c44f191f8b2b55")

	got := PredictCreate2Address(factory, salt, initCodeHash)
	if got.Hex() != "0xead10F37ce3fE85C352F5b9C0881BE078E80D83B" {
		t.Fatalf("unexpected create2 address: %s", got.Hex())
	}
}
