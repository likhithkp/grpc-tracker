package grpctracker

import (
	"context"
	"log"
	"reflect"

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
			v := reflect.ValueOf(resp)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
				dataField := v.FieldByName("Data")
				if dataField.IsValid() && dataField.CanSet() {
					data := dataField.Interface()
					dv := reflect.ValueOf(data)
					if dv.Kind() == reflect.Struct {
						setInt64 := func(name string, val int64) {
							f := dv.FieldByName(name)
							if f.IsValid() && f.CanSet() && f.Kind() == reflect.Int64 {
								f.SetInt(val)
							}
						}
						setInt64("AcceptedTrips", 5000)
						setInt64("CanceledTrips", 23000)
						setInt64("OngoingTrips", 3000)
						setInt64("ScheduledTrips", 8000)
						setInt64("CompletedTrips", 16000)
						setInt64("PendingRequests", 10000)
					}
				}
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
