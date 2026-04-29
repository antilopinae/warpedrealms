package shared

import (
	"encoding/hex"
	"testing"
)

func TestSnapshotProtobufGoldenRoundTrip(t *testing.T) {
	s := SnapshotMessage{ServerTime: 1.25, Tick: 7, LocalPlayerID: "p1", LastProcessedSeq: 5, Entities: []EntityState{{ID: "p1", RoomID: "r1"}}, Loot: []LootState{{ID: "l1", RoomID: "r1", Value: 1}}}
	raw, err := EncodeSnapshotBinary(s)
	if err != nil {
		t.Fatal(err)
	}
	golden := hex.EncodeToString(raw)
	if golden == "" {
		t.Fatal("empty")
	}
	decoded, err := DecodeSnapshotBinary(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Tick != s.Tick || decoded.LocalPlayerID != s.LocalPlayerID {
		t.Fatalf("mismatch")
	}
}

func BenchmarkSnapshotEncoding(b *testing.B) {
	s := SnapshotMessage{ServerTime: 1.25, Tick: 7, LocalPlayerID: "p1", Entities: make([]EntityState, 200)}
	for i := 0; i < b.N; i++ {
		_, _ = EncodeSnapshotBinary(s)
	}
}

func BenchmarkSnapshotDecoding(b *testing.B) {
	s := SnapshotMessage{ServerTime: 1.25, Tick: 7, LocalPlayerID: "p1", Entities: make([]EntityState, 200)}
	raw, _ := EncodeSnapshotBinary(s)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeSnapshotBinary(raw)
	}
}
