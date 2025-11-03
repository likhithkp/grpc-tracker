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
	"bufio"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"time"
)

func init() {
	go startTracker()
}

func startTracker() {
	listen := os.Getenv("GRPC_TRACKER_ADDR")
	if listen == "" {
		listen = ":4440"
	}
	backend := os.Getenv("GRPC_BACKEND_ADDR")
	if backend == "" {
		backend = "127.0.0.1:4430"
	}

	l, err := net.Listen("tcp", listen)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ failed to listen on %s: %v", listen, err)
		return
	}
	log.Printf("[grpc-tracker] 🚀 Listening on %s (proxying to %s)", listen, backend)

	for {
		clientConn, err := l.Accept()
		if err != nil {
			continue
		}
		go handleConn(clientConn, backend)
	}
}

func handleConn(client net.Conn, backend string) {
	defer client.Close()

	serverConn, err := net.DialTimeout("tcp", backend, 3*time.Second)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ cannot connect to backend %s: %v", backend, err)
		return
	}
	defer serverConn.Close()

	log.Printf("[grpc-tracker] 🔗 new proxy connection: client=%s -> backend=%s", client.RemoteAddr(), backend)

	// Regex for detecting gRPC method paths
	methodRegex := regexp.MustCompile(`\/[a-zA-Z0-9_.]+\/[a-zA-Z0-9_]+`)

	// --- read client → backend (requests)
	go func() {
		reader := bufio.NewReader(client)
		for {
			data := make([]byte, 4096)
			n, err := reader.Read(data)
			if n > 0 {
				chunk := data[:n]
				if m := methodRegex.Find(chunk); m != nil {
					log.Printf("[grpc-tracker] 🚀 Client calling: %s", string(m))
				}
				_, _ = serverConn.Write(chunk)
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("[grpc-tracker] client->server closed: %v", err)
				}
				return
			}
		}
	}()

	// --- read backend → client (responses)
	go func() {
		reader := bufio.NewReader(serverConn)
		for {
			data := make([]byte, 4096)
			n, err := reader.Read(data)
			if n > 0 {
				chunk := data[:n]
				// You could inspect response data here too if you want
				_, _ = client.Write(chunk)
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("[grpc-tracker] server->client closed: %v", err)
				}
				return
			}
		}
	}()
}
