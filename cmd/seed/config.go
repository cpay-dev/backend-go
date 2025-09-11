package main

import (
	"github.com/cpay-dev/backend-go/pkg/config"
)

type Schema = string

const (
	SchemaApp        Schema = "app"
	SchemaBlockchain Schema = "blockchain"
)

type Config struct {
	Database config.Database
	Schema   Schema `json:"schema"`
}
