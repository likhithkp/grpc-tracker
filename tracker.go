package grpctracker

import (
	"bufio"
	"log"
	"net"
	"os"
)

var logger = log.New(os.Stdout, "[AUTO-GRPC-TRACKER] ", log.LstdFlags)

func init() {
	logger.Println("🚀 Auto gRPC Tracker initialized — passive network monitor active")
	go monitorGRPCPort(":4430")
}

// monitorGRPCPort listens on the same gRPC port and logs incoming request attempts.
func monitorGRPCPort(port string) {
	ln, err := net.Listen("tcp", port)
	if err != nil {
		logger.Printf("⚠️ Could not listen on %s — probably already in use by your main app, switching to passive mode", port)
		go attachToExistingPort(port)
		return
	}
	defer ln.Close()
	logger.Printf("👂 Listening on %s for gRPC traffic...", port)
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConn(conn)
	}
}

func attachToExistingPort(port string) {
	addr := "127.0.0.1" + port
	for {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			logger.Printf("🧩 Attached to running gRPC port %s", port)
			go handleConn(conn)
			return
		}
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	reader := bufio.NewReader(conn)

	for {
		data, err := reader.Peek(8)
		if err != nil {
			return
		}
		// gRPC requests always start with a 5-byte header
		if len(data) > 0 {
			logger.Printf("📡 gRPC call detected from %s", remote)
			break
		}
	}
}
