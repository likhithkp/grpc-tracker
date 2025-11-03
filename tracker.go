package grpctracker

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	log.Printf("Incoming gRPC call: %s", info.FullMethod)

	resp, err := handler(ctx, req)

	st, _ := status.FromError(err)
	log.Printf("Completed %s → status=%s", info.FullMethod, st.Code())

	// Example of response manipulation:
	// if strings.Contains(info.FullMethod, "TripService/CancelTrip") {
	//     // mutate resp here (requires type assertion)
	// }

	return resp, err
}
