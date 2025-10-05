package authn

import (
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/app"
)

type AuthnService struct {
	appRepo *app.PostgresRepo
}

func NewService(appRepo *app.PostgresRepo) *AuthnService {
	return &AuthnService{appRepo: appRepo}
}
