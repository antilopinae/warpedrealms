package transport

import (
	"fmt"
	"github.com/gorilla/websocket"
	"warpedrealms/shared"
	"warpedrealms/shared/pbwire"
)

type Encoding string

const (
	EncodingProtobuf Encoding = "protobuf"
)

func WriteServerMessage(conn *websocket.Conn, _ Encoding, m shared.ServerMessage) error {
	w := &pbwire.Writer{}
	w.String(1, m.Type)
	switch m.Type {
	case "snapshot":
		if m.Snapshot == nil {
			return fmt.Errorf("snapshot nil")
		}
		w.Message(2, EncodeSnapshot(m.Snapshot))
	case "welcome":
		if m.Welcome == nil {
			return fmt.Errorf("welcome nil")
		}
		w.Message(3, EncodeWelcome(m.Welcome))
	case "pong":
		if m.Pong == nil {
			return fmt.Errorf("pong nil")
		}
		w.Message(4, EncodePong(m.Pong))
	case "error":
		w.String(9, m.Error)
	}
	return conn.WriteMessage(websocket.BinaryMessage, w.Bytes())
}
func ReadServerMessage(msgType int, data []byte, out *shared.ServerMessage) error {
	if msgType != websocket.BinaryMessage {
		return fmt.Errorf("binary only")
	}
	r := pbwire.NewReader(data)
	for {
		f, _, p, err := r.Next()
		if err != nil {
			break
		}
		switch f {
		case 1:
			out.Type = string(p)
		case 2:
			s, err := DecodeSnapshot(p)
			if err != nil {
				return err
			}
			out.Snapshot = &s
		case 3:
			w, err := DecodeWelcome(p)
			if err != nil {
				return err
			}
			out.Welcome = &w
		case 4:
			pg, err := DecodePong(p)
			if err != nil {
				return err
			}
			out.Pong = &pg
		case 9:
			out.Error = string(p)
		}
	}
	return nil
}
