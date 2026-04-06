.PHONY: tidy fmt test run migrate up down proto proto-docker

tidy:
	GOCACHE=/tmp/go-build GOPATH=/tmp/go go mod tidy

proto:
	./scripts/gen-proto.sh

proto-docker:
	docker build -f docker/proto.Dockerfile -t cpay-proto-gen .
	docker run --rm -v "$$PWD":/workspace -w /workspace cpay-proto-gen \
		bash -lc "protoc -I proto --go_out=paths=source_relative:internal/gen --go-grpc_out=paths=source_relative:internal/gen proto/cpay/v1/auth.proto proto/cpay/v1/payment_link.proto proto/cpay/v1/checkout.proto"

fmt:
	GOCACHE=/tmp/go-build GOPATH=/tmp/go go fmt ./...

test:
	GOCACHE=/tmp/go-build GOPATH=/tmp/go go test ./...

run:
	GOCACHE=/tmp/go-build GOPATH=/tmp/go go run ./cmd/api-gateway

migrate:
	GOCACHE=/tmp/go-build GOPATH=/tmp/go go run ./cmd/migrate

up:
	docker compose up -d --build

down:
	docker compose down
