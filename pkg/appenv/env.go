package appenv

import (
	"errors"
	"strings"
)

type Environment uint8

const (
	EnvLocal Environment = iota
	EnvDev
	EnvStg
	EnvProd
)

func (e Environment) String() string {
	switch e {
	case EnvDev:
		return "dev"
	case EnvStg:
		return "staging"
	case EnvProd:
		return "prod"
	default:
		return "local"
	}
}

// MarshalText implements encoding.TextMarshaler.
func (e Environment) MarshalText() ([]byte, error) { return []byte(e.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler and treats empty string as local.
func (e *Environment) UnmarshalText(text []byte) error {
	s := strings.ToLower(strings.TrimSpace(string(text)))
	if s == "" {
		*e = EnvLocal
		return nil
	}
	switch s {
	case "local":
		*e = EnvLocal
	case "dev":
		*e = EnvDev
	case "staging":
		*e = EnvStg
	case "prod":
		*e = EnvProd
	default:
		return errors.New("unknown environment: " + s)
	}
	return nil
}
