package grpcmw

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrPanic = errors.New("panic")

func UnaryCore() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if errors.Is(err, ErrPanic) {
			err = status.Error(codes.Internal, "")
		} else {
			if _, ok := status.FromError(err); !ok {
				err = status.Error(codes.Internal, "")
			}
		}
		return resp, err
	}
}

func UnaryPanicRecover() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(error); ok {
					err = e
				} else {
					err = fmt.Errorf("%v", r)
				}
				err = fmt.Errorf("%w: %v", ErrPanic, err)
			}
		}()
		return handler(ctx, req)
	}
}

func UnaryRequestLogger() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		log.Debug().Str("method", info.FullMethod).Msg("grpc: request start")
		resp, err := handler(ctx, req)
		dur := time.Since(start)
		var event *zerolog.Event
		if errors.Is(err, ErrPanic) {
			event = log.WithLevel(zerolog.PanicLevel)
		} else if err != nil {
			event = log.Error()
		} else {
			event = log.Debug()
		}
		event.Str("method", info.FullMethod).Dur("dur", dur).Err(err).Msg("grpc: request end")
		return resp, err
	}
}
