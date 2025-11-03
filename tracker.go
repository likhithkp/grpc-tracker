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
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// --- entrypoint ---
func init() {
	go startProxy()
}

func startProxy() {
	listenAddr := getEnv("GRPC_TRACKER_ADDR", ":4440")
	backendAddr := getEnv("GRPC_BACKEND_ADDR", "127.0.0.1:4430")
	modifyMethod := getEnv("GRPC_MODIFY_METHOD", "/tripProto.TripService/GetTripStats")
	responseTypeName := getEnv("GRPC_RESPONSE_TYPE", "tripProto.GetTripStatsResponse")

	log.Printf("[grpc-tracker] 🚀 Proxy listening on %s -> %s", listenAddr, backendAddr)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[grpc-tracker] listen error: %v", err)
	}

	server := grpc.NewServer(
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			fullMethod, _ := grpc.MethodFromServerStream(stream)

			conn, err := grpc.Dial(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Printf("[grpc-tracker] dial error: %v", err)
				return err
			}
			defer conn.Close()

			clientCtx := stream.Context()
			if md, ok := metadata.FromIncomingContext(clientCtx); ok {
				clientCtx = metadata.NewOutgoingContext(clientCtx, md)
			}

			desc := &grpc.StreamDesc{StreamName: "proxy", ServerStreams: true, ClientStreams: true}
			clientStream, err := conn.NewStream(clientCtx, desc, fullMethod)
			if err != nil {
				log.Printf("[grpc-tracker] stream create error: %v", err)
				return err
			}

			errc := make(chan error, 2)

			// Client -> Backend
			go func() {
				for {
					var in dynamicpb.Message
					if err := stream.RecvMsg(&in); err != nil {
						errc <- err
						return
					}
					if err := clientStream.SendMsg(&in); err != nil {
						errc <- err
						return
					}
				}
			}()

			// Backend -> Client
			go func() {
				for {
					msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(responseTypeName))
					if err != nil {
						log.Printf("[grpc-tracker] type lookup failed: %v", err)
						errc <- err
						return
					}

					resp := dynamicpb.NewMessage(msgType.Descriptor())
					if err := clientStream.RecvMsg(resp); err != nil {
						errc <- err
						return
					}

					// Modify only our target method
					if fullMethod == modifyMethod {
						modifyTripStatsDynamic(resp)
						log.Printf("[grpc-tracker] ✏️ modified response for %s", fullMethod)
					}

					if err := stream.SendMsg(resp); err != nil {
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
		}),
	)

	if err := server.Serve(lis); err != nil {
		log.Fatalf("[grpc-tracker] serve error: %v", err)
	}
}

func modifyTripStatsDynamic(m proto.Message) {
	if m == nil {
		return
	}
	v := m.ProtoReflect()
	dataField := v.Descriptor().Fields().ByName("data")
	if dataField == nil {
		log.Println("[grpc-tracker] ⚠️ no 'data' field found in message")
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
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
