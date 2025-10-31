package authn

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type NoopJWKS struct{}

func (NoopJWKS) Keyfunc() jwt.Keyfunc            { return nil }
func (NoopJWKS) Bootstrap(_ time.Duration) error { return nil }
