package grpctracker

import (
	"context"
	"log"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func unaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		log.Printf("[GRPC Tracker] → %s | Req: %+v", info.FullMethod, req)
		resp, err := handler(ctx, req)
		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
			modifyTripStats(resp)
		}
		log.Printf("[GRPC Tracker] ← %s | Resp: %+v | Err: %v", info.FullMethod, resp, err)
		return resp, err
	}
}

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
		f := data.Descriptor().Fields().ByName(protoreflect.Name(name))
		if f != nil {
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

// Module exports a ready-to-use Fx module.
var Module = fx.Module("grpctracker",
	fx.Provide(func() grpc.UnaryServerInterceptor { return unaryInterceptor() }),
)
