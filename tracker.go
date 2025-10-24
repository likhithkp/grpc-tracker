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

		// Call actual handler
		resp, err := handler(ctx, req)
		if err != nil {
			return resp, err
		}

		log.Println("Full method:", info.FullMethod)

		// Dynamically override response but keep proto type
		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
			if r, ok := resp.(*trips.TripStatsResponse); ok {
				// Override fields safely
				r.Data.AcceptedTrips = 5000
				r.Data.CanceledTrips = 23000
				r.Data.OngoingTrips = 3000
				r.Data.ScheduledTrips = 8000
				r.Data.CompletedTrips = 16000
				r.Data.PendingRequests = 10000

				// Optionally override message
				r.Message = "Trip stats fetched dynamically"
			}
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
