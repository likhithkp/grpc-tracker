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
	"io"
	"log"
	"net"
	"os"
	"time"
)

// A simple transparent TCP proxy. It forwards every TCP byte between client and backend.
// This avoids any gRPC-level unmarshal/marshal errors because it does not interpret frames.
//
// Behavior:
// - Listen on GRPC_TRACKER_ADDR (default ":4440")
// - Dial GRPC_BACKEND_ADDR   (default "127.0.0.1:4430")
// - For each incoming TCP connection, open a connection to backend and copy both ways.

func init() {
	go startTCPProxy()
}

func startTCPProxy() {
	listenAddr := os.Getenv("GRPC_TRACKER_ADDR")
	if listenAddr == "" {
		listenAddr = ":4440"
	}
	backendAddr := os.Getenv("GRPC_BACKEND_ADDR")
	if backendAddr == "" {
		backendAddr = "127.0.0.1:4430"
	}

	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ failed to listen on %s: %v", listenAddr, err)
		return
	}
	log.Printf("[grpc-tracker] 🚀 TCP proxy listening on %s -> %s", listenAddr, backendAddr)

	for {
		clientConn, err := l.Accept()
		if err != nil {
			log.Printf("[grpc-tracker] accept error: %v", err)
			continue
		}

		// handle connection in goroutine
		go func(c net.Conn) {
			defer c.Close()

			backendConn, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
			if err != nil {
				log.Printf("[grpc-tracker] failed to connect backend %s: %v", backendAddr, err)
				return
			}
			defer backendConn.Close()

			// log remote/local addresses
			log.Printf("[grpc-tracker] new proxy connection: client=%s -> backend=%s", c.RemoteAddr(), backendConn.RemoteAddr())

			// copy both directions
			done := make(chan struct{}, 2)
			// client -> backend
			go func() {
				_, err := io.Copy(backendConn, c)
				if err != nil {
					// EOF or other error — log at debug level
					log.Printf("[grpc-tracker] copy client->backend closed: %v", err)
				}
				// close the write side to notify backend
				_ = backendConn.(*net.TCPConn).CloseWrite()
				done <- struct{}{}
			}()

			// backend -> client
			go func() {
				_, err := io.Copy(c, backendConn)
				if err != nil {
					log.Printf("[grpc-tracker] copy backend->client closed: %v", err)
				}
				// close the write side to notify client
				_ = c.(*net.TCPConn).CloseWrite()
				done <- struct{}{}
			}()

			// wait for one side to finish, then wait the other (so we cleanly close)
			<-done
			<-done

			log.Printf("[grpc-tracker] connection closed: client=%s", c.RemoteAddr())
		}(clientConn)
	}
}
