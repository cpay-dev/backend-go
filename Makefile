.PHONY: dev dev-db migrate migrate-down migrate-reset sqlc proto compile-sol build test clean

# Development
dev:
	docker compose up --build

dev-db:
	docker compose up postgres nats minio minio-init

# Database
DATABASE_URL ?= postgres://cpay_admin:devpassword@localhost:5432/cpay?sslmode=disable

migrate:
	docker run --rm --network host -v "$(PWD)/migrations:/migrations" migrate/migrate:v4.17.0 \
		-path /migrations -database "$(DATABASE_URL)" up

migrate-down:
	docker run --rm --network host -v "$(PWD)/migrations:/migrations" migrate/migrate:v4.17.0 \
		-path /migrations -database "$(DATABASE_URL)" down 1

migrate-reset:
	docker run --rm --network host -v "$(PWD)/migrations:/migrations" migrate/migrate:v4.17.0 \
		-path /migrations -database "$(DATABASE_URL)" drop -f
	docker run --rm --network host -v "$(PWD)/migrations:/migrations" migrate/migrate:v4.17.0 \
		-path /migrations -database "$(DATABASE_URL)" up

# Code Generation
sqlc:
	docker run --rm -v "$(PWD)/backend:/src" -v "$(PWD)/migrations:/migrations" -w /src sqlc/sqlc:1.28.0 generate

proto:
	docker run --rm -v "$(PWD):/workspace" -w /workspace namely/protoc-all:1.51_2 \
		-f proto/chain/v1/chain_service.proto \
		-l go -o backend/internal/grpc/gen/chainv1 \
		--go-source-relative

compile-sol:
	docker run --rm -v "$(PWD)/contracts:/contracts" ethereum/solc:0.8.20 \
		--optimize --optimize-runs 1 --bin --abi -o /contracts/out /contracts/PaymentWallet.sol

# Build
build-go:
	cd backend && go build -o bin/server ./cmd/server

build-rust:
	cd chain-service && cargo build --release

build: build-go build-rust

# Test
test-go:
	cd backend && go test ./...

test-rust:
	cd chain-service && cargo test

test: test-go test-rust

# Frontend
frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build

# Clean
clean:
	rm -rf backend/bin
	cd chain-service && cargo clean
	rm -rf frontend/.next
