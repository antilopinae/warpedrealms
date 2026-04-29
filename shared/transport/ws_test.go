package transport

import (
	"testing"
	"warpedrealms/shared"
	"warpedrealms/shared/minipb"
)

func TestBinarySnapshotGolden(t *testing.T) {
	snap := &shared.SnapshotMessage{ServerTime: 1.5, Tick: 7, LocalPlayerID: "p1", LastProcessedSeq: 3, Entities: []shared.EntityState{{ID: "e1", Name: "n", Kind: shared.EntityKindPlayer, HP: 10, MaxHP: 20}}, Loot: []shared.LootState{{ID: "l1", Value: 5}}}
	raw := EncodeSnapshot(snap, minipb.LittleEndian)
	out, err := DecodeSnapshot(raw, minipb.LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if out.Tick != 7 || out.LocalPlayerID != "p1" || len(out.Entities) != 1 || out.Entities[0].HP != 10 {
		t.Fatalf("unexpected %#v", out)
	}
}
