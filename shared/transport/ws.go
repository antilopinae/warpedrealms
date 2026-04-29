package transport

import (
	"fmt"
	"github.com/gorilla/websocket"
	"warpedrealms/shared"
	"warpedrealms/shared/minipb"
)

type Encoding string

const (
	EncodingProtobuf Encoding = "protobuf"
)

func WriteServerMessage(conn *websocket.Conn, _ Encoding, m shared.ServerMessage) error {
	w := minipb.NewWriter(minipb.LittleEndian)
	w.Field(1, []byte(m.Type))
	switch m.Type {
	case "snapshot":
		if m.Snapshot == nil {
			return fmt.Errorf("snapshot nil")
		}
		w.Field(2, EncodeSnapshot(m.Snapshot, minipb.LittleEndian))
	case "welcome":
		if m.Welcome != nil {
			w.Field(3, []byte(m.Welcome.PlayerID))
		}
	case "error":
		w.Field(9, []byte(m.Error))
	}
	return conn.WriteMessage(websocket.BinaryMessage, w.Bytes())
}

func ReadServerMessage(msgType int, data []byte, out *shared.ServerMessage) error {
	if msgType != websocket.BinaryMessage {
		return fmt.Errorf("binary only")
	}
	r := minipb.NewReader(data, minipb.LittleEndian)
	for {
		tag, p, err := r.Next()
		if err != nil {
			break
		}
		switch tag {
		case 1:
			out.Type = string(p)
		case 2:
			s, err := DecodeSnapshot(p, minipb.LittleEndian)
			if err != nil {
				return err
			}
			out.Snapshot = &s
		case 9:
			out.Error = string(p)
		}
	}
	return nil
}
