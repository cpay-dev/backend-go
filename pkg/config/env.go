package config

type Environment int

const (
	EnvironmentLocal Environment = 0
	EnvironmentStage Environment = 1
	EnvironmentProd  Environment = 2
)

type DefaultEnvironment struct {
	Value Environment `json:"environment" env:"ENVIRONMENT,notEmpty" envDefault:"0"`
}
