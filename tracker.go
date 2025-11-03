package grpctracker

import (
	"bufio"
	"log"
	"net"
	"os"
	"time"
)

var logger = log.New(os.Stdout, "[AUTO-GRPC-TRACKER] ", log.LstdFlags)

func init() {
	logger.Println("🚀 Auto gRPC Tracker initialized — passive attach mode")
	go passiveMonitor(":4430")
}

// passiveMonitor tries to attach to an already running gRPC port safely.
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

func handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		data, err := reader.Peek(8)
		if err != nil {
			return
		}

		// gRPC frames begin with a 5-byte header
		if len(data) > 0 {
			logger.Printf("📡 gRPC activity detected on %s", conn.RemoteAddr())
			return
		}
	}
}
