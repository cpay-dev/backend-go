FROM golang:1.25.1 AS build
WORKDIR /

COPY go.mod go.mod
COPY go.sum go.sum

RUN --mount=type=cache,id=go-build,target=/go/pkg/mod \
    go mod download && go mod verify

COPY pkg pkg
COPY internal internal
COPY cmd cmd

ARG PACKAGE
RUN --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /app $PACKAGE

FROM gcr.io/distroless/static
WORKDIR /
USER 1000:1000
COPY --from=build --chown=1000:1000 /app /app
ENV USER=nobody
ENTRYPOINT ["/app"]
