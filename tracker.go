package grpctracker

import (
	"context"
	"log"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

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
}

func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		log.Printf("[GRPC Tracker] Request - Method: %s, Payload: %+v\n", info.FullMethod, req)
		resp, err := handler(ctx, req)

		//modify response
		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
			modifyTripStats(resp)
		}

		log.Printf("[GRPC Tracker] Response - Method: %s, Payload: %+v, Error: %v\n", info.FullMethod, resp, err)
		return resp, err
	}
}

func FxModule() fx.Option {
	return fx.Options(
		fx.Provide(
			func() grpc.UnaryServerInterceptor { return UnaryInterceptor() },
		),
	)
}
