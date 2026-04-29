package transport

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
	"warpedrealms/shared"
)

type Encoding string

const (
	EncodingJSON     Encoding = "json"
	EncodingProtobuf Encoding = "protobuf"
)

// Binary frame envelope: [1 byte version][1 byte typeLen][type bytes][payload bytes].
func WriteServerMessage(conn *websocket.Conn, enc Encoding, m shared.ServerMessage) error {
	if enc == EncodingJSON {
		return conn.WriteJSON(m)
	}
	if m.Type != "snapshot" || m.Snapshot == nil {
		return conn.WriteJSON(m)
	}
	payload, err := json.Marshal(m.Snapshot)
	if err != nil {
		return err
	}
	typeBytes := []byte(m.Type)
	if len(typeBytes) > 255 {
		return fmt.Errorf("type too long")
	}
	frame := make([]byte, 0, 2+len(typeBytes)+len(payload))
	frame = append(frame, 1, byte(len(typeBytes)))
	frame = append(frame, typeBytes...)
	frame = append(frame, payload...)
	return conn.WriteMessage(websocket.BinaryMessage, frame)
}

func ReadServerMessage(msgType int, data []byte, out *shared.ServerMessage) error {
	if msgType == websocket.TextMessage {
		return json.Unmarshal(data, out)
	}
	if msgType != websocket.BinaryMessage || len(data) < 2 {
		return fmt.Errorf("unsupported frame")
	}
	typeLen := int(data[1])
	if len(data) < 2+typeLen {
		return fmt.Errorf("bad frame")
	}
	out.Type = string(data[2 : 2+typeLen])
	if out.Type == "snapshot" {
		var snap shared.SnapshotMessage
		if err := json.Unmarshal(data[2+typeLen:], &snap); err != nil {
			return err
		}
		out.Snapshot = &snap
	}
	return nil
}
