package grpctracker

import (
	"context"
	"log"

	"github.com/likhithkp/grpc-tracker/modifiers"
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

	//modify response
	if info.FullMethod == "/tripProto.TripService/GetTripStats" {
		modifiers.ModifyTripStats(resp)
	}

	return resp, err
}
