package api

import (
	"context"

	"github.com/cpay-dev/backend-go/pkg/grpc/middleware"
	"google.golang.org/grpc/metadata"
)

func ContextWithApiKey(ctx context.Context, apiKey string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(make(map[string]string))
	}
	md.Set(middleware.ApiKeyHeaderKey, apiKey)
	return metadata.NewOutgoingContext(ctx, md)
}
