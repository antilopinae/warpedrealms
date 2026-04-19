package serverapp

import (
	"path/filepath"
	"testing"
	"time"

	"warpedrealms/content"
	"warpedrealms/shared"
)

func TestGeneratedRaidHasExpectedRoomCount(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	raid, err := bundle.GenerateRaid(12345)
	if err != nil {
		t.Fatalf("generate raid: %v", err)
	}
	if len(raid.Layout.Rooms) < shared.GeneratedRoomCountMin || len(raid.Layout.Rooms) > shared.GeneratedRoomCountMax {
		t.Fatalf("unexpected room count %d", len(raid.Layout.Rooms))
	}
}

func TestLastPlayerLeaveDoesNotFinishRaidAndAllowsRejoin(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	room, err := NewRaidRoom("raid-test", "Raid Test", bundle, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration, 12345)
	if err != nil {
		t.Fatalf("new raid room: %v", err)
	}
	room.Start()

	firstPeer := &Peer{
		playerID:   "player-1",
		playerName: "player",
		classID:    shared.PlayerClassKnight,
		send:       make(chan shared.ServerMessage, 8),
	}
	if err := room.Join(firstPeer); err != nil {
		t.Fatalf("join first peer: %v", err)
	}

	waitForRoomPhase(t, room, shared.RaidPhaseActive)

	room.Leave(firstPeer.playerID, firstPeer)
	waitForPeerCount(t, room, 0)

	if summary := room.Summary(); summary.Phase != shared.RaidPhaseActive {
		t.Fatalf("expected active raid after last disconnect, got %s", summary.Phase)
	}

	secondPeer := &Peer{
		playerID:   "player-1",
		playerName: "player",
		classID:    shared.PlayerClassKnight,
		send:       make(chan shared.ServerMessage, 8),
	}
	if err := room.Join(secondPeer); err != nil {
		t.Fatalf("rejoin peer: %v", err)
	}
}

func TestHiddenMimicSnapshotDoesNotLeakMimicDetails(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	room, err := NewRaidRoom("raid-test", "Raid Test", bundle, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration, 12345)
	if err != nil {
		t.Fatalf("new raid room: %v", err)
	}

	npc := hiddenMimicFromRoom(t, room)
	snapshot := room.snapshotFor("")

	for _, entity := range snapshot.Entities {
		if entity.Position != npc.state.Position {
			continue
		}
		if entity.Kind != shared.EntityKindProp {
			t.Fatalf("expected concealed mimic kind prop, got %s", entity.Kind)
		}
		if entity.ProfileID != npc.disguiseProfile {
			t.Fatalf("expected disguise profile %s, got %s", npc.disguiseProfile, entity.ProfileID)
		}
		if entity.Name != "" || entity.HP != 0 || entity.MaxHP != 0 {
			t.Fatalf("concealed mimic leaked name/hp: %+v", entity)
		}
		if entity.Faction != shared.FactionNeutral {
			t.Fatalf("expected concealed mimic neutral, got %s", entity.Faction)
		}
		if entity.ID == npc.state.ID || entity.FamilyID == npc.state.FamilyID {
			t.Fatalf("concealed mimic leaked raw identity: %+v", entity)
		}
		return
	}

	t.Fatal("concealed mimic not present in snapshot")
}

func TestHiddenMimicOpensOnlyOnInteract(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	room, err := NewRaidRoom("raid-test", "Raid Test", bundle, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration, 12345)
	if err != nil {
		t.Fatalf("new raid room: %v", err)
	}

	npc := hiddenMimicFromRoom(t, room)
	startHP := npc.state.HP
	room.applyDamageToNPC(npc, 999, npc.state.Position, room.serverTime())
	if npc.awakened {
		t.Fatal("hidden mimic awakened from damage before interact")
	}
	if npc.state.HP != startHP {
		t.Fatalf("hidden mimic took damage before reveal: got %d want %d", npc.state.HP, startHP)
	}

	classDef, ok := bundle.Manifest.Class(shared.PlayerClassKnight)
	if !ok {
		t.Fatal("knight class missing")
	}
	profile, ok := bundle.Manifest.Profile(classDef.ProfileID)
	if !ok {
		t.Fatal("knight profile missing")
	}
	player := &serverPlayer{
		class:     classDef,
		profile:   profile,
		status:    shared.PlayerRaidStatusActive,
		cooldowns: make(map[string]float64),
		state: shared.EntityState{
			ID:             "player-test",
			Name:           "player",
			ClassID:        classDef.ID,
			Kind:           shared.EntityKindPlayer,
			Faction:        shared.FactionPlayers,
			ProfileID:      profile.ID,
			Position:       npc.state.Position,
			RoomID:         npc.state.RoomID,
			Collider:       profile.Collider,
			Hurtbox:        profile.Hurtbox,
			InteractionBox: profile.InteractionBox,
			SpriteSize:     profile.SpriteSize,
			SpriteOffset:   profile.SpriteOffset,
			Scale:          profile.Scale,
			MaxHP:          profile.MaxHP,
			HP:             profile.MaxHP,
			Facing:         1,
		},
	}

	now := room.serverTime()
	room.processInteraction(player, now)

	if !npc.awakened {
		t.Fatal("hidden mimic did not awaken on interact")
	}
	if npc.state.Animation != shared.AnimationEmerge {
		t.Fatalf("expected emerge animation, got %s", npc.state.Animation)
	}
}

func hiddenMimicFromRoom(t *testing.T, room *RaidRoom) *serverNPC {
	t.Helper()
	for _, npc := range room.npcs {
		if npc.state.Kind == shared.EntityKindMimic && !npc.awakened && npc.disguiseProfile != "" {
			return npc
		}
	}
	t.Fatal("no hidden mimic in generated room")
	return nil
}

func waitForRoomPhase(t *testing.T, room *RaidRoom, phase shared.RaidPhase) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if room.Summary().Phase == phase {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("room phase %s was not observed", phase)
}

func waitForPeerCount(t *testing.T, room *RaidRoom, count int) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(room.peers) == count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("peer count %d was not observed", count)
}
