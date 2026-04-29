package transport

import (
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
	"warpedrealms/shared"
)

func TestBinarySnapshotGolden(t *testing.T) {
	msg := shared.ServerMessage{Type: "snapshot", Snapshot: &shared.SnapshotMessage{Tick: 7, LocalPlayerID: "p1"}}
	payload, _ := json.Marshal(msg.Snapshot)
	frame := append([]byte{1, 8}, []byte("snapshot")...)
	frame = append(frame, payload...)
	var out shared.ServerMessage
	if err := ReadServerMessage(websocket.BinaryMessage, frame, &out); err != nil { t.Fatal(err) }
	if out.Type != "snapshot" || out.Snapshot == nil || out.Snapshot.Tick != 7 { t.Fatalf("unexpected %#v", out) }
}

func BenchmarkSnapshotDecode(b *testing.B) {
	msg := shared.ServerMessage{Type: "snapshot", Snapshot: &shared.SnapshotMessage{Tick: 7, LocalPlayerID: "p1"}}
	payload, _ := json.Marshal(msg.Snapshot)
	frame := append([]byte{1, 8}, []byte("snapshot")...)
	frame = append(frame, payload...)
	var out shared.ServerMessage
	b.ReportAllocs()
	for i:=0;i<b.N;i++ { _ = ReadServerMessage(websocket.BinaryMessage, frame, &out) }
}
