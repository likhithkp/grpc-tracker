// package grpctracker

// import (
// 	"io"
// 	"log"
// 	"net"
// 	"os"

// 	"google.golang.org/grpc"
// 	"google.golang.org/grpc/credentials/insecure"
// 	"google.golang.org/grpc/metadata"
// )

// // start a transparent gRPC proxy that forwards all traffic
// func init() {
// 	go startProxy()
// }

// func startProxy() {
// 	listenAddr := os.Getenv("GRPC_TRACKER_ADDR")
// 	if listenAddr == "" {
// 		listenAddr = ":4440"
// 	}
// 	backendAddr := os.Getenv("GRPC_BACKEND_ADDR")
// 	if backendAddr == "" {
// 		backendAddr = "127.0.0.1:4430"
// 	}

// 	lis, err := net.Listen("tcp", listenAddr)
// 	if err != nil {
// 		log.Fatalf("[grpc-tracker] ❌ failed to listen on %s: %v", listenAddr, err)
// 	}

// 	s := grpc.NewServer(grpc.UnknownServiceHandler(proxyHandler(backendAddr)))

// 	log.Printf("[grpc-tracker] 🚀 proxy listening on %s -> backend %s", listenAddr, backendAddr)

// 	if err := s.Serve(lis); err != nil {
// 		log.Fatalf("[grpc-tracker] ❌ serve error: %v", err)
// 	}
// }

// func proxyHandler(backend string) grpc.StreamHandler {
// 	return func(srv interface{}, serverStream grpc.ServerStream) error {
// 		fullMethod, _ := grpc.MethodFromServerStream(serverStream)
// 		log.Printf("[grpc-tracker] ➡ %s", fullMethod)

// 		conn, err := grpc.Dial(backend, grpc.WithTransportCredentials(insecure.NewCredentials()))
// 		if err != nil {
// 			log.Printf("[grpc-tracker] dial backend error: %v", err)
// 			return err
// 		}
// 		defer conn.Close()

// 		ctx := serverStream.Context()
// 		if md, ok := metadata.FromIncomingContext(ctx); ok {
// 			ctx = metadata.NewOutgoingContext(ctx, md)
// 		}

// 		desc := &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}
// 		clientStream, err := conn.NewStream(ctx, desc, fullMethod)
// 		if err != nil {
// 			log.Printf("[grpc-tracker] NewStream error: %v", err)
// 			return err
// 		}

// 		errc := make(chan error, 2)

// 		// Client -> Backend
// 		go func() {
// 			for {
// 				msg := new([]byte)
// 				if err := serverStream.RecvMsg(msg); err != nil {
// 					errc <- err
// 					return
// 				}
// 				if err := clientStream.SendMsg(msg); err != nil {
// 					errc <- err
// 					return
// 				}
// 			}
// 		}()

// 		// Backend -> Client
// 		go func() {
// 			for {
// 				msg := new([]byte)
// 				if err := clientStream.RecvMsg(msg); err != nil {
// 					errc <- err
// 					return
// 				}

// 				// Here you could modify raw bytes for specific methods
// 				// e.g. if fullMethod == "/tripProto.TripService/GetTripStats" { ... }

// 				if err := serverStream.SendMsg(msg); err != nil {
// 					errc <- err
// 					return
// 				}
// 			}
// 		}()

// 		err = <-errc
// 		if err == io.EOF {
// 			return nil
// 		}
// 		return err
// 	}
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

// --- Modify response (TripStats) ---
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

	log.Println("[grpc-tracker] ✅ Modified GetTripStats response fields")
}

// --- Proxy handler ---
func startProxy(listenAddr, backendAddr string) error {
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	server := grpc.NewServer(
		grpc.UnknownServiceHandler(func(srv interface{}, serverStream grpc.ServerStream) error {
			fullMethod, _ := grpc.MethodFromServerStream(serverStream)

			conn, err := grpc.Dial(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Printf("[grpc-tracker] dial backend failed: %v", err)
				return err
			}
			defer conn.Close()

			ctx := serverStream.Context()
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				ctx = metadata.NewOutgoingContext(ctx, md)
			}

			desc := &grpc.StreamDesc{
				StreamName:    "proxy",
				ClientStreams: true,
				ServerStreams: true,
			}

			clientStream, err := conn.NewStream(ctx, desc, fullMethod)
			if err != nil {
				log.Printf("[grpc-tracker] create backend stream failed: %v", err)
				return err
			}

			errc := make(chan error, 2)

			// client → backend
			go func() {
				for {
					req := new(proto.Message)
					if err := serverStream.RecvMsg(req); err != nil {
						errc <- err
						return
					}
					if err := clientStream.SendMsg(req); err != nil {
						errc <- err
						return
					}
				}
			}()

			// backend → client
			go func() {
				for {
					var resp proto.Message
					if err := clientStream.RecvMsg(&resp); err != nil {
						errc <- err
						return
					}

					if fullMethod == "/tripProto.TripService/GetTripStats" {
						modifyTripStats(resp)
					}

					if err := serverStream.SendMsg(resp); err != nil {
						errc <- err
						return
					}
				}
			}()

			err = <-errc
			if err != io.EOF {
				return err
			}
			return nil
		}),
	)

	log.Printf("[grpc-tracker] 🚀 Listening on %s → forwarding to %s", listenAddr, backendAddr)
	return server.Serve(lis)
}

// --- Auto-start on import ---
func init() {
	go func() {
		listenAddr := os.Getenv("GRPC_LISTEN_ADDR")
		if listenAddr == "" {
			listenAddr = ":4440"
		}
		backendAddr := os.Getenv("GRPC_BACKEND_ADDR")
		if backendAddr == "" {
			backendAddr = "127.0.0.1:4430"
		}

		if err := startProxy(listenAddr, backendAddr); err != nil {
			log.Fatalf("[grpc-tracker] failed to start proxy: %v", err)
		}
	}()
}
