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
	"log"
	"net"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// CONFIG
const grpcPort = 4430

func init() {
	go func() {
		time.Sleep(2 * time.Second)
		startEBPFGrpcSniffer()
	}()
}

func startEBPFGrpcSniffer() {
	log.Printf("[grpc-tracker] 🧠 attaching eBPF sniffer to :%d ...", grpcPort)

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_TCP)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ raw socket error: %v", err)
		return
	}
	defer unix.Close(fd)

	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		log.Printf("[grpc-tracker] setsockopt failed: %v", err)
	}

	localAddr := &unix.SockaddrInet4{Port: grpcPort}
	copy(localAddr.Addr[:], net.ParseIP("127.0.0.1").To4())

	if err := unix.Bind(fd, localAddr); err != nil {
		log.Printf("[grpc-tracker] ⚠️ cannot bind (expected if port already in use): %v", err)
	}

	buf := make([]byte, 65535)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			if errorsIsTimeout(err) {
				continue
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		parseGrpcFrame(buf[:n])
	}
}

func errorsIsTimeout(err error) bool {
	return strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "resource temporarily unavailable")
}

func parseGrpcFrame(packet []byte) {
	// Skip IP + TCP headers roughly (~40 bytes)
	if len(packet) < 40 {
		return
	}
	payload := packet[40:]

	if bytes.Contains(payload, []byte(":path")) {
		idx := bytes.Index(payload, []byte(":path"))
		if idx >= 0 {
			pathStart := idx + len(":path")
			snip := payload[pathStart:]
			// extract /service/method style
			if p := extractGrpcPath(snip); p != "" {
				log.Printf("[grpc-tracker] 🚀 %s", p)
			}
		}
	}
}

func extractGrpcPath(data []byte) string {
	s := string(data)
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[i:]
		if j := strings.IndexAny(s, "\r\n "); j >= 0 {
			s = s[:j]
		}
		if strings.Count(s, "/") >= 2 {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
