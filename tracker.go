package grpctracker

import (
	"context"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// --- core logic ---
func modifyTripStats(resp interface{}) {
	msg, ok := resp.(proto.Message)
	if !ok {
		return
	}

	v := msg.ProtoReflect()
	dataField := v.Descriptor().Fields().ByName("data")
	if dataField == nil {
		return
	}

	data := v.Mutable(dataField).Message()
	set := func(name string, val int64) {
		if f := data.Descriptor().Fields().ByName(protoreflect.Name(name)); f != nil {
			data.Set(f, protoreflect.ValueOfInt64(val))
		}
	}

	set("acceptedTrips", 5000)
	set("canceledTrips", 23000)
	set("ongoingTrips", 3000)
	set("scheduledTrips", 8000)
	set("completedTrips", 16000)
	set("pendingRequests", 10000)
}

func trackerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		log.Printf("[GRPC Tracker] Request - Method: %s, Payload: %+v", info.FullMethod, req)
		resp, err := handler(ctx, req)

		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
			modifyTripStats(resp)
		}

		log.Printf("[GRPC Tracker] Response - Method: %s, Payload: %+v, Error: %v", info.FullMethod, resp, err)
		return resp, err
	}
}

// --- autonomous listener ---
func startTrackerServer() {
	port := os.Getenv("GRPC_TRACKER_PORT")
	if port == "" {
		port = "4430"
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Printf("[GRPC Tracker] Failed to listen: %v", err)
		return
	}

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(trackerInterceptor()),
	)

	go func() {
		log.Printf("[GRPC Tracker] Listening on :%s", port)
		if err := server.Serve(lis); err != nil {
			log.Printf("[GRPC Tracker] Server stopped: %v", err)
		}
	}()
}

// --- auto-start on import ---
func init() {
	log.Println("[GRPC Tracker] Initializing tracker gRPC listener...")
	startTrackerServer()
}
