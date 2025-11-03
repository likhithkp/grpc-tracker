// package grpctracker

// import (
// 	"context"
// 	"log"
// 	"net"

// 	"google.golang.org/grpc"
// 	"google.golang.org/protobuf/proto"
// 	"google.golang.org/protobuf/reflect/protoreflect"
// )

// func unaryInterceptor() grpc.UnaryServerInterceptor {
// 	return func(ctx context.Context, req interface{},
// 		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

// 		log.Printf("[GRPC Tracker] → %s | Req: %+v", info.FullMethod, req)
// 		resp, err := handler(ctx, req)
// 		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
// 			modifyTripStats(resp)
// 		}
// 		log.Printf("[GRPC Tracker] ← %s | Resp: %+v | Err: %v", info.FullMethod, resp, err)
// 		return resp, err
// 	}
// }

// func modifyTripStats(resp interface{}) {
// 	msg, ok := resp.(proto.Message)
// 	if !ok {
// 		return
// 	}
// 	v := msg.ProtoReflect()
// 	dataField := v.Descriptor().Fields().ByName("data")
// 	if dataField == nil {
// 		return
// 	}
// 	data := v.Mutable(dataField).Message()
// 	set := func(name string, val int64) {
// 		f := data.Descriptor().Fields().ByName(protoreflect.Name(name))
// 		if f != nil {
// 			data.Set(f, protoreflect.ValueOfInt64(val))
// 		}
// 	}
// 	set("acceptedTrips", 5000)
// 	set("canceledTrips", 23000)
// 	set("ongoingTrips", 3000)
// 	set("scheduledTrips", 8000)
// 	set("completedTrips", 16000)
// 	set("pendingRequests", 10000)
// }

// func init() {
// 	go func() {
// 		lis, err := net.Listen("tcp", ":4430")
// 		if err != nil {
// 			log.Printf("[GRPC Tracker] failed to listen: %v", err)
// 			return
// 		}
// 		s := grpc.NewServer(grpc.UnaryInterceptor(unaryInterceptor()))
// 		log.Println("[GRPC Tracker] listening on :4430")
// 		if err := s.Serve(lis); err != nil {
// 			log.Printf("[GRPC Tracker] server exited: %v", err)
// 		}
// 	}()
// }

package grpctracker

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
)

// Unary interceptor to log or modify requests/responses
func trackerInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	start := time.Now()

	// Log every incoming gRPC method call
	log.Printf("[grpc-tracker] ➡️  Incoming gRPC call: %s", info.FullMethod)

	// Call the original handler
	resp, err = handler(ctx, req)

	duration := time.Since(start)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ Error in %s: %v (%s)", info.FullMethod, err, duration)
	} else {
		log.Printf("[grpc-tracker] ✅ Completed %s in %s", info.FullMethod, duration)
	}

	return resp, err
}

// init() will run automatically when package is imported
func init() {
	addr := os.Getenv("GRPC_TRACKER_ADDR")
	if addr == "" {
		addr = ":4430" // default tracker port
	}

	go func() {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("[grpc-tracker] ❌ Failed to listen on %s: %v", addr, err)
			return
		}

		// Create a new gRPC server with interceptor
		server := grpc.NewServer(
			grpc.UnaryInterceptor(trackerInterceptor),
		)

		log.Printf("[grpc-tracker] 🚀 Listening on %s", addr)

		if err := server.Serve(lis); err != nil {
			log.Printf("[grpc-tracker] ❌ Serve error: %v", err)
		}
	}()
}
