package grpctracker

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

var logger = log.New(os.Stdout, "[AUTO-GRPC-TRACKER] ", log.LstdFlags)

func init() {
	logger.Println("🚀 Auto gRPC Tracker initialized — passive attach mode with method sniffing")
	go passiveMonitor(":4430")
}

// passiveMonitor attaches to the running gRPC port and monitors activity.
func passiveMonitor(port string) {
	for {
		conn, err := net.Dial("tcp", "127.0.0.1"+port)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		logger.Printf("🧩 Attached to running gRPC server on %s", port)
		handleConn(conn)
		conn.Close()
		time.Sleep(2 * time.Second)
	}
}

// handleConn reads a little of the TCP stream and tries to extract the :path header
// that contains the gRPC service and method name.
func handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}

		// gRPC over HTTP/2 starts with headers containing ":path"
		if bytes.Contains(line, []byte(":path")) {
			// example: ":path: /ride.RideService/CancelTrip"
			method := extractMethod(string(line))
			if method != "" {
				logger.Printf("📡 gRPC call detected: %s", method)
			} else {
				logger.Printf("📡 gRPC activity detected (unparsed)")
			}
			return
		}
	}
}

func extractMethod(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ":path") {
		return ""
	}
	parts := strings.SplitN(s, ":path:", 2)
	if len(parts) < 2 {
		return ""
	}
	path := strings.TrimSpace(parts[1])
	return path
}
