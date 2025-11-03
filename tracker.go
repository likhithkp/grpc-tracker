// package grpctracker

// import (
// 	"io"
// 	"log"
// 	"net"
// 	"os"

// 	"google.golang.org/grpc"
// 	"google.golang.org/grpc/credentials/insecure"
// 	"google.golang.org/protobuf/proto"
// 	"google.golang.org/protobuf/reflect/protoreflect"
// )

// // --- modify response fields dynamically ---
// func modifyTripStats(m proto.Message) {
// 	if m == nil {
// 		return
// 	}
// 	v := m.ProtoReflect()
// 	dataField := v.Descriptor().Fields().ByName("data")
// 	if dataField == nil {
// 		return
// 	}
// 	data := v.Mutable(dataField).Message()

// 	set := func(name string, val int64) {
// 		if f := data.Descriptor().Fields().ByName(protoreflect.Name(name)); f != nil {
// 			data.Set(f, protoreflect.ValueOfInt64(val))
// 		}
// 	}

// 	set("acceptedTrips", 5000)
// 	set("canceledTrips", 23000)
// 	set("ongoingTrips", 3000)
// 	set("scheduledTrips", 8000)
// 	set("completedTrips", 16000)
// 	set("pendingRequests", 10000)
// 	log.Println("[grpc-tracker] ✅ Modified GetTripStats response fields")
// }

// // --- proxy that forwards every request ---
// func startProxy() {
// 	listenAddr := os.Getenv("GRPC_LISTEN_ADDR")
// 	if listenAddr == "" {
// 		listenAddr = ":4440"
// 	}
// 	backendAddr := os.Getenv("GRPC_BACKEND_ADDR")
// 	if backendAddr == "" {
// 		backendAddr = "127.0.0.1:4430"
// 	}

// 	log.Printf("[grpc-tracker] 🚀 Proxy listening on %s → backend %s", listenAddr, backendAddr)

// 	lis, err := net.Listen("tcp", listenAddr)
// 	if err != nil {
// 		log.Fatalf("[grpc-tracker] ❌ failed to listen: %v", err)
// 	}

// 	server := grpc.NewServer(
// 		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
// 			fullMethod, _ := grpc.MethodFromServerStream(stream)
// 			log.Printf("[grpc-tracker] 🔁 proxying %s", fullMethod)

// 			// connect to backend
// 			conn, err := grpc.Dial(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
// 			if err != nil {
// 				log.Printf("[grpc-tracker] ❌ backend dial failed: %v", err)
// 				return err
// 			}
// 			defer conn.Close()

// 			clientDesc := &grpc.StreamDesc{
// 				ServerStreams: true,
// 				ClientStreams: true,
// 			}

// 			clientCtx := stream.Context()
// 			clientStream, err := conn.NewStream(clientCtx, clientDesc, fullMethod)
// 			if err != nil {
// 				return err
// 			}

// 			errc := make(chan error, 2)

// 			// client → backend
// 			go func() {
// 				for {
// 					req := new([]byte)
// 					if err := stream.RecvMsg(req); err != nil {
// 						errc <- err
// 						return
// 					}
// 					if err := clientStream.SendMsg(req); err != nil {
// 						errc <- err
// 						return
// 					}
// 				}
// 			}()

// 			// backend → client
// 			go func() {
// 				for {
// 					resp := new([]byte)
// 					if err := clientStream.RecvMsg(resp); err != nil {
// 						errc <- err
// 						return
// 					}

// 					// decode & modify (only for GetTripStats)
// 					if fullMethod == "/tripProto.TripService/GetTripStats" {
// 						// try to unmarshal dynamically into proto.Message
// 						var msg proto.Message
// 						if err := proto.Unmarshal(*resp, msg); err == nil && msg != nil {
// 							modifyTripStats(msg)
// 							newBytes, _ := proto.Marshal(msg)
// 							resp = &newBytes
// 						}
// 					}

// 					if err := stream.SendMsg(resp); err != nil {
// 						errc <- err
// 						return
// 					}
// 				}
// 			}()

// 			err = <-errc
// 			if err == io.EOF {
// 				return nil
// 			}
// 			return err
// 		}),
// 	)

// 	if err := server.Serve(lis); err != nil {
// 		log.Fatalf("[grpc-tracker] ❌ serve error: %v", err)
// 	}
// }

// func init() {
// 	go startProxy()
// }

package grpctracker

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// --- modify TripStats response dynamically ---
func modifyTripStats(resp proto.Message) {
	if resp == nil {
		return
	}

	v := resp.ProtoReflect()
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

	log.Println("[grpc-tracker] ✅ Modified GetTripStats response")
}

// --- unary interceptor ---
func trackerUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	if err != nil {
		return resp, err
	}

	if info.FullMethod == "/tripProto.TripService/GetTripStats" {
		if msg, ok := resp.(proto.Message); ok {
			modifyTripStats(msg)
		}
	}

	log.Printf("[grpc-tracker] %s took %s", info.FullMethod, time.Since(start))
	return resp, nil
}

// --- proxy handler ---
func makeProxyHandler(target string) grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		ctx := stream.Context()
		md, _ := metadata.FromIncomingContext(ctx)
		outCtx := metadata.NewOutgoingContext(ctx, md)

		conn, err := grpc.DialContext(outCtx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("[grpc-tracker] ❌ connect to backend failed: %v", err)
			return err
		}
		defer conn.Close()

		fullMethod, ok := grpc.MethodFromServerStream(stream)
		if !ok {
			log.Println("[grpc-tracker] ❌ cannot get method name")
			return err
		}

		client, err := grpc.NewClientStream(outCtx, &_StreamDesc, conn, fullMethod)
		if err != nil {
			return err
		}

		errCh := make(chan error, 2)
		go func() {
			for {
				m := new([]byte)
				if err := stream.RecvMsg(m); err != nil {
					errCh <- err
					return
				}
				if err := client.SendMsg(m); err != nil {
					errCh <- err
					return
				}
			}
		}()
		go func() {
			for {
				m := new([]byte)
				if err := client.RecvMsg(m); err != nil {
					errCh <- err
					return
				}
				if err := stream.SendMsg(m); err != nil {
					errCh <- err
					return
				}
			}
		}()
		for i := 0; i < 2; i++ {
			if err := <-errCh; err != io.EOF {
				return err
			}
		}
		return nil
	}
}

var _StreamDesc = grpc.StreamDesc{
	StreamName:    "proxy",
	ServerStreams: true,
	ClientStreams: true,
}

// --- auto start on import ---
func init() {
	go func() {
		backendAddr := os.Getenv("GRPC_BACKEND_ADDR")
		if backendAddr == "" {
			backendAddr = "localhost:4430"
		}
		listenAddr := os.Getenv("GRPC_TRACKER_ADDR")
		if listenAddr == "" {
			listenAddr = ":4440"
		}

		lis, err := net.Listen("tcp", listenAddr)
		if err != nil {
			log.Printf("[grpc-tracker] ❌ listen failed on %s: %v", listenAddr, err)
			return
		}

		srv := grpc.NewServer(
			grpc.UnknownServiceHandler(makeProxyHandler(backendAddr)),
			grpc.UnaryInterceptor(trackerUnaryInterceptor),
		)
		reflection.Register(srv)

		log.Printf("[grpc-tracker] 🚀 listening on %s -> %s", listenAddr, backendAddr)
		if err := srv.Serve(lis); err != nil {
			log.Printf("[grpc-tracker] ❌ Serve failed: %v", err)
		}
	}()
}
