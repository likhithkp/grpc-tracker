package grpcxlogger

import (
	"log"
	"net"
	"os"
	"reflect"

	"google.golang.org/grpc"
)

var logger = log.New(os.Stdout, "[AUTO-GRPC-LOGGER] ", log.LstdFlags)

func init() {
	logger.Println("🚀 Auto gRPC logger initialized — will log all incoming gRPC calls")

	// Monkey patch grpc.Serve (safe reflection)
	patchServe()
}

func patchServe() {
	// We’ll get the reflect.Value of grpc.(*Server).Serve
	serveMethod, ok := reflect.TypeOf(&grpc.Server{}).MethodByName("Serve")
	if !ok {
		logger.Println("⚠️ Could not find grpc.Server.Serve method")
		return
	}

	// Wrap Serve using method value interception
	orig := serveMethod.Func

	wrapper := func(args []reflect.Value) []reflect.Value {
		srv := args[0].Interface().(*grpc.Server)
		lis := args[1].Interface().(net.Listener)

		logger.Printf("🧩 Intercepted grpc.Server.Serve on %v", lis.Addr())

		// Wrap the listener so we can log every accepted connection
		wrappedLis := &loggingListener{Listener: lis}
		return orig.Call([]reflect.Value{
			reflect.ValueOf(srv),
			reflect.ValueOf(wrappedLis),
		})
	}

	// Replace Serve
	reflect.ValueOf(&grpc.Server{}).Elem()
	reflect.ValueOf(&serveMethod.Func).Set(reflect.ValueOf(wrapper))
}

type loggingListener struct {
	net.Listener
}

func (l *loggingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	logger.Printf("🔗 New gRPC connection from %s", conn.RemoteAddr())
	return &loggingConn{Conn: conn}, nil
}

type loggingConn struct {
	net.Conn
}

func (c *loggingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		logger.Printf("📩 Received %d bytes from %s", n, c.RemoteAddr())
	}
	return n, err
}

func (c *loggingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		logger.Printf("📤 Sent %d bytes to %s", n, c.RemoteAddr())
	}
	return n, err
}
