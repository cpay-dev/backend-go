package authsvc

import (
	"context"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"google.golang.org/grpc/codes"
)

func (s *Service) GetProfileSecurity(ctx context.Context, req *cpayv1.ProfileSecurityRequest) (*cpayv1.ProfileSecurityResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	return s.profileSecurity(ctx, userID, merchantID)
}

func (s *Service) DeleteIdentity(ctx context.Context, req *cpayv1.DeleteIdentityRequest) (*cpayv1.DeleteAuthMethodResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err = s.ensureAlternativeAuthMethod(ctx, userID, merchantID, "identity", id); err != nil {
		return nil, err
	}
	cmd, err := s.db.Exec(ctx, `DELETE FROM auth.identities WHERE id=$1 AND user_id=$2 AND merchant_id=$3`, id, userID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to delete identity")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "identity not found")
	}
	return &cpayv1.DeleteAuthMethodResponse{Id: id, Deleted: true}, nil
}

func (s *Service) DeletePasskey(ctx context.Context, req *cpayv1.DeletePasskeyRequest) (*cpayv1.DeleteAuthMethodResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err = s.ensureAlternativeAuthMethod(ctx, userID, merchantID, "passkey", id); err != nil {
		return nil, err
	}
	cmd, err := s.db.Exec(ctx, `DELETE FROM auth.passkey_credentials WHERE id=$1 AND user_id=$2 AND merchant_id=$3`, id, userID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to delete passkey")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "passkey not found")
	}
	return &cpayv1.DeleteAuthMethodResponse{Id: id, Deleted: true}, nil
}

func (s *Service) profileSecurity(ctx context.Context, userID, merchantID string) (*cpayv1.ProfileSecurityResponse, error) {
	identities := []*cpayv1.Identity{}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, provider, provider_subject, COALESCE(email, ''), COALESCE(display_name, ''), created_at
		FROM auth.identities
		WHERE user_id=$1 AND merchant_id=$2
		ORDER BY created_at DESC
	`, userID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to list identities")
	}
	defer rows.Close()
	for rows.Next() {
		var item cpayv1.Identity
		var createdAt time.Time
		if err = rows.Scan(&item.Id, &item.Provider, &item.ProviderSubject, &item.Email, &item.DisplayName, &createdAt); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to scan identity")
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		identities = append(identities, &item)
	}

	passkeys := []*cpayv1.PasskeyCredentialInfo{}
	rows, err = s.db.Query(ctx, `
		SELECT id::text, credential_id, COALESCE(name, ''), created_at, last_used_at
		FROM auth.passkey_credentials
		WHERE user_id=$1 AND merchant_id=$2
		ORDER BY created_at DESC
	`, userID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to list passkeys")
	}
	defer rows.Close()
	for rows.Next() {
		var item cpayv1.PasskeyCredentialInfo
		var createdAt time.Time
		var lastUsedAt *time.Time
		if err = rows.Scan(&item.Id, &item.CredentialId, &item.Name, &createdAt, &lastUsedAt); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to scan passkey")
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		item.LastUsedAt = formatTimePtr(lastUsedAt)
		passkeys = append(passkeys, &item)
	}
	return &cpayv1.ProfileSecurityResponse{Identities: identities, Passkeys: passkeys}, nil
}

func (s *Service) lookupUserAuth(ctx context.Context, userID string) (string, string, string, error) {
	var merchantID, role, email string
	err := s.db.QueryRow(ctx, `SELECT merchant_id::text, role, lower(email) FROM auth.users WHERE id=$1`, userID).Scan(&merchantID, &role, &email)
	if err != nil {
		return "", "", "", rpcx.E(codes.NotFound, "not_found", "user not found")
	}
	return merchantID, role, email, nil
}

func (s *Service) ensureAlternativeAuthMethod(ctx context.Context, userID, merchantID, deletingKind, deletingID string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.users WHERE id=$1 AND merchant_id=$2)`, userID, merchantID).Scan(&exists); err != nil || !exists {
		return rpcx.E(codes.NotFound, "not_found", "user not found")
	}
	var identities, passkeys int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auth.identities WHERE user_id=$1 AND merchant_id=$2 AND ($3 <> 'identity' OR id::text <> $4)`, userID, merchantID, deletingKind, deletingID).Scan(&identities)
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auth.passkey_credentials WHERE user_id=$1 AND merchant_id=$2 AND ($3 <> 'passkey' OR id::text <> $4)`, userID, merchantID, deletingKind, deletingID).Scan(&passkeys)
	if identities == 0 && passkeys == 0 {
		return rpcx.E(codes.FailedPrecondition, "last_auth_method", "add another sign-in method before removing this one")
	}
	return nil
}
