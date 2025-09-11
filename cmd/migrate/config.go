package main

import "github.com/cpay-dev/backend-go/pkg/db/migration"

type Schema = string

const (
	SchemaApp        Schema = "app"
	SchemaBlockchain Schema = "blockchain"
)

type Config struct {
	migration.Config
	Schema string `json:"schema"`
}
