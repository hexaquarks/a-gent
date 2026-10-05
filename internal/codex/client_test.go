package codex

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCancellationClosesBlockedSocketReads(t *testing.T) {
	for _, handshake := range []bool{false, true} {
		t.Run(fmt.Sprintf("handshake=%v", handshake), func(t *testing.T) {
			// Unix socket paths have a small platform limit; keep the name short.
			directory, err := os.MkdirTemp("/tmp", "a-gent-socket-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(directory)
			socket := filepath.Join(directory, "s")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				accepted <- connection
				request, err := http.ReadRequest(bufio.NewReader(connection))
				if err != nil || !handshake {
					return
				}
				fmt.Fprintf(connection, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", webSocketAccept(request.Header.Get("Sec-WebSocket-Key")))
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			reading := make(chan struct{})
			go func() {
				client, err := connect(ctx, socket)
				if err == nil {
					defer client.Close()
					close(reading)
					_, err = client.readJSON()
				}
				result <- err
			}()
			select {
			case connection := <-accepted:
				defer connection.Close()
			case <-time.After(time.Second):
				t.Fatal("client did not connect")
			}
			if handshake {
				select {
				case <-reading:
				case <-time.After(time.Second):
					t.Fatal("handshake did not finish")
				}
			}
			cancel()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("cancelled read returned no error")
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation did not interrupt socket read")
			}
		})
	}
}
