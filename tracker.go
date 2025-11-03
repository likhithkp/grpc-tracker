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
	"encoding/binary"
	"io"
	"log"
	"net"
)

func init() {
	go startSniffer(":4440", "127.0.0.1:4430")
}

func startSniffer(listenAddr, backendAddr string) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ listen %v", err)
		return
	}
	log.Printf("[grpc-tracker] 👂 proxy listening on %s → backend %s", listenAddr, backendAddr)

	for {
		clientConn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConn(clientConn, backendAddr)
	}
}

func handleConn(clientConn net.Conn, backendAddr string) {
	serverConn, err := net.Dial("tcp", backendAddr)
	if err != nil {
		clientConn.Close()
		return
	}

	go pipeWithSniff(clientConn, serverConn)
	go io.Copy(clientConn, serverConn)
}

func pipeWithSniff(src, dst net.Conn) {
	reader := bufio.NewReader(src)
	writer := bufio.NewWriter(dst)

	defer src.Close()
	defer dst.Close()

	var buffer []byte
	for {
		header := make([]byte, 9)
		if _, err := io.ReadFull(reader, header); err != nil {
			return
		}

		length := binary.BigEndian.Uint32(append([]byte{0}, header[0:3]...))
		frameType := header[3]
		flags := header[4]
		streamID := binary.BigEndian.Uint32(header[5:9]) & 0x7FFFFFFF

		payload := make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return
		}

		// Write everything through
		writer.Write(header)
		writer.Write(payload)
		writer.Flush()

		// Detect gRPC request headers
		if frameType == 1 { // HEADERS frame
			method := sniffGrpcMethod(payload)
			if method != "" {
				log.Printf("[grpc-tracker] 🚀 RPC call detected on stream %d: %s", streamID, method)
			}
		}

		// Optional: buffer initial frames for other analyzers
		buffer = append(buffer, payload...)
		_ = flags
	}
}

func sniffGrpcMethod(payload []byte) string {
	// Basic HPACK parsing to find ":path" (this works for non-compressed headers)
	for i := 0; i < len(payload)-5; i++ {
		if payload[i] == 0x40 && i+5 < len(payload) { // Literal Header Field without Indexing
			n := int(payload[i+1])
			if i+2+n < len(payload) && string(payload[i+2:i+2+n]) == ":path" {
				l := int(payload[i+2+n+1])
				if i+2+n+2+l <= len(payload) {
					return string(payload[i+2+n+2 : i+2+n+2+l])
				}
			}
		}
	}
	return ""
}
