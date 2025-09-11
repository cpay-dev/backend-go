package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type (
	UserStatus       string
	UserIdentityType string
)

const (
	UserStatusActive   UserStatus = "ACTIVE"
	UserStatusInactive UserStatus = "INACTIVE"
	UserStatusBanned   UserStatus = "BANNED"

	UserIdentityTypeEmail  UserIdentityType = "EMAIL"
	UserIdentityTypeWallet UserIdentityType = "WALLET"
)

type User struct {
	ID        string     `db:"id"`
	Status    UserStatus `db:"status"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt time.Time  `db:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

type UserIdentity struct {
	ID           string           `db:"id"`
	UserID       string           `db:"user_id"`
	IdentityType UserIdentityType `db:"identity_type"`
	Identity     string           `db:"identity"`
	CreatedAt    time.Time        `db:"created_at"`
	UpdatedAt    time.Time        `db:"updated_at"`
	DeletedAt    *time.Time       `db:"deleted_at"`
}

func (r *PostgresRepo) CreateUser(ctx context.Context, user User) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO app.users (id, status, created_at, updated_at)
		VALUES (@id, @status, NOW(), NOW());
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"id":     user.ID,
		"status": user.Status,
	})
	return err
}

func (r *PostgresRepo) CreateUserIdentity(ctx context.Context, identity UserIdentity) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO app.user_identities (id, user_id, identity_type, identity, created_at, updated_at)
		VALUES (@id, @user_id, @identity_type, @identity, NOW(), NOW());
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"id":            identity.ID,
		"user_id":       identity.UserID,
		"identity_type": identity.IdentityType,
		"identity":      identity.Identity,
	})
	return err
}
