package grpctracker

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryInterceptor logs and optionally modifies gRPC requests/responses.
func UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	log.Printf("📡 Incoming gRPC call: %s", info.FullMethod)

	// Pre-modify request if needed
	// e.g. inject trace ID, sanitize input, etc.
	// (Use type assertion to modify known messages)

	resp, err := handler(ctx, req) // call original handler

	// Post-modify response or log status
	st, _ := status.FromError(err)
	log.Printf("✅ Completed %s → status=%s", info.FullMethod, st.Code())

	// Example of response manipulation:
	// if strings.Contains(info.FullMethod, "TripService/CancelTrip") {
	//     // mutate resp here (requires type assertion)
	// }

	return resp, err
}

// StreamInterceptor handles streaming RPCs too
func StreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	log.Printf("📡 Streaming gRPC call: %s", info.FullMethod)
	err := handler(srv, ss)
	st, _ := status.FromError(err)
	log.Printf("✅ Stream %s closed → status=%s", info.FullMethod, st.Code())
	return err
}
