package codex

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

const maximumMessageSize = 1024 * 1024

type client struct {
	connection       net.Conn
	stopCancellation func() bool
	reader           *bufio.Reader
	writer           *bufio.Writer
	nextID           int
}

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type thread struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Preview          string          `json:"preview"`
	WorkingDirectory string          `json:"cwd"`
	Status           threadStatus    `json:"status"`
	ParentThreadID   string          `json:"parentThreadId"`
	Source           json.RawMessage `json:"source"`
	UpdatedAt        int64           `json:"updatedAt"`
}

func connect(ctx context.Context, socketPath string) (*client, error) {
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to Codex daemon: %w", err)
	}

	// Closing the connection stops a blocked read when a poll is cancelled.
	stopCancellation := context.AfterFunc(ctx, func() { connection.Close() })
	handshakeComplete := false
	defer func() {
		if !handshakeComplete {
			stopCancellation()
		}
	}()

	if deadline, ok := ctx.Deadline(); ok {
		if err := connection.SetDeadline(deadline); err != nil {
			connection.Close()
			return nil, fmt.Errorf("set Codex connection deadline: %w", err)
		}
	}

	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	key, err := webSocketKey()
	if err != nil {
		connection.Close()
		return nil, err
	}

	if _, err := fmt.Fprintf(writer, "GET / HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", key); err != nil {
		connection.Close()
		return nil, fmt.Errorf("send WebSocket handshake: %w", err)
	}
	if err := writer.Flush(); err != nil {
		connection.Close()
		return nil, fmt.Errorf("flush WebSocket handshake: %w", err)
	}

	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("read WebSocket handshake: %w", err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		connection.Close()
		return nil, fmt.Errorf("Codex rejected WebSocket handshake: %s", response.Status)
	}
	if response.Header.Get("Sec-WebSocket-Accept") != webSocketAccept(key) {
		connection.Close()
		return nil, fmt.Errorf("Codex returned an invalid WebSocket handshake")
	}

	handshakeComplete = true
	return &client{
		connection:       connection,
		reader:           reader,
		writer:           writer,
		nextID:           1,
		stopCancellation: stopCancellation,
	}, nil
}

func (client *client) Close() error {
	if client.stopCancellation != nil {
		client.stopCancellation()
	}
	return client.connection.Close()
}

func (client *client) initialize() error {
	_, err := client.request("initialize", map[string]any{
		"clientInfo": map[string]string{
			"name":    "a-gent",
			"title":   "a-gent",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return err
	}

	return client.writeJSON(map[string]any{"method": "initialized", "params": map[string]any{}})
}

func (client *client) loadedThreadIDs() ([]string, error) {
	result, err := client.request("thread/loaded/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var loadedThreads struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(result, &loadedThreads); err != nil {
		return nil, fmt.Errorf("decode loaded Codex threads: %w", err)
	}

	return loadedThreads.Data, nil
}

func (client *client) thread(threadID string) (thread, error) {
	result, err := client.request("thread/read", map[string]any{
		"threadId":     threadID,
		"includeTurns": false,
	})
	if err != nil {
		return thread{}, err
	}

	var response struct {
		Thread thread `json:"thread"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return thread{}, fmt.Errorf("decode Codex thread: %w", err)
	}

	return response.Thread, nil
}

func (client *client) request(method string, params any) (json.RawMessage, error) {
	requestID := client.nextID
	client.nextID++

	if err := client.writeJSON(map[string]any{"id": requestID, "method": method, "params": params}); err != nil {
		return nil, err
	}

	for {
		message, err := client.readJSON()
		if err != nil {
			return nil, err
		}

		var response rpcResponse
		if err := json.Unmarshal(message, &response); err != nil {
			return nil, fmt.Errorf("decode Codex response: %w", err)
		}
		if response.ID != requestID {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("Codex returned %d: %s", response.Error.Code, response.Error.Message)
		}

		return response.Result, nil
	}
}

func (client *client) writeJSON(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return client.writeFrame(0x1, payload)
}

func (client *client) readJSON() ([]byte, error) {
	for {
		opcode, payload, err := client.readFrame()
		if err != nil {
			return nil, err
		}

		switch opcode {
		case 0x1:
			return payload, nil
		case 0x8:
			return nil, io.EOF
		case 0x9:
			if err := client.writeFrame(0xA, payload); err != nil {
				return nil, err
			}
		}
	}
}

func (client *client) writeFrame(opcode byte, payload []byte) error {
	if len(payload) > maximumMessageSize {
		return fmt.Errorf("WebSocket message is too large")
	}

	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}

	header := []byte{0x80 | opcode}
	switch {
	case len(payload) <= 125:
		header = append(header, 0x80|byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 0x80|126, byte(len(payload)>>8), byte(len(payload)))
	default:
		header = append(header, 0x80|127, 0, 0, 0, 0)
		header = binary.BigEndian.AppendUint32(header, uint32(len(payload)))
	}

	if _, err := client.writer.Write(header); err != nil {
		return err
	}
	if _, err := client.writer.Write(mask); err != nil {
		return err
	}

	maskedPayload := make([]byte, len(payload))
	for index, value := range payload {
		maskedPayload[index] = value ^ mask[index%len(mask)]
	}
	if _, err := client.writer.Write(maskedPayload); err != nil {
		return err
	}

	return client.writer.Flush()
}

func (client *client) readFrame() (byte, []byte, error) {
	firstByte, err := client.reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	secondByte, err := client.reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	payloadLength := int(secondByte & 0x7F)
	switch payloadLength {
	case 126:
		var length uint16
		if err := binary.Read(client.reader, binary.BigEndian, &length); err != nil {
			return 0, nil, err
		}
		payloadLength = int(length)
	case 127:
		var length uint64
		if err := binary.Read(client.reader, binary.BigEndian, &length); err != nil {
			return 0, nil, err
		}
		if length > maximumMessageSize {
			return 0, nil, fmt.Errorf("WebSocket message is too large")
		}
		payloadLength = int(length)
	}
	if payloadLength > maximumMessageSize {
		return 0, nil, fmt.Errorf("WebSocket message is too large")
	}

	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(client.reader, payload); err != nil {
		return 0, nil, err
	}

	return firstByte & 0x0F, payload, nil
}

func webSocketKey() (string, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("create WebSocket key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(key), nil
}

func webSocketAccept(key string) string {
	const webSocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	digest := sha1.Sum([]byte(key + webSocketGUID))
	return base64.StdEncoding.EncodeToString(digest[:])
}
