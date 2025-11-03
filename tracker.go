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
	"google.golang.org/grpc/reflection"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// --- modify response (proto reflection) ---
func modifyTripStats(resp interface{}) {
	msg, ok := resp.(proto.Message)
	if !ok || msg == nil {
		return
	}

	v := msg.ProtoReflect()
	dataField := v.Descriptor().Fields().ByName("data")
	if dataField == nil {
		return
	}

	data := v.Mutable(dataField).Message()

	setField := func(name string, val int64) {
		f := data.Descriptor().Fields().ByName(protoreflect.Name(name))
		if f != nil {
			data.Set(f, protoreflect.ValueOfInt64(val))
		}
	}

	setField("acceptedTrips", 5000)
	setField("canceledTrips", 23000)
	setField("ongoingTrips", 3000)
	setField("scheduledTrips", 8000)
	setField("completedTrips", 16000)
	setField("pendingRequests", 10000)
	log.Println("[grpc-tracker] ✅ Modified GetTripStats response fields")
}

// --- unary interceptor ---
func trackerUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()
	log.Printf("[grpc-tracker] ➡ %s | req: %+v", info.FullMethod, req)

	resp, err := handler(ctx, req)

	if err != nil {
		log.Printf("[grpc-tracker] ❌ %s error: %v (took %s)", info.FullMethod, err, time.Since(start))
	} else {
		// if the RPC is GetTripStats, modify the response
		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
			modifyTripStats(resp)
		}
		log.Printf("[grpc-tracker] ⬅ %s | resp: %+v (took %s)", info.FullMethod, resp, time.Since(start))
	}

	return resp, err
}

// --- init starts tracker server automatically on import ---
func init() {
	go func() {
		addr := os.Getenv("GRPC_TRACKER_ADDR")
		if addr == "" {
			addr = ":4440" // default, change with env if needed
		}

		lis, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("[grpc-tracker] ❌ failed to listen on %s: %v", addr, err)
			return
		}

		server := grpc.NewServer(
			grpc.UnaryInterceptor(trackerUnaryInterceptor),
		)

		// Register reflection so tools like grpcurl can introspect
		reflection.Register(server)

		log.Printf("[grpc-tracker] 🚀 listening on %s", addr)
		if err := server.Serve(lis); err != nil {
			log.Printf("[grpc-tracker] ❌ server.Serve error: %v", err)
		}
	}()
}
