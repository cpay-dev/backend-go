package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/ethereum/go-ethereum/common"
)

func TestMerchantSettingsRequiresUserToken(t *testing.T) {
	s := &Server{
		authClient: newAuthTestClient(t, &authMiddlewareTestServer{
			validateFn: func(context.Context, *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
				t.Fatal("ValidateCredential should not be called for missing credentials")
				return nil, nil
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/merchant/settings", strings.NewReader(`{"settlement_address":"0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"}`))
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMerchantSettingsRejectsAPIKeyPrincipal(t *testing.T) {
	merchantID := ids.New()
	s := &Server{
		authClient: newAuthTestClient(t, &authMiddlewareTestServer{
			validateFn: func(context.Context, *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
				return &cpayv1.ValidateCredentialResponse{Principal: &cpayv1.Principal{
					MerchantId: merchantID,
					Role:       "api_key",
					ApiKeyId:   ids.New(),
					IsUser:     false,
				}}, nil
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/merchant/settings", strings.NewReader(`{"settlement_address":"0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"}`))
	req.Header.Set("X-API-Key", "cpay_test")
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestUpdateMerchantSettingsRejectsInvalidAddress(t *testing.T) {
	merchantID := ids.New()
	s := &Server{
		authClient: newAuthTestClient(t, &authMiddlewareTestServer{
			validateFn: func(context.Context, *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
				return &cpayv1.ValidateCredentialResponse{Principal: &cpayv1.Principal{
					MerchantId: merchantID,
					UserId:     ids.New(),
					Role:       "admin",
					IsUser:     true,
				}}, nil
			},
			updateMerchantSettingsFn: func(context.Context, *cpayv1.UpdateMerchantSettingsRequest) (*cpayv1.UpdateMerchantSettingsResponse, error) {
				t.Fatal("UpdateMerchantSettings should not be called for invalid address")
				return nil, nil
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/merchant/settings", strings.NewReader(`{"settlement_address":"not-a-wallet"}`))
	req.Header.Set("Authorization", "Bearer test")
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestUpdateMerchantSettingsStoresChecksumAddress(t *testing.T) {
	merchantID := ids.New()
	rawAddress := "0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae"
	checksumAddress := common.HexToAddress(rawAddress).Hex()
	var gotAddress string

	s := &Server{
		authClient: newAuthTestClient(t, &authMiddlewareTestServer{
			validateFn: func(context.Context, *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
				return &cpayv1.ValidateCredentialResponse{Principal: &cpayv1.Principal{
					MerchantId: merchantID,
					UserId:     ids.New(),
					Role:       "admin",
					IsUser:     true,
				}}, nil
			},
			updateMerchantSettingsFn: func(_ context.Context, req *cpayv1.UpdateMerchantSettingsRequest) (*cpayv1.UpdateMerchantSettingsResponse, error) {
				if req.GetMerchantId() != merchantID {
					t.Fatalf("unexpected merchant id: %s", req.GetMerchantId())
				}
				gotAddress = req.GetSettlementAddress()
				return &cpayv1.UpdateMerchantSettingsResponse{Settings: &cpayv1.MerchantSettings{
					MerchantId:        merchantID,
					SettlementAddress: req.GetSettlementAddress(),
				}}, nil
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/merchant/settings", strings.NewReader(`{"settlement_address":"`+rawAddress+`"}`))
	req.Header.Set("Authorization", "Bearer test")
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotAddress != checksumAddress {
		t.Fatalf("expected checksum address %s, got %s", checksumAddress, gotAddress)
	}
}

func TestGetMerchantSettingsReturnsCurrentSettings(t *testing.T) {
	merchantID := ids.New()
	settlementAddress := "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"
	s := &Server{
		authClient: newAuthTestClient(t, &authMiddlewareTestServer{
			validateFn: func(context.Context, *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
				return &cpayv1.ValidateCredentialResponse{Principal: &cpayv1.Principal{
					MerchantId: merchantID,
					UserId:     ids.New(),
					Role:       "admin",
					IsUser:     true,
				}}, nil
			},
			getMerchantSettingsFn: func(_ context.Context, req *cpayv1.GetMerchantSettingsRequest) (*cpayv1.GetMerchantSettingsResponse, error) {
				if req.GetMerchantId() != merchantID {
					t.Fatalf("unexpected merchant id: %s", req.GetMerchantId())
				}
				return &cpayv1.GetMerchantSettingsResponse{Settings: &cpayv1.MerchantSettings{
					MerchantId:        merchantID,
					SettlementAddress: settlementAddress,
				}}, nil
			},
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/merchant/settings", nil)
	req.Header.Set("Authorization", "Bearer test")
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), settlementAddress) {
		t.Fatalf("response does not include settlement address: %s", rr.Body.String())
	}
}
