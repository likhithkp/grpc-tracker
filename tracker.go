package grpctracker

import (
	"context"
	"log"

	"go.uber.org/fx"
	"google.golang.org/grpc"
)

// UnaryInterceptor logs and can modify unary gRPC calls
func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		log.Printf("[GRPC Tracker] Request - Method: %s, Payload: %+v\n", info.FullMethod, req)
		resp, err := handler(ctx, req)
		log.Printf("[GRPC Tracker] Response - Method: %s, Payload: %+v, Error: %v\n", info.FullMethod, resp, err)
		return resp, err
	}
}

// StreamInterceptor logs streaming gRPC calls
func StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		log.Printf("[GRPC Tracker] Stream - Method: %s\n", info.FullMethod)
		return handler(srv, ss)
	}
}

// FxModule provides interceptors for automatic injection
func FxModule() fx.Option {
	return fx.Options(
		fx.Provide(
			func() grpc.UnaryServerInterceptor {
				return UnaryInterceptor()
			},
			func() grpc.StreamServerInterceptor {
				return StreamInterceptor()
			},
		),
	)
}
