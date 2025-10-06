package db

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPgxTlsConfigIgnored = errors.New("pgx: config ignores tls options")

type PgxConnectionCtxKey struct{}

type PgxConnection interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, rowSrc pgx.CopyFromSource) (int64, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type PgxPoolWrapper interface {
	RunInTx(ctx context.Context, fn func(context.Context) error) error
	RunInTxWithOptions(ctx context.Context, opt pgx.TxOptions, fn func(context.Context) error) error
	GetConnectionFromCtx(ctx context.Context) PgxConnection
}

func NewPgxPoolFromConn(
	ctx context.Context,
	conn string,
	tracer pgx.QueryTracer,
	tls *tls.Config,
) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(conn)
	if err != nil {
		return nil, fmt.Errorf("pgx: parse config: %w", err)
	}
	config.ConnConfig.Tracer = tracer
	if tls != nil {
		if config.ConnConfig.TLSConfig == nil {
			return nil, ErrPgxTlsConfigIgnored
		}
		config.ConnConfig.TLSConfig.InsecureSkipVerify = tls.InsecureSkipVerify
		config.ConnConfig.TLSConfig.ServerName = tls.ServerName
		config.ConnConfig.TLSConfig.RootCAs = tls.RootCAs
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("pgx: new pool: %w", err)
	}
	return pool, nil
}

func NewPgxPoolWrappedFromConn(
	ctx context.Context,
	conn string,
	tracer pgx.QueryTracer,
	tls *tls.Config,
) (PgxPoolWrapper, error) {
	pool, err := NewPgxPoolFromConn(ctx, conn, tracer, tls)
	if err != nil {
		return nil, err
	}
	return NewPgxPoolWrapper(pool), nil
}

func NewPgxPoolWrapper(pool *pgxpool.Pool) PgxPoolWrapper {
	return &pgxPoolWrapper{pool: pool}
}

type pgxPoolWrapper struct {
	pool *pgxpool.Pool
}

func (r *pgxPoolWrapper) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ctx = context.WithValue(ctx, PgxConnectionCtxKey{}, tx)
	err = fn(ctx)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *pgxPoolWrapper) RunInTxWithOptions(
	ctx context.Context, opts pgx.TxOptions, fn func(context.Context) error,
) error {
	tx, err := r.pool.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ctx = context.WithValue(ctx, PgxConnectionCtxKey{}, tx)
	err = fn(ctx)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *pgxPoolWrapper) GetConnectionFromCtx(ctx context.Context) PgxConnection {
	conn, ok := ctx.Value(PgxConnectionCtxKey{}).(PgxConnection)
	if !ok {
		return r.pool
	}
	return conn
}
