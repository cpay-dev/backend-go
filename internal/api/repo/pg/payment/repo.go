package payment

import (
	"github.com/cpay-dev/backend-go/pkg/db"
)

type PostgresRepo struct {
	db.PgxPoolWrapper
}

func NewPostgresRepo(pool db.PgxPoolWrapper) *PostgresRepo {
	return &PostgresRepo{PgxPoolWrapper: pool}
}
