package codex

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// WebSocket frame fields and payload limits come from RFC 6455, section 5.2.
const (
	testTextFrame          = byte(1)
	testCloseFrame         = byte(8)
	testPingFrame          = byte(9)
	testPongFrame          = byte(10)
	testFinalFrame         = byte(128)
	testMaskFlag           = byte(128)
	testMaskBytes          = 4
	testShortPayloadLimit  = 125
	testMediumPayloadLimit = 65535
	testMediumLengthMarker = byte(126)
	testLargeLengthMarker  = byte(127)
)

func TestClientRequestsFollowDaemonProtocol(t *testing.T) {
	var incoming bytes.Buffer
	for _, response := range []string{
		`{"method":"thread/status/changed","params":{}}`,
		`{"id":99,"result":{"ignored":true}}`,
		`{"id":1,"result":{}}`,
		`{"id":2,"result":{"data":["main","worker"]}}`,
		`{"id":3,"result":{"thread":{"id":"main","name":"Fix bug","cwd":"/workspace","status":{"type":"idle"}}}}`,
	} {
		incoming.Write(serverFrame(testTextFrame, []byte(response)))
	}

	var outgoing bytes.Buffer
	client := bufferedTestClient(&incoming, &outgoing)
	if err := client.initialize(); err != nil {
		t.Fatal(err)
	}
	ids, err := client.loadedThreadIDs()
	if err != nil || !reflect.DeepEqual(ids, []string{"main", "worker"}) {
		t.Fatalf("loaded threads = %v, %v", ids, err)
	}
	loaded, err := client.thread("main")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "main" || loaded.Name != "Fix bug" ||
		loaded.WorkingDirectory != "/workspace" || loaded.Status.Type != "idle" {
		t.Fatalf("decoded thread = %+v", loaded)
	}

	reader := bufio.NewReader(&outgoing)
	var requests []map[string]any
	for reader.Buffered() > 0 || outgoing.Len() > 0 {
		opcode, payload, err := readMaskedClientFrame(reader)
		if err != nil {
			t.Fatal(err)
		}
		if opcode != testTextFrame {
			t.Fatalf("request opcode = %d", opcode)
		}

		var request map[string]any
		if err := json.Unmarshal(payload, &request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
	}

	const expectedRequests = `[
		{"id":1,"method":"initialize","params":{"clientInfo":{"name":"a-gent","title":"a-gent","version":"0.1.0"}}},
		{"method":"initialized","params":{}},
		{"id":2,"method":"thread/loaded/list","params":{}},
		{"id":3,"method":"thread/read","params":{"threadId":"main","includeTurns":false}}
	]`
	var expected []map[string]any
	if err := json.Unmarshal([]byte(expectedRequests), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requests, expected) {
		t.Fatalf("requests = %#v, want %#v", requests, expected)
	}
}

func TestClientResponseErrors(t *testing.T) {
	testCases := []struct {
		name     string
		response string
		call     func(*client) error
		want     string
	}{
		{
			name: "initialize rejected", response: `{"id":1,"error":{"code":-32000,"message":"unavailable"}}`,
			call: (*client).initialize, want: "Codex returned -32000: unavailable",
		},
		{
			name: "invalid response JSON", response: `{`,
			call: (*client).initialize, want: "decode Codex response",
		},
		{
			name: "loaded threads rejected", response: `{"id":1,"error":{"code":-1,"message":"not initialized"}}`,
			call: func(client *client) error {
				_, err := client.loadedThreadIDs()
				return err
			},
			want: "not initialized",
		},
		{
			name: "loaded threads wrong shape", response: `{"id":1,"result":{"data":42}}`,
			call: func(client *client) error {
				_, err := client.loadedThreadIDs()
				return err
			},
			want: "decode loaded Codex threads",
		},
		{
			name: "thread rejected", response: `{"id":1,"error":{"code":-2,"message":"thread gone"}}`,
			call: func(client *client) error {
				_, err := client.thread("gone")
				return err
			},
			want: "thread gone",
		},
		{
			name: "thread wrong shape", response: `{"id":1,"result":{"thread":42}}`,
			call: func(client *client) error {
				_, err := client.thread("main")
				return err
			},
			want: "decode Codex thread",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			incoming := bytes.NewReader(serverFrame(testTextFrame, []byte(testCase.response)))
			var outgoing bytes.Buffer
			client := bufferedTestClient(incoming, &outgoing)

			if err := testCase.call(client); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestClientAnswersPingBeforeReadingResponse(t *testing.T) {
	incoming := append(serverFrame(testPingFrame, []byte("alive")), serverFrame(testTextFrame, []byte(`{"ok":true}`))...)
	var outgoing bytes.Buffer
	client := bufferedTestClient(bytes.NewReader(incoming), &outgoing)

	payload, err := client.readJSON()
	if err != nil || string(payload) != `{"ok":true}` {
		t.Fatalf("response = %s, %v", payload, err)
	}
	opcode, pong, err := readMaskedClientFrame(bufio.NewReader(&outgoing))
	if err != nil || opcode != testPongFrame || string(pong) != "alive" {
		t.Fatalf("pong = %d, %q, %v", opcode, pong, err)
	}
}

func TestClientHandlesClosedAndTruncatedFrames(t *testing.T) {
	mediumFrame := serverFrame(testTextFrame, bytes.Repeat([]byte("m"), testShortPayloadLimit+1))
	largeFrame := serverFrame(testTextFrame, bytes.Repeat([]byte("l"), testMediumPayloadLimit+1))
	testCases := []struct {
		name  string
		frame []byte
		want  error
	}{
		{name: "closed connection", want: io.EOF},
		{name: "close frame", frame: serverFrame(testCloseFrame, nil), want: io.EOF},
		{name: "missing length", frame: []byte{testFinalFrame | testTextFrame}, want: io.EOF},
		{name: "truncated medium length", frame: mediumFrame[:3], want: io.ErrUnexpectedEOF},
		{name: "truncated large length", frame: largeFrame[:4], want: io.ErrUnexpectedEOF},
		{name: "truncated payload", frame: serverFrame(testTextFrame, []byte("text"))[:4], want: io.ErrUnexpectedEOF},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := bufferedTestClient(bytes.NewReader(testCase.frame), io.Discard)
			if _, err := client.readJSON(); !errors.Is(err, testCase.want) {
				t.Fatalf("read error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestClientFrameSizeBoundaries(t *testing.T) {
	sizes := []int{
		0,
		testShortPayloadLimit,
		testShortPayloadLimit + 1,
		testMediumPayloadLimit,
		testMediumPayloadLimit + 1,
		maximumMessageSize,
	}
	for _, size := range sizes {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			payload := bytes.Repeat([]byte("x"), size)
			var outgoing bytes.Buffer
			client := bufferedTestClient(bytes.NewReader(serverFrame(testTextFrame, payload)), &outgoing)

			if err := client.writeFrame(testTextFrame, payload); err != nil {
				t.Fatal(err)
			}
			opcode, written, err := readMaskedClientFrame(bufio.NewReader(&outgoing))
			if err != nil || opcode != testTextFrame || !bytes.Equal(written, payload) {
				t.Fatalf("written frame opcode = %d, size = %d, error = %v", opcode, len(written), err)
			}

			opcode, read, err := client.readFrame()
			if err != nil || opcode != testTextFrame || !bytes.Equal(read, payload) {
				t.Fatalf("read frame opcode = %d, size = %d, error = %v", opcode, len(read), err)
			}
		})
	}

	tooLarge := bytes.Repeat([]byte("x"), maximumMessageSize+1)
	var outgoing bytes.Buffer
	client := bufferedTestClient(bytes.NewReader(serverFrame(testTextFrame, tooLarge)), &outgoing)
	if err := client.writeFrame(testTextFrame, tooLarge); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized write error = %v", err)
	}
	if outgoing.Len() != 0 {
		t.Fatal("oversized frame was partially written")
	}
	if _, _, err := client.readFrame(); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized read error = %v", err)
	}
}

func TestClientPropagatesWriteFailures(t *testing.T) {
	writeFailure := errors.New("connection lost")
	client := bufferedTestClient(strings.NewReader(""), failingClientWriter{writeFailure})
	if _, err := client.request("thread/read", map[string]any{}); !errors.Is(err, writeFailure) {
		t.Fatalf("request error = %v", err)
	}

	client = bufferedTestClient(bytes.NewReader(serverFrame(testPingFrame, nil)), failingClientWriter{writeFailure})
	if _, err := client.readJSON(); !errors.Is(err, writeFailure) {
		t.Fatalf("pong error = %v", err)
	}

	var outgoing bytes.Buffer
	client = bufferedTestClient(strings.NewReader(""), &outgoing)
	if err := client.writeJSON(make(chan int)); err == nil {
		t.Fatal("unsupported JSON value did not fail")
	}
	if outgoing.Len() != 0 {
		t.Fatal("invalid JSON was written")
	}
}

type failingClientWriter struct {
	err error
}

func (writer failingClientWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func bufferedTestClient(reader io.Reader, writer io.Writer) *client {
	return &client{reader: bufio.NewReader(reader), writer: bufio.NewWriter(writer), nextID: 1}
}

// serverFrame emits unmasked server frames; client frames must carry a mask.
// The explicit wire encoding lets tests independently verify the existing transport.
func serverFrame(opcode byte, payload []byte) []byte {
	var frame bytes.Buffer
	frame.WriteByte(testFinalFrame | opcode)
	switch {
	case len(payload) <= testShortPayloadLimit:
		frame.WriteByte(byte(len(payload)))
	case len(payload) <= testMediumPayloadLimit:
		frame.WriteByte(testMediumLengthMarker)
		binary.Write(&frame, binary.BigEndian, uint16(len(payload)))
	default:
		frame.WriteByte(testLargeLengthMarker)
		binary.Write(&frame, binary.BigEndian, uint64(len(payload)))
	}
	frame.Write(payload)
	return frame.Bytes()
}

func readMaskedClientFrame(reader *bufio.Reader) (byte, []byte, error) {
	first, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	lengthByte, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	if first&testFinalFrame == 0 || lengthByte&testMaskFlag == 0 {
		return 0, nil, errors.New("client frame must be final and masked")
	}

	length := uint64(lengthByte &^ testMaskFlag)
	switch length {
	case uint64(testMediumLengthMarker):
		var mediumLength uint16
		if err := binary.Read(reader, binary.BigEndian, &mediumLength); err != nil {
			return 0, nil, err
		}
		length = uint64(mediumLength)
	case uint64(testLargeLengthMarker):
		if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
			return 0, nil, err
		}
	}
	if length > maximumMessageSize {
		return 0, nil, errors.New("client frame exceeds message limit")
	}

	var mask [testMaskBytes]byte
	if _, err := io.ReadFull(reader, mask[:]); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}
	for index := range payload {
		payload[index] ^= mask[index%len(mask)]
	}
	return first &^ testFinalFrame, payload, nil
}
