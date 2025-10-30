package authn

import "github.com/rs/zerolog"

type ProvidersConfig struct {
	Google GoogleProvider
}

type GoogleProvider struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
}

type ServerConfig struct {
	AuthService AuthService
	Logger      zerolog.Logger
}
