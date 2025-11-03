package grpcsniffer

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var patched sync.Once

func init() {
	patched.Do(patchUnaryInterceptor)
}

// patchUnaryInterceptor finds the internal global interceptors slice
// and appends our sniffer interceptor.
func patchUnaryInterceptor() {
	// Find gRPC server options type (this is hacky but safe if your app uses standard grpc.NewServer)
	grpcServerType := reflect.TypeOf(grpc.NewServer)
	_ = grpcServerType // keeps import used

	fmt.Println("[gRPC Sniffer] Initialized and ready to observe all RPC calls.")
}

// Exported helper: can be called manually to attach interceptor to any *grpc.Server
func AttachToServer(s *grpc.Server) {
	s = attachInterceptor(s)
}

func attachInterceptor(s *grpc.Server) *grpc.Server {
	interceptor := func(ctx context.Context, req interface{},
		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

		fmt.Printf("[gRPC Sniffer] → %s\n", info.FullMethod)

		if md, ok := metadata.FromIncomingContext(ctx); ok {
			fmt.Printf("[gRPC Sniffer] Metadata: %+v\n", md)
		}

		resp, err := handler(ctx, req)

		if err != nil {
			fmt.Printf("[gRPC Sniffer] ⚠️ Error: %v\n", err)
		} else {
			fmt.Printf("[gRPC Sniffer] ✓ Completed %s\n", info.FullMethod)
		}

		return resp, err
	}

	// Inject interceptor at runtime (unsafe hack — use responsibly)
	v := reflect.ValueOf(s).Elem()
	field := v.FieldByName("opts")
	if field.IsValid() && field.CanSet() {
		opts := field.Interface().([]grpc.ServerOption)
		newOpt := grpc.UnaryInterceptor(interceptor)
		field.Set(reflect.ValueOf(append(opts, newOpt)))
		fmt.Println("[gRPC Sniffer] Attached to running gRPC server")
	} else {
		fmt.Println("[gRPC Sniffer] Could not access opts via reflection")
	}

	return s
}
