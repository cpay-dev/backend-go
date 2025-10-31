package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/goccy/go-json"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/cpay-dev/backend-go/internal/authn"
	"github.com/cpay-dev/backend-go/pkg/appenv"
	"github.com/cpay-dev/backend-go/pkg/sig"
	valkey "github.com/valkey-io/valkey-go"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("authn service failed")
	}
}

func run() error {
	flag.String("config", "./config.json", "path to config file")
	flag.Parse()

	cfg, err := LoadServiceConfig()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.InterfaceMarshalFunc = json.Marshal
	zerolog.DurationFieldInteger = true
	zerolog.DurationFieldUnit = time.Microsecond
	configureWriters(cfg.Environment)
	zerolog.SetGlobalLevel(cfg.LogLevel)

	lis, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.ListenAddress, err)
	}
	defer func() {
		if err := lis.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Error().Err(err).Msg("listen: close listener")
		}
	}()

	vkc, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{cfg.Valkey.Address},
		Password:    cfg.Valkey.Password,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	})
	if err != nil {
		return fmt.Errorf("valkey: new client: %w", err)
	}
	defer vkc.Close()

	store := authn.NewValkeyInitStateStore(authn.NewValkeyKVAdapter(vkc))
	authService := authn.NewOAuthProviderService(authn.ProvidersConfig{
		Google: authn.GoogleProvider{
			ClientID:     cfg.Providers.Google.ClientID,
			ClientSecret: cfg.Providers.Google.ClientSecret,
			RedirectURI:  cfg.Providers.Google.RedirectURI,
			Scopes:       cfg.Providers.Google.Scopes,
		},
	}, store)

	srv := authn.NewServer(authn.ServerConfig{
		AuthService: authService,
		Logger:      log.Logger,
	})

	gs := grpc.NewServer(grpc.ChainUnaryInterceptor(authn.DefaultMiddleware()...))
	srv.Register(gs)
	srv.MarkReady()

	go func() {
		log.Info().Str("listen", cfg.ListenAddress).Msg("authn grpc serving")
		if err := gs.Serve(lis); err != nil {
			log.Error().Err(err).Msg("grpc server exited")
		}
	}()

	sig.WaitForTermination()
	srv.Shutdown(gs)
	return nil
}

func configureWriters(e appenv.Environment) {
	if e == appenv.EnvLocal {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339Nano})
		return
	}
	log.Logger = zerolog.New(appenv.SplitLevelWriter{}).With().Timestamp().Logger()
}
