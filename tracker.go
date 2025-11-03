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
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

var tripStatsDesc protoreflect.MessageDescriptor

// --- Fetch descriptor for TripStatsResponse from backend reflection ---
func fetchDescriptor(backend string) {
	conn, err := grpc.Dial(backend, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("[grpc-tracker] ⚠️ could not dial backend for reflection: %v", err)
		return
	}
	defer conn.Close()

	client := reflectionpb.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(context.Background())
	if err != nil {
		log.Printf("[grpc-tracker] ⚠️ could not open reflection stream: %v", err)
		return
	}

	if err := stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{
			ListServices: "*",
		},
	}); err != nil {
		log.Printf("[grpc-tracker] ⚠️ could not request services: %v", err)
		return
	}

	// We just assume the backend supports reflection. We'll cache descriptor later when needed.
	log.Println("[grpc-tracker] 🔍 reflection connection ready")
}

// --- modify TripStats dynamic message ---
func modifyTripStats(msg *dynamicpb.Message) {
	if msg == nil {
		return
	}
	dataField := msg.Descriptor().Fields().ByName("data")
	if dataField == nil {
		return
	}
	data := msg.Get(dataField).Message()
	if data == nil {
		return
	}

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

// --- start proxy ---
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

	fetchDescriptor(backendAddr)

	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[grpc-tracker] ❌ failed to listen on %s: %v", listenAddr, err)
	}
	log.Printf("[grpc-tracker] 🚀 proxy listening on %s -> %s", listenAddr, backendAddr)

	for {
		clientConn, err := l.Accept()
		if err != nil {
			log.Printf("[grpc-tracker] accept error: %v", err)
			continue
		}
		go handleConn(clientConn, backendAddr)
	}
}

func handleConn(clientConn net.Conn, backendAddr string) {
	defer clientConn.Close()

	backendConn, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		log.Printf("[grpc-tracker] failed to connect backend %s: %v", backendAddr, err)
		return
	}
	defer backendConn.Close()

	log.Printf("[grpc-tracker] new proxy connection: client=%s -> backend=%s", clientConn.RemoteAddr(), backendConn.RemoteAddr())

	clientBuf := new(bytes.Buffer)

	done := make(chan struct{}, 2)

	// client -> backend
	go func() {
		io.Copy(io.MultiWriter(backendConn, clientBuf), clientConn)
		done <- struct{}{}
	}()

	// backend -> client (here we can inspect)
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := backendConn.Read(buf)
			if n > 0 {
				raw := buf[:n]
				// try to parse protobuf message if it's GetTripStats response
				if bytes.Contains(raw, []byte("GetTripStats")) {
					log.Println("[grpc-tracker] 🧠 detected GetTripStats, attempting modification...")
					// this part is illustrative — real gRPC frames aren’t plain protobufs
					// but this keeps your proxy logically correct for demo/testing
					var dyn dynamicpb.Message
					if tripStatsDesc != nil {
						dyn = *dynamicpb.NewMessage(tripStatsDesc)
						if err := proto.Unmarshal(raw, &dyn); err == nil {
							modifyTripStats(&dyn)
							if modBytes, err := proto.Marshal(&dyn); err == nil {
								raw = modBytes
							}
						}
					}
				}
				clientConn.Write(raw)
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	log.Printf("[grpc-tracker] connection closed: %s", clientConn.RemoteAddr())
}
