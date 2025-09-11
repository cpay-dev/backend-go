package config

type Server struct {
	ListenAddress string `json:"listen_address" env:"SERVER_LISTEN_ADDRESS,notEmpty" envDefault:"0.0.0.0:8080"`
}
