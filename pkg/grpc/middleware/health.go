package middleware

var DefaultHealthBypass = []string{
	"/grpc.health.v1.Health/Check",
	"/grpc.health.v1.Health/Watch",
	"/grpc.health.v1.Health/List",
}
