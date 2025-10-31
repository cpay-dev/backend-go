FROM golang:1.25.3 AS build
WORKDIR /

COPY go.mod go.mod
COPY go.sum go.sum

RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
  go mod download && go mod verify

COPY pkg pkg
COPY internal internal
COPY cmd cmd

ARG PACKAGE
ENV CGO_ENABLED=0 GOAMD64=v3
ENV GOCACHE=/gocache

RUN --mount=type=cache,id=gobuild,target=/gocache \
  go build \
  -trimpath -buildvcs=false -mod=readonly \
  -ldflags="-s -w" \
  -o /app $PACKAGE

FROM gcr.io/distroless/static:nonroot
WORKDIR /
USER nonroot:nonroot
COPY --from=build --chown=nonroot:nonroot /app /app
ENTRYPOINT ["/app"]
