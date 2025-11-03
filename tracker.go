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
)

// start a transparent gRPC proxy that forwards all traffic
func init() {
	go startProxy()
}

func startProxy() {
	listenAddr := os.Getenv("GRPC_TRACKER_ADDR")
	if listenAddr == "" {
		listenAddr = ":4440"
	}
	backendAddr := os.Getenv("GRPC_BACKEND_ADDR")
	if backendAddr == "" {
		backendAddr = "127.0.0.1:4430"
	}

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[grpc-tracker] ❌ failed to listen on %s: %v", listenAddr, err)
	}

	s := grpc.NewServer(grpc.UnknownServiceHandler(proxyHandler(backendAddr)))

	log.Printf("[grpc-tracker] 🚀 proxy listening on %s -> backend %s", listenAddr, backendAddr)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("[grpc-tracker] ❌ serve error: %v", err)
	}
}

func proxyHandler(backend string) grpc.StreamHandler {
	return func(srv interface{}, serverStream grpc.ServerStream) error {
		fullMethod, _ := grpc.MethodFromServerStream(serverStream)
		log.Printf("[grpc-tracker] ➡ %s", fullMethod)

		conn, err := grpc.Dial(backend, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("[grpc-tracker] dial backend error: %v", err)
			return err
		}
		defer conn.Close()

		ctx := serverStream.Context()
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			ctx = metadata.NewOutgoingContext(ctx, md)
		}

		desc := &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}
		clientStream, err := conn.NewStream(ctx, desc, fullMethod)
		if err != nil {
			log.Printf("[grpc-tracker] NewStream error: %v", err)
			return err
		}

		errc := make(chan error, 2)

		// Client -> Backend
		go func() {
			for {
				msg := new([]byte)
				if err := serverStream.RecvMsg(msg); err != nil {
					errc <- err
					return
				}
				if err := clientStream.SendMsg(msg); err != nil {
					errc <- err
					return
				}
			}
		}()

		// Backend -> Client
		go func() {
			for {
				msg := new([]byte)
				if err := clientStream.RecvMsg(msg); err != nil {
					errc <- err
					return
				}

				// Here you could modify raw bytes for specific methods
				// e.g. if fullMethod == "/tripProto.TripService/GetTripStats" { ... }

				if err := serverStream.SendMsg(msg); err != nil {
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
	}
}
