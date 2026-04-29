package transport

import (
	"testing"
	"warpedrealms/shared"
)

func TestBinarySnapshotGolden(t *testing.T) {
	snap := &shared.SnapshotMessage{ServerTime: 1.5, Tick: 7, LocalPlayerID: "p1", LastProcessedSeq: 3, Entities: []shared.EntityState{{ID: "e1", Name: "n", Kind: shared.EntityKindPlayer, HP: 10, MaxHP: 20, RoomID: "r1"}}, Loot: []shared.LootState{{ID: "l1", Value: 5}}}
	raw := EncodeSnapshot(snap)
	out, err := DecodeSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out.Tick != 7 || out.LocalPlayerID != "p1" || len(out.Entities) != 1 || out.Entities[0].HP != 10 || out.Entities[0].RoomID != "r1" {
		t.Fatalf("unexpected %#v", out)
	}
}

func TestWelcomeRoundTrip(t *testing.T) {
	in := &shared.WelcomeMessage{PlayerID: "p1", PlayerName: "u", ClassID: "knight", RaidID: "r", RaidName: "raid", ContentVersion: "v", ServerTime: 1, TickRate: 60, SnapshotRate: 20, InterpolationBackTime: 0.1}
	raw := EncodeWelcome(in)
	out, err := DecodeWelcome(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out.PlayerID != in.PlayerID || out.TickRate != in.TickRate {
		t.Fatalf("unexpected %#v", out)
	}
}
