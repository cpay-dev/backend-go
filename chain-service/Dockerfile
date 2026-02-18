FROM rust:1.85-slim AS builder

RUN apt-get update && apt-get install -y protobuf-compiler pkg-config libssl-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY chain-service/ ./chain-service/
COPY proto/ ./proto/

WORKDIR /app/chain-service
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/chain-service/target/release/chain-service /chain-service
EXPOSE 50051
CMD ["/chain-service"]
