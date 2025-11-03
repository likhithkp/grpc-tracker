package grpctracker

import (
	"context"
	"log"

	"github.com/likhithkp/grpc-tracker/modifiers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

//NOTE: Add below in server.go inside NewGRPCServer()
// grpcServer := grpc.NewServer(
// 	grpc.ChainUnaryInterceptor(
// 		interceptor.Unary(logger),
// 		tracker.UnaryInterceptor,
// 	),
// 	grpc.MaxRecvMsgSize(1024*1024*100),
// 	grpc.MaxSendMsgSize(1024*1024*100),
// )

//NOTE:Import
//  go get github.com/likhithkp/grpc-tracker@v0.0.54
// version can change check github for latest release

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
