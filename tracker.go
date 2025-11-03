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
	"bytes"
	"log"
	"net"
	"regexp"
	"strings"
	"time"
)

func init() {
	go startSniffer(":4440")
}

func startSniffer(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ failed to bind %s: %v", addr, err)
		return
	}
	log.Printf("[grpc-tracker] 👂 sniffer listening on %s (same as backend)", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[grpc-tracker] accept error: %v", err)
			continue
		}
		go sniffConn(conn)
	}
}

func sniffConn(c net.Conn) {
	defer c.Close()

	clientAddr := c.RemoteAddr().String()
	log.Printf("[grpc-tracker] 🔗 new client %s", clientAddr)

	reader := bufio.NewReader(c)
	methodRegex := regexp.MustCompile(`\/[a-zA-Z0-9_.]+\/[a-zA-Z0-9_]+`)

	for {
		buf := make([]byte, 4096)
		c.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if m := methodRegex.Find(chunk); m != nil {
				method := string(bytes.TrimSpace(m))
				if strings.Contains(method, "tripProto") {
					log.Printf("[grpc-tracker] 🚀 RPC called: %s", method)
				}
			}
		}
		if err != nil {
			// stop quietly when done
			return
		}
	}
}
