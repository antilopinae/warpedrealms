// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

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

func TestJoinUsesRequestedClassAndWelcomeEchoesIt(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	room, err := NewRaidRoom("raid-test", "Raid Test", bundle, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration, 12345)
	if err != nil {
		t.Fatalf("new raid room: %v", err)
	}
	room.Start()

	peer := &Peer{
		playerID:   "player-archer",
		playerName: "player",
		classID:    shared.PlayerClassKnight,
		send:       make(chan shared.ServerMessage, 8),
	}
	if err := room.Join(peer); err != nil {
		t.Fatalf("join peer: %v", err)
	}

	player := room.players[peer.playerID]
	if player == nil {
		t.Fatal("player missing after join")
	}
	if player.class.ID != shared.PlayerClassKnight {
		t.Fatalf("expected class %s, got %s", shared.PlayerClassKnight, player.class.ID)
	}
	if player.state.ClassID != shared.PlayerClassKnight {
		t.Fatalf("expected state class %s, got %s", shared.PlayerClassKnight, player.state.ClassID)
	}
	if player.profile.ID != player.class.ProfileID {
		t.Fatalf("expected profile %s, got %s", player.class.ProfileID, player.profile.ID)
	}

	message := <-peer.send
	if message.Welcome == nil {
		t.Fatal("expected welcome message")
	}
	if message.Welcome.ClassID != shared.PlayerClassKnight {
		t.Fatalf("expected welcome class %s, got %s", shared.PlayerClassKnight, message.Welcome.ClassID)
	}
}

func TestJoinInvalidClassFallsBackToDefaultClass(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	room, err := NewRaidRoom("raid-test", "Raid Test", bundle, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration, 12345)
	if err != nil {
		t.Fatalf("new raid room: %v", err)
	}
	room.Start()

	peer := &Peer{
		playerID:   "player-default",
		playerName: "player",
		classID:    "not-a-class",
		send:       make(chan shared.ServerMessage, 8),
	}
	if err := room.Join(peer); err != nil {
		t.Fatalf("join peer: %v", err)
	}

	player := room.players[peer.playerID]
	if player == nil {
		t.Fatal("player missing after join")
	}
	if player.class.ID != shared.PlayerClassKnight {
		t.Fatalf("expected default class %s, got %s", shared.PlayerClassKnight, player.class.ID)
	}
	if player.state.ClassID != shared.PlayerClassKnight {
		t.Fatalf("expected default state class %s, got %s", shared.PlayerClassKnight, player.state.ClassID)
	}
}

func TestJoinClassWithMissingProfileFallsBackToDefaultClass(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	room, err := NewRaidRoom("raid-test", "Raid Test", bundle, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration, 12345)
	if err != nil {
		t.Fatalf("new raid room: %v", err)
	}
	room.Start()

	peer := &Peer{
		playerID:   "player-broken-class",
		playerName: "player",
		classID:    shared.PlayerClassArcherAssassin,
		send:       make(chan shared.ServerMessage, 8),
	}
	if err := room.Join(peer); err != nil {
		t.Fatalf("join peer: %v", err)
	}

	player := room.players[peer.playerID]
	if player == nil {
		t.Fatal("player missing after join")
	}
	if player.class.ID != shared.PlayerClassKnight {
		t.Fatalf("expected fallback class %s, got %s", shared.PlayerClassKnight, player.class.ID)
	}
}

func TestSimulatePlayerDoesNotPanicWithMissingSkillSlots(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	room, err := NewRaidRoom("raid-test", "Raid Test", bundle, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration, 12345)
	if err != nil {
		t.Fatalf("new raid room: %v", err)
	}

	profile, ok := bundle.Manifest.Profile("player_knight")
	if !ok {
		t.Fatal("player_knight profile missing")
	}
	player := &serverPlayer{
		class: content.ClassDefinition{
			ID:        "stub-class",
			ProfileID: profile.ID,
			Skills: []content.AbilityDefinition{
				{ID: "basic_slash", Name: "Basic Slash", Cooldown: 0.35},
			},
		},
		profile:   profile,
		status:    shared.PlayerRaidStatusActive,
		cooldowns: make(map[string]float64),
		state: func() shared.EntityState {
			state := profile.DefaultState()
			state.ID = "player-test"
			state.Name = "player"
			state.RoomID = room.startRoomID()
			return state
		}(),
	}

	room.simulatePlayer(player, shared.InputCommand{
		PrimaryAttack: true,
		Skill1:        true,
		Skill2:        true,
		Skill3:        true,
	}, room.serverTime())

	if len(player.cooldowns) > 1 {
		t.Fatalf("expected only one cooldown entry, got %d", len(player.cooldowns))
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

func TestSnapshotSendsLayoutOnlyOncePerPeer(t *testing.T) {
	bundle, err := content.LoadBundle(filepath.Join("..", shared.DefaultAssetManifestPath), filepath.Join("..", shared.DefaultRoomsDir))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	room, err := NewRaidRoom("raid-test", "Raid Test", bundle, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration, 12345)
	if err != nil {
		t.Fatalf("new raid room: %v", err)
	}

	peer := &Peer{
		playerID:   "player-layout",
		playerName: "player",
		classID:    shared.PlayerClassKnight,
		send:       make(chan shared.ServerMessage, 8),
	}
	room.peers[peer.playerID] = peer

	first := room.snapshotFor(peer.playerID)
	if first.Layout == nil {
		t.Fatal("expected first snapshot to include layout")
	}

	second := room.snapshotFor(peer.playerID)
	if second.Layout != nil {
		t.Fatal("expected second snapshot to omit layout")
	}

	rejoinedPeer := &Peer{
		playerID:   peer.playerID,
		playerName: "player",
		classID:    shared.PlayerClassKnight,
		send:       make(chan shared.ServerMessage, 8),
	}
	room.peers[peer.playerID] = rejoinedPeer

	rejoinSnapshot := room.snapshotFor(rejoinedPeer.playerID)
	if rejoinSnapshot.Layout == nil {
		t.Fatal("expected rejoin snapshot to include layout for new peer")
	}
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

	profile, ok := room.bundle.Manifest.Profile("mob_rat")
	if !ok {
		t.Fatal("mob_rat profile missing")
	}
	disguiseProfile, ok := room.bundle.Manifest.Profile("player_knight")
	if !ok {
		t.Fatal("player_knight profile missing")
	}

	state := profile.DefaultState()
	state.ID = "test-hidden-mimic"
	state.Name = "Hidden Mimic"
	state.Kind = shared.EntityKindMimic
	state.RoomID = room.startRoomID()
	state.Position = room.raid.PlayerSpawn
	state.AnimationStartedAt = room.serverTime()

	npc := &serverNPC{
		profile:         profile,
		state:           state,
		home:            state.Position,
		patrolMin:       state.Position.X - 220,
		patrolMax:       state.Position.X + 220,
		direction:       1,
		disguiseProfile: disguiseProfile.ID,
		awakened:        false,
	}
	room.npcs[state.ID] = npc
	return npc
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
