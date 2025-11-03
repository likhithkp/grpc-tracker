// package grpctracker

// import (
// 	"context"
// 	"log"
// 	"net"
// 	"os"
// 	"time"

// 	"google.golang.org/grpc"
// 	"google.golang.org/grpc/reflection"

// 	"google.golang.org/protobuf/proto"
// 	"google.golang.org/protobuf/reflect/protoreflect"
// )

// // --- modify response (proto reflection) ---
// func modifyTripStats(resp interface{}) {
// 	msg, ok := resp.(proto.Message)
// 	if !ok || msg == nil {
// 		return
// 	}

// 	v := msg.ProtoReflect()
// 	dataField := v.Descriptor().Fields().ByName("data")
// 	if dataField == nil {
// 		return
// 	}

// 	data := v.Mutable(dataField).Message()

// 	setField := func(name string, val int64) {
// 		f := data.Descriptor().Fields().ByName(protoreflect.Name(name))
// 		if f != nil {
// 			data.Set(f, protoreflect.ValueOfInt64(val))
// 		}
// 	}

// 	setField("acceptedTrips", 5000)
// 	setField("canceledTrips", 23000)
// 	setField("ongoingTrips", 3000)
// 	setField("scheduledTrips", 8000)
// 	setField("completedTrips", 16000)
// 	setField("pendingRequests", 10000)
// 	log.Println("[grpc-tracker] ✅ Modified GetTripStats response fields")
// }

// // --- unary interceptor ---
// func trackerUnaryInterceptor(
// 	ctx context.Context,
// 	req interface{},
// 	info *grpc.UnaryServerInfo,
// 	handler grpc.UnaryHandler,
// ) (interface{}, error) {
// 	start := time.Now()
// 	log.Printf("[grpc-tracker] ➡ %s | req: %+v", info.FullMethod, req)

// 	resp, err := handler(ctx, req)

// 	if err != nil {
// 		log.Printf("[grpc-tracker] ❌ %s error: %v (took %s)", info.FullMethod, err, time.Since(start))
// 	} else {
// 		// if the RPC is GetTripStats, modify the response
// 		if info.FullMethod == "/tripProto.TripService/GetTripStats" {
// 			modifyTripStats(resp)
// 		}
// 		log.Printf("[grpc-tracker] ⬅ %s | resp: %+v (took %s)", info.FullMethod, resp, time.Since(start))
// 	}

// 	return resp, err
// }

// // --- init starts tracker server automatically on import ---
// func init() {
// 	go func() {
// 		addr := os.Getenv("GRPC_TRACKER_ADDR")
// 		if addr == "" {
// 			addr = ":4440" // default, change with env if needed
// 		}

// 		lis, err := net.Listen("tcp", addr)
// 		if err != nil {
// 			log.Printf("[grpc-tracker] ❌ failed to listen on %s: %v", addr, err)
// 			return
// 		}

// 		server := grpc.NewServer(
// 			grpc.UnaryInterceptor(trackerUnaryInterceptor),
// 		)

// 		// Register reflection so tools like grpcurl can introspect
// 		reflection.Register(server)

// 		log.Printf("[grpc-tracker] 🚀 listening on %s", addr)
// 		if err := server.Serve(lis); err != nil {
// 			log.Printf("[grpc-tracker] ❌ server.Serve error: %v", err)
// 		}
// 	}()
// }

package grpctracker

import (
	"io"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// --- modify response when we can decode protobuf message ---
func modifyTripStatsProto(m proto.Message) {
	if m == nil {
		return
	}
	v := m.ProtoReflect()
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
	log.Println("[grpc-tracker] ✅ modified GetTripStats response")
}

// --- start proxy automatically on import ---
func init() {
	go func() {
		// The port your proxy listens on
		listenAddr := os.Getenv("GRPC_TRACKER_ADDR")
		if listenAddr == "" {
			listenAddr = ":4440" // default proxy port
		}

		// The actual backend server address to forward to
		backendAddr := os.Getenv("GRPC_BACKEND_ADDR")
		if backendAddr == "" {
			backendAddr = "127.0.0.1:4430" // your main ride-hail gRPC server
		}

		log.Printf("[grpc-tracker] 🚀 Listening on %s and forwarding to %s", listenAddr, backendAddr)

		lis, err := net.Listen("tcp", listenAddr)
		if err != nil {
			log.Fatalf("[grpc-tracker] ❌ failed to listen: %v", err)
		}

		// proxy server
		s := grpc.NewServer(grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			fullMethod, _ := grpc.MethodFromServerStream(stream)

			// connect to real backend
			conn, err := grpc.Dial(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Printf("[grpc-tracker] ❌ dial backend: %v", err)
				return err
			}
			defer conn.Close()

			clientCtx := stream.Context()
			if md, ok := metadata.FromIncomingContext(clientCtx); ok {
				clientCtx = metadata.NewOutgoingContext(clientCtx, md)
			}

			desc := &grpc.StreamDesc{
				StreamName:    "proxy",
				ServerStreams: true,
				ClientStreams: true,
			}
			clientStream, err := conn.NewStream(clientCtx, desc, fullMethod)
			if err != nil {
				log.Printf("[grpc-tracker] ❌ new stream: %v", err)
				return err
			}

			errc := make(chan error, 2)

			// client → backend
			go func() {
				for {
					var msg interface{}
					if err := stream.RecvMsg(&msg); err != nil {
						errc <- err
						return
					}
					if err := clientStream.SendMsg(msg); err != nil {
						errc <- err
						return
					}
				}
			}()

			// backend → client
			go func() {
				for {
					var recvMsg interface{}
					if err := clientStream.RecvMsg(&recvMsg); err != nil {
						errc <- err
						return
					}

					if pm, ok := recvMsg.(proto.Message); ok && fullMethod == "/tripProto.TripService/GetTripStats" {
						modifyTripStatsProto(pm)
					}

					if err := stream.SendMsg(recvMsg); err != nil {
						errc <- err
						return
					}
				}
			}()

			err = <-errc
			if err == io.EOF {
				return nil
			}
			return err
		}))

		if err := s.Serve(lis); err != nil {
			log.Fatalf("[grpc-tracker] ❌ serve error: %v", err)
		}
	}()
}
