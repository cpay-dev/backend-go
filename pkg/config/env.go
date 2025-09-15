package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Environment int

const (
	EnvironmentLocal Environment = 0
	EnvironmentStage Environment = 1
	EnvironmentProd  Environment = 2
)

type DefaultEnvironment struct {
	Value Environment `json:"environment" env:"ENVIRONMENT,notEmpty" envDefault:"0"`
}

func (e Environment) String() string {
	switch e {
	case EnvironmentLocal:
		return "local"
	case EnvironmentStage:
		return "stage"
	case EnvironmentProd:
		return "prod"
	default:
		return fmt.Sprintf("unknown(%d)", int(e))
	}
}

func (e Environment) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.String())
}

func (e *Environment) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return e.UnmarshalText([]byte(s))
	}

	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		return e.fromInt(n)
	}

	return fmt.Errorf("invalid environment: %s", string(data))
}

func (e Environment) MarshalText() ([]byte, error) {
	return []byte(e.String()), nil
}

func (e *Environment) UnmarshalText(text []byte) error {
	v := strings.TrimSpace(strings.ToLower(string(text)))
	if v == "" {
		return fmt.Errorf("environment cannot be empty")
	}
	switch v {
	case "local":
		*e = EnvironmentLocal
		return nil
	case "stage":
		*e = EnvironmentStage
		return nil
	case "prod":
		*e = EnvironmentProd
		return nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		return e.fromInt(n)
	}
	return fmt.Errorf("invalid environment: %q", v)
}

func (e *Environment) fromInt(n int) error {
	switch n {
	case int(EnvironmentLocal):
		*e = EnvironmentLocal
		return nil
	case int(EnvironmentStage):
		*e = EnvironmentStage
		return nil
	case int(EnvironmentProd):
		*e = EnvironmentProd
		return nil
	default:
		return fmt.Errorf("invalid environment value: %d", n)
	}
}
