package grpctracker

import (
	"context"
	"log"

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

		// Modify response
		/*
			if info.FullMethod == "/tripProto.TripService/GetTripStats" {
				if r, ok := resp.(*tripProto.GetTripStatsResponse); ok {
					// override fields
					r.Data.PendingRequests = "10"
					r.Data.AcceptedTrips = "5"
					r.Data.OngoingTrips = "3"
					r.Data.ScheduledTrips = "8"
					r.Data.CompletedTrips = "160"
					r.Data.CanceledTrips = "230"
					resp = r
				}
			}
		*/

		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
			log.Println("[GRPC Tracker] Overriding GetTripStats response dynamically")
			resp = map[string]interface{}{
				"status":  true,
				"message": "Trip stats fetched successfully",
				"data": map[string]string{
					"pendingRequests": "10000",
					"acceptedTrips":   "5000",
					"ongoingTrips":    "3000",
					"scheduledTrips":  "8000",
					"completedTrips":  "16000",
					"canceledTrips":   "23000",
				},
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
