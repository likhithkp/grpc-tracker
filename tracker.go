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

// Automatically starts proxy when imported.
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

	modifyMethod := os.Getenv("GRPC_MODIFY_METHOD")
	if modifyMethod == "" {
		modifyMethod = "/tripProto.TripService/GetTripStats"
	}

	responseTypeName := os.Getenv("GRPC_RESPONSE_TYPE")
	if responseTypeName == "" {
		responseTypeName = "tripProto.GetTripStatsResponse"
	}

	log.Printf("[grpc-tracker] 🚀 proxy listening on %s -> %s (method=%s type=%s)",
		listenAddr, backendAddr, modifyMethod, responseTypeName)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[grpc-tracker] listen error: %v", err)
	}

	server := grpc.NewServer(
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			fullMethod, _ := grpc.MethodFromServerStream(stream)

			conn, err := grpc.Dial(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Printf("[grpc-tracker] dial backend error: %v", err)
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
				log.Printf("[grpc-tracker] new stream error: %v", err)
				return err
			}

			errc := make(chan error, 2)

			// Client -> Backend
			go func() {
				for {
					var inBytes []byte
					if err := stream.RecvMsg(&inBytes); err != nil {
						errc <- err
						return
					}
					if err := clientStream.SendMsg(inBytes); err != nil {
						errc <- err
						return
					}
				}
			}()

			// Backend -> Client
			go func() {
				for {
					var outBytes []byte
					if err := clientStream.RecvMsg(&outBytes); err != nil {
						errc <- err
						return
					}

					if fullMethod == modifyMethod && len(outBytes) > 0 {
						modified, ok := tryModifyResponse(outBytes, responseTypeName)
						if ok {
							outBytes = modified
							log.Printf("[grpc-tracker] ✏️ modified response for %s", fullMethod)
						}
					}

					if err := stream.SendMsg(outBytes); err != nil {
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

// --- Try to decode and modify response dynamically ---
func tryModifyResponse(data []byte, typeName string) ([]byte, bool) {
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(typeName))
	if err != nil {
		log.Printf("[grpc-tracker] ❌ type lookup failed: %v", err)
		return nil, false
	}

	msgDesc := msgType.Descriptor()
	dm := dynamicpb.NewMessage(msgDesc)

	if err := proto.Unmarshal(data, dm); err != nil {
		log.Printf("[grpc-tracker] ❌ failed to unmarshal: %v", err)
		return nil, false
	}

	modifyTripStatsDynamic(dm)

	newBytes, err := proto.Marshal(dm)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ failed to marshal after modification: %v", err)
		return nil, false
	}

	return newBytes, true
}

// --- Modify data fields dynamically ---
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
