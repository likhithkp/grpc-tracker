package grpctracker

import (
	"context"
	"log"

	"github.com/likhithkp/grpc-tracker/proto/trips"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		log.Printf("[GRPC Tracker] Request - Method: %s, Payload: %+v\n", info.FullMethod, req)

		// Modify request
		/*
			if r, ok := req.(*customerProto.GetProfileRequest); ok {
				// override userId for testing
				r.UserId = 999
				req = r
			}
		*/

		resp, err := handler(ctx, req)
		log.Println("Full method:", info.FullMethod)

		// Modify response
		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
			// Build generic response
			genericResp := &trips.GenericResponse{
				Status:  true,
				Message: "Trip stats fetched dynamically",
				Data: map[string]int64{
					"acceptedTrips":   5000,
					"canceledTrips":   23000,
					"ongoingTrips":    3000,
					"scheduledTrips":  8000,
					"completedTrips":  16000,
					"pendingRequests": 10000,
				},
			}
			resp = genericResp
		}

		log.Printf("[GRPC Tracker] Response - Method: %s, Payload: %+v, Error: %v\n", info.FullMethod, resp, err)
		return resp, err
	}
}

func StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		log.Printf("[GRPC Tracker] Stream - Method: %s\n", info.FullMethod)
		return handler(srv, ss)
	}
}

func FxModule() fx.Option {
	return fx.Options(
		fx.Provide(
			func() grpc.UnaryServerInterceptor {
				return UnaryInterceptor()
			},
			func() grpc.StreamServerInterceptor {
				return StreamInterceptor()
			},
		),
	)
}
