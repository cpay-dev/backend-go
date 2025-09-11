package authn

import (
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/app"
)

type AuthnService struct {
	repo *app.PostgresRepo
}

func NewService(repo *app.PostgresRepo) *AuthnService {
	return &AuthnService{repo: repo}
}
