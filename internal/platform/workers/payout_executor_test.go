package workers

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type fakeEVMClient struct {
	estimateErr error
	callErr     error
	sentTx      *types.Transaction
	lastCall    ethereum.CallMsg
}

func (c *fakeEVMClient) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 7, nil
}

func (c *fakeEVMClient) SuggestGasPrice(context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (c *fakeEVMClient) EstimateGas(_ context.Context, msg ethereum.CallMsg) (uint64, error) {
	c.lastCall = msg
	if c.estimateErr != nil {
		return 0, c.estimateErr
	}
	return 50_000, nil
}

func (c *fakeEVMClient) SendTransaction(_ context.Context, tx *types.Transaction) error {
	c.sentTx = tx
	return nil
}

func (c *fakeEVMClient) ChainID(context.Context) (*big.Int, error) {
	return big.NewInt(1337), nil
}

func (c *fakeEVMClient) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	if c.callErr != nil {
		return nil, c.callErr
	}
	return common.LeftPadBytes([]byte{6}, 32), nil
}

func (c *fakeEVMClient) Close() {}

func TestDecimalToBaseUnits(t *testing.T) {
	got, err := decimalToBaseUnits("1.230000", 6)
	if err != nil {
		t.Fatalf("decimalToBaseUnits returned error: %v", err)
	}
	if got.String() != "1230000" {
		t.Fatalf("unexpected base units: %s", got)
	}
}

func TestDecimalToBaseUnitsRejectsPrecisionLoss(t *testing.T) {
	if _, err := decimalToBaseUnits("1.0000001", 6); err == nil {
		t.Fatalf("expected precision error")
	}
}

func TestEVMPayoutExecutorSendsNativeTransfer(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	client := &fakeEVMClient{}
	executor := &EVMPayoutExecutor{clients: map[string]evmRPCClient{"base": client}}

	result, err := executor.ExecutePayout(context.Background(), PayoutExecutionRequest{
		Chain:                "base",
		AmountRaw:            "0.5",
		DepositPrivateKeyHex: "0x" + common.Bytes2Hex(crypto.FromECDSA(privateKey)),
		SettlementAddress:    "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe",
	})
	if err != nil {
		t.Fatalf("ExecutePayout returned error: %v", err)
	}
	if result.TxHash == "" {
		t.Fatalf("expected tx hash")
	}
	if client.sentTx == nil {
		t.Fatalf("expected transaction to be sent")
	}
	if client.sentTx.Value().String() != "500000000000000000" {
		t.Fatalf("unexpected native value: %s", client.sentTx.Value())
	}
}

func TestEVMPayoutExecutorSendsERC20Transfer(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	client := &fakeEVMClient{}
	executor := &EVMPayoutExecutor{clients: map[string]evmRPCClient{"base": client}}

	_, err = executor.ExecutePayout(context.Background(), PayoutExecutionRequest{
		Chain:                "base",
		TokenAddress:         "0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
		AmountRaw:            "12.34",
		DepositPrivateKeyHex: "0x" + common.Bytes2Hex(crypto.FromECDSA(privateKey)),
		SettlementAddress:    "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe",
	})
	if err != nil {
		t.Fatalf("ExecutePayout returned error: %v", err)
	}
	if client.sentTx == nil {
		t.Fatalf("expected transaction to be sent")
	}
	if client.sentTx.Value().Sign() != 0 {
		t.Fatalf("expected zero native value for erc20 transfer")
	}
	if !strings.HasPrefix(common.Bytes2Hex(client.sentTx.Data()), "a9059cbb") {
		t.Fatalf("expected erc20 transfer calldata, got %x", client.sentTx.Data())
	}
}

func TestEVMPayoutExecutorFailsWhenGasIsMissing(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	client := &fakeEVMClient{estimateErr: errors.New("insufficient funds for gas")}
	executor := &EVMPayoutExecutor{clients: map[string]evmRPCClient{"base": client}}

	_, err = executor.ExecutePayout(context.Background(), PayoutExecutionRequest{
		Chain:                "base",
		TokenAddress:         "0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
		AmountRaw:            "12.34",
		DepositPrivateKeyHex: "0x" + common.Bytes2Hex(crypto.FromECDSA(privateKey)),
		SettlementAddress:    "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe",
	})
	if err == nil || !strings.Contains(err.Error(), "insufficient funds") {
		t.Fatalf("expected insufficient funds error, got %v", err)
	}
}
