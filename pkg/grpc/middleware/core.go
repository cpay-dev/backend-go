package middleware

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewCore(logger zerolog.Logger, bypass []string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (ret interface{}, err error) {
		start := time.Now()
		ctx, cancel := context.WithCancel(ctx)
		defer func() {
			cancel()
			logCtx := logger.With()

			if methodPaths := strings.Split(info.FullMethod, "/"); len(methodPaths) == 3 {
				logCtx = logCtx.Str("service", methodPaths[1]).Str("method", methodPaths[2])
			}

			logger := logCtx.
				Str("path", info.FullMethod).
				Str("dur", time.Since(start).String()).
				Logger()

			if r := recover(); r != nil {
				var recErr error
				if val, ok := r.(error); ok {
					recErr = val
				} else {
					recErr = fmt.Errorf("%v", r)
				}
				ret = nil
				err = status.Error(codes.Internal, fmt.Sprintf("panic: %s", recErr.Error()))
				logger.WithLevel(zerolog.PanicLevel).Err(recErr).Any("err_details", err).Msg("handler panic")
			} else {
				code := status.Code(err)
				if code == codes.OK && slices.Contains(bypass, info.FullMethod) {
					return
				}
				var event *zerolog.Event
				switch code {
				case codes.Internal:
					event = logger.Error()
				default:
					event = logger.Info()
				}
				event = event.Uint32("code", uint32(code))
				if err != nil {
					if st, ok := status.FromError(err); ok {
						event = event.Str("desc", st.Message())
					} else {
						event = event.Str("info", err.Error()).Any("err_details", err)
					}

				}
				event.Msg("")
			}
		}()

		ret, err = handler(ctx, req)

		return
	}
}
