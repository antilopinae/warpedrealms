// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package serverapp

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"warpedrealms/content"
	"warpedrealms/shared"
)

type Peer struct {
	playerID   string
	playerName string
	classID    string
	layoutSent bool
	conn       *websocket.Conn
	send       chan shared.ServerMessage
	room       *RaidRoom
	useProto   bool
}

type RaidRoom struct {
	id          string
	name        string
	maxPlayers  int
	duration    float64
	seed        int64
	bundle      *content.Bundle
	raid        *content.GeneratedRaid
	forceKnight bool

	joinCh    chan joinRequest
	leaveCh   chan leaveRequest
	inputCh   chan inputRequest
	summaryCh chan chan shared.RaidSummary

	players       map[string]*serverPlayer
	peers         map[string]*Peer
	npcs          map[string]*serverNPC
	loot          map[string]*lootPickup
	roomSolids    map[string][]shared.Rect // roomID → its own solid rects
	roomPlatforms map[string][]shared.Rect // roomID → one-way platform rects
	exitRotation  []shared.ExitState
	spawnRotation []shared.Vec2
	exitCursor    int
	spawnCursor   int
	nextLootID    int

	tick          uint64
	createdAt     time.Time
	startedAt     time.Time
	phase         shared.RaidPhase
	rand          *rand.Rand
	lastRiftSpawn float64 // serverTime() when a rift was last spawned
}

type joinRequest struct {
	peer  *Peer
	reply chan error
}

type leaveRequest struct {
	playerID string
	peer     *Peer
}

type inputRequest struct {
	playerID string
	commands []shared.InputCommand
}

type serverPlayer struct {
	class     content.ClassDefinition
	profile   content.AssetProfile
	state     shared.EntityState
	pending   []shared.InputCommand
	lastInput shared.InputCommand

	lastQueuedSeq    uint32
	lastProcessedSeq uint32
	attackCombo      int

	status       shared.PlayerRaidStatus
	carriedLoot  int
	assignedExit shared.ExitState
	cooldowns    map[string]float64
	nextMobHitAt float64
}

type serverNPC struct {
	profile         content.AssetProfile
	state           shared.EntityState
	home            shared.Vec2
	patrolMin       float64
	patrolMax       float64
	direction       float64
	nextAttackAt    float64
	awakened        bool
	disguiseProfile string
	deadAt          float64
}

type lootPickup struct {
	state shared.LootState
}

func NewRaidRoom(id string, name string, bundle *content.Bundle, maxPlayers int, duration float64, seed int64) (*RaidRoom, error) {
	raid, err := bundle.GenerateRaid(seed)
	if err != nil {
		return nil, err
	}
	return newRaidRoomFromRaid(id, name, bundle, maxPlayers, duration, raid, false)
}

// NewRaidRoomProcGen builds a raid via procedural node-based generation.
// seed=0 uses the current time.
func NewRaidRoomProcGen(id string, name string, bundle *content.Bundle, seed int64, maxPlayers int, duration float64) (*RaidRoom, error) {
	raid, err := bundle.GenerateRaidProcGen(seed)
	if err != nil {
		return nil, err
	}
	return newRaidRoomFromRaid(id, name, bundle, maxPlayers, duration, raid, true)
}

func newRaidRoomFromRaid(id string, name string, bundle *content.Bundle, maxPlayers int, duration float64, raid *content.GeneratedRaid, forceKnight bool) (*RaidRoom, error) {
	room := &RaidRoom{
		id:          id,
		name:        name,
		maxPlayers:  maxPlayers,
		duration:    duration,
		seed:        raid.Layout.Seed,
		bundle:      bundle,
		raid:        raid,
		forceKnight: forceKnight,
		joinCh:      make(chan joinRequest, 16),
		leaveCh:     make(chan leaveRequest, 16),
		inputCh:     make(chan inputRequest, 128),
		summaryCh:   make(chan chan shared.RaidSummary, 16),
		players:     make(map[string]*serverPlayer),
		peers:       make(map[string]*Peer),
		npcs:        make(map[string]*serverNPC),
		loot:        make(map[string]*lootPickup),
		createdAt:   time.Now(),
		phase:       shared.RaidPhaseWaiting,
		rand:        rand.New(rand.NewSource(raid.Layout.Seed)),
	}
	room.cacheSolids()
	room.spawnNPCs()
	return room, nil
}

func (r *RaidRoom) Start() {
	go r.loop()
}

func (r *RaidRoom) Join(peer *Peer) error {
	reply := make(chan error, 1)
	r.joinCh <- joinRequest{peer: peer, reply: reply}
	return <-reply
}

func (r *RaidRoom) Leave(playerID string, peer *Peer) {
	r.leaveCh <- leaveRequest{playerID: playerID, peer: peer}
}

func (r *RaidRoom) EnqueueInputs(playerID string, commands []shared.InputCommand) {
	if len(commands) == 0 {
		return
	}
	r.inputCh <- inputRequest{playerID: playerID, commands: commands}
}

func (r *RaidRoom) Summary() shared.RaidSummary {
	reply := make(chan shared.RaidSummary, 1)
	r.summaryCh <- reply
	return <-reply
}

func (r *RaidRoom) loop() {
	ticker := time.NewTicker(time.Second / time.Duration(shared.SimulationTickRate))
	defer ticker.Stop()

	snapshotInterval := int(shared.SimulationTickRate / shared.SnapshotTickRate)
	if snapshotInterval < 1 {
		snapshotInterval = 1
	}

	for {
		select {
		case request := <-r.joinCh:
			request.reply <- r.handleJoin(request.peer)
		case request := <-r.leaveCh:
			r.handleLeave(request)
		case request := <-r.inputCh:
			r.handleInput(request)
		case reply := <-r.summaryCh:
			reply <- r.summarySnapshot()
		case <-ticker.C:
			r.simulateTick()
			if int(r.tick)%snapshotInterval == 0 {
				r.broadcastSnapshots()
			}
		}
	}
}

func (r *RaidRoom) handleJoin(peer *Peer) error {
	if existing := r.peers[peer.playerID]; existing != nil {
		_ = existing.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "replaced"),
			time.Now().Add(250*time.Millisecond),
		)
		close(existing.send)
		_ = existing.conn.Close()
	}

	player, exists := r.players[peer.playerID]
	if !exists {
		if len(r.players) >= r.maxPlayers {
			return fmt.Errorf("raid is full")
		}
		classDef, profile, err := r.resolveJoinLoadout(peer.classID)
		if err != nil {
			return err
		}
		state := profile.DefaultState()
		state.ID = peer.playerID
		state.Name = peer.playerName
		state.Position = r.spawnPosition(profile)
		state.RoomID = r.startRoomID()
		state.AnimationStartedAt = r.serverTime()
		exit := r.assignExitForJoin()
		player = &serverPlayer{
			class:        classDef,
			profile:      profile,
			state:        state,
			status:       shared.PlayerRaidStatusWaiting,
			assignedExit: exit,
			cooldowns:    make(map[string]float64, 8),
		}
		if r.phase == shared.RaidPhaseActive {
			player.status = shared.PlayerRaidStatusActive
		}
		r.players[peer.playerID] = player
	}
	if err := r.ensurePlayerLoadout(player, peer.classID); err != nil {
		return err
	}

	r.peers[peer.playerID] = peer
	peer.room = r
	peer.send <- shared.ServerMessage{
		Type: "welcome",
		Welcome: &shared.WelcomeMessage{
			PlayerID:              peer.playerID,
			PlayerName:            peer.playerName,
			ClassID:               player.state.ClassID,
			RaidID:                r.id,
			RaidName:              r.name,
			ContentVersion:        r.bundle.Manifest.Version,
			ServerTime:            r.serverTime(),
			TickRate:              shared.SimulationTickRate,
			SnapshotRate:          shared.SnapshotTickRate,
			InterpolationBackTime: shared.InterpolationBackTime,
		},
	}
	peer.send <- shared.ServerMessage{
		Type:     "snapshot",
		Snapshot: r.snapshotFor(peer.playerID),
	}
	return nil
}

func (r *RaidRoom) handleLeave(request leaveRequest) {
	current := r.peers[request.playerID]
	if current == nil || current != request.peer {
		return
	}
	delete(r.peers, request.playerID)
	close(current.send)
}

func (r *RaidRoom) handleInput(request inputRequest) {
	player := r.players[request.playerID]
	if player == nil || player.status != shared.PlayerRaidStatusActive {
		return
	}
	for _, command := range request.commands {
		if command.Seq <= player.lastProcessedSeq || command.Seq <= player.lastQueuedSeq {
			continue
		}
		player.pending = append(player.pending, command)
		player.lastQueuedSeq = command.Seq
	}
}

func (r *RaidRoom) resolveJoinLoadout(requestedClassID string) (content.ClassDefinition, content.AssetProfile, error) {
	manifest := r.bundle.Manifest
	candidateIDs := make([]string, 0, len(manifest.Classes)+1)
	if requestedClassID != "" {
		candidateIDs = append(candidateIDs, requestedClassID)
	}
	if defaultClass, ok := manifest.DefaultPlayerClass(); ok && defaultClass.ID != "" {
		duplicate := false
		for _, id := range candidateIDs {
			if id == defaultClass.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			candidateIDs = append(candidateIDs, defaultClass.ID)
		}
	}
	for _, classDef := range manifest.Classes {
		duplicate := false
		for _, id := range candidateIDs {
			if id == classDef.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			candidateIDs = append(candidateIDs, classDef.ID)
		}
	}

	for _, classID := range candidateIDs {
		classDef, ok := manifest.Class(classID)
		if !ok {
			continue
		}
		profile, ok := manifest.Profile(classDef.ProfileID)
		if !ok {
			continue
		}
		if profile.ClassID == "" {
			profile.ClassID = classDef.ID
		}
		return classDef, profile, nil
	}
	return content.ClassDefinition{}, content.AssetProfile{}, fmt.Errorf("player class profile missing")
}

func (r *RaidRoom) ensureAllPlayerLoadouts() {
	for _, player := range r.players {
		_ = r.ensurePlayerLoadout(player, "")
	}
}

func (r *RaidRoom) ensurePlayerLoadout(player *serverPlayer, preferredClassID string) error {
	if player == nil {
		return nil
	}
	if player.cooldowns == nil {
		player.cooldowns = make(map[string]float64, 8)
	}

	var (
		classDef content.ClassDefinition
		profile  content.AssetProfile
		ok       bool
	)

	switch {
	case player.class.ID != "":
		classDef = player.class
	case player.state.ClassID != "":
		classDef, ok = r.bundle.Manifest.Class(player.state.ClassID)
	case player.profile.ClassID != "":
		classDef, ok = r.bundle.Manifest.Class(player.profile.ClassID)
	case player.state.ProfileID != "":
		classDef, ok = r.bundle.Manifest.ClassByProfileID(player.state.ProfileID)
	case player.profile.ID != "":
		classDef, ok = r.bundle.Manifest.ClassByProfileID(player.profile.ID)
	case preferredClassID != "":
		classDef, ok = r.bundle.Manifest.Class(preferredClassID)
	}
	if classDef.ID == "" && ok {
		ok = false
	}
	if !ok && classDef.ID == "" {
		var err error
		classDef, profile, err = r.resolveJoinLoadout(preferredClassID)
		if err != nil {
			return err
		}
	}

	if profile.ID == "" {
		switch {
		case classDef.ProfileID != "":
			profile, ok = r.bundle.Manifest.Profile(classDef.ProfileID)
		case player.profile.ID != "":
			profile, ok = r.bundle.Manifest.Profile(player.profile.ID)
		case player.state.ProfileID != "":
			profile, ok = r.bundle.Manifest.Profile(player.state.ProfileID)
		}
		if !ok {
			var err error
			classDef, profile, err = r.resolveJoinLoadout(preferredClassID)
			if err != nil {
				return err
			}
		}
	}
	if profile.ClassID == "" {
		profile.ClassID = classDef.ID
	}

	player.class = classDef
	player.profile = profile

	defaultState := profile.DefaultState()
	player.state.ClassID = classDef.ID
	player.state.ProfileID = profile.ID
	if player.state.Kind == "" {
		player.state.Kind = defaultState.Kind
	}
	if player.state.Faction == "" {
		player.state.Faction = defaultState.Faction
	}
	if player.state.FamilyID == "" {
		player.state.FamilyID = defaultState.FamilyID
	}
	if player.state.Scale == 0 {
		player.state.Scale = defaultState.Scale
	}
	if player.state.SpriteSize == (shared.Vec2{}) {
		player.state.SpriteSize = defaultState.SpriteSize
	}
	if player.state.SpriteOffset == (shared.Vec2{}) {
		player.state.SpriteOffset = defaultState.SpriteOffset
	}
	if player.state.Collider.W == 0 || player.state.Collider.H == 0 {
		player.state.Collider = defaultState.Collider
	}
	if player.state.Hurtbox.W == 0 || player.state.Hurtbox.H == 0 {
		player.state.Hurtbox = defaultState.Hurtbox
	}
	if player.state.InteractionBox.W == 0 || player.state.InteractionBox.H == 0 {
		player.state.InteractionBox = defaultState.InteractionBox
	}
	if player.state.MaxHP == 0 {
		player.state.MaxHP = defaultState.MaxHP
		if player.state.HP <= 0 {
			player.state.HP = defaultState.MaxHP
		}
	}
	return nil
}

func (r *RaidRoom) simulateTick() {
	r.tick++
	now := r.serverTime()
	r.ensureAllPlayerLoadouts()

	if r.phase == shared.RaidPhaseWaiting && len(r.players) > 0 {
		r.phase = shared.RaidPhaseActive
		r.startedAt = time.Now()
		for _, player := range r.players {
			if player.status == shared.PlayerRaidStatusWaiting {
				player.status = shared.PlayerRaidStatusActive
			}
		}
	}

	if r.phase == shared.RaidPhaseActive && r.timeRemaining() <= 0 {
		for _, player := range r.players {
			if player.status == shared.PlayerRaidStatusActive {
				// r.dropCarriedLoot(player, player.state.Position)
				player.carriedLoot = 0
				player.status = shared.PlayerRaidStatusExpired
			}
		}
		r.phase = shared.RaidPhaseFinished
	}

	for _, player := range r.players {
		r.updateTravel(&player.state, now)
	}
	for _, npc := range r.npcs {
		r.updateTravel(&npc.state, now)
	}

	if r.phase != shared.RaidPhaseActive {
		return
	}

	for _, player := range r.players {
		if player.status != shared.PlayerRaidStatusActive {
			continue
		}
		input := r.nextInput(player)
		r.simulatePlayer(player, input, now)
	}

	for id, npc := range r.npcs {
		r.simulateNPC(npc, now)
		if npc.state.HP <= 0 && npc.deadAt != 0 && now >= npc.deadAt {
			delete(r.npcs, id)
		}
	}

	for _, player := range r.players {
		shared.RefreshAnimation(&player.state, now)
	}
	for _, npc := range r.npcs {
		shared.RefreshAnimation(&npc.state, now)
	}

	if r.shouldFinishRaid() {
		r.phase = shared.RaidPhaseFinished
	}

	// Spawn a new rift every 60 seconds, up to 20 active rifts per room.
	const riftInterval = 60.0
	const maxActiveRifts = 20
	if now-r.lastRiftSpawn >= riftInterval {
		r.trySpawnRift(now)
		r.lastRiftSpawn = now
	}
}

// trySpawnRift picks a random rift zone for a random room and materialises a
// new rift there, checking:
//   - the room already has fewer than maxActiveRifts open rifts
//   - the chosen zone does not overlap an existing rift (min 1 block = 16 px gap)
func (r *RaidRoom) trySpawnRift(now float64) {
	const maxActiveRifts = 20
	const block = 16.0
	const riftW, riftH = 32.0, 48.0

	if r.raid == nil {
		return
	}
	rooms := r.raid.Layout.Rooms
	if len(rooms) == 0 {
		return
	}

	// Pick a random room that has rift zones and isn't already at the cap.
	candidates := make([]int, 0, len(rooms))
	for i, room := range rooms {
		if len(room.RiftZones) == 0 {
			continue
		}
		active := 0
		for _, rf := range room.Rifts {
			if rf.IsOpen() {
				active++
			}
		}
		if active < maxActiveRifts {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return
	}
	ri := candidates[r.rand.Intn(len(candidates))]
	room := &r.raid.Layout.Rooms[ri]

	// Pick a random rift zone in that room.
	zone := room.RiftZones[r.rand.Intn(len(room.RiftZones))]

	// Place the rift centred in the zone.
	cx := zone.X + zone.W*0.5
	px := cx - riftW/2
	py := zone.Y - riftH

	newArea := shared.Rect{X: px, Y: py, W: riftW, H: riftH}

	// Check overlap: no existing open rift within 1 block gap.
	for _, existing := range room.Rifts {
		if !existing.IsOpen() {
			continue
		}
		// Expand existing area by 1 block on each side for the gap check.
		expanded := shared.Rect{
			X: existing.Area.X - block,
			Y: existing.Area.Y - block,
			W: existing.Area.W + block*2,
			H: existing.Area.H + block*2,
		}
		if newArea.X < expanded.X+expanded.W &&
			newArea.X+newArea.W > expanded.X &&
			newArea.Y < expanded.Y+expanded.H &&
			newArea.Y+newArea.H > expanded.Y {
			return // too close to an existing rift
		}
	}

	// Pick a random target room.
	n := len(rooms)
	targetIdx := r.rand.Intn(n - 1)
	if targetIdx >= ri {
		targetIdx++
	}
	targetRoom := rooms[targetIdx].ID

	riftID := fmt.Sprintf("%s-rift-dyn-%d", room.ID, r.tick)
	room.Rifts = append(room.Rifts, shared.RiftState{
		ID:           riftID,
		RoomID:       room.ID,
		TargetRoomID: targetRoom,
		Area:         newArea,
		Arrival:      shared.Vec2{X: cx, Y: py - 50},
		Kind:         shared.RiftKindGreen,
		Capacity:     shared.RiftCapacity(shared.RiftKindGreen),
		UsedCount:    0,
	})
}

func (r *RaidRoom) simulatePlayer(player *serverPlayer, input shared.InputCommand, now float64) {
	if err := r.ensurePlayerLoadout(player, ""); err != nil {
		return
	}
	if player.state.Travel == nil || !player.state.Travel.Active {
		shared.SimulatePlayer(&player.state, input, r.solidsForRoom(player.state.RoomID), r.platformsForRoom(player.state.RoomID))
		// Do NOT call roomIDForPosition here: procgen rooms all share the same
		// origin so position-based room detection doesn't work.  Room changes
		// happen exclusively via explicit JumpLink/portal use (tryUseJumpLink).
	}
	player.lastInput = input
	player.lastProcessedSeq = input.Seq

	if input.UseJumpLink || input.Interact {
		if r.tryUseJumpLink(player, now) {
			return
		}
		if r.tryUseRift(player, now) {
			return
		}
	}
	if input.PrimaryAttack {
		if ability, ok := classSkill(player.class, 0); ok {
			r.useAbility(player, ability, shared.NextAttackAnimation(player.state, player.attackCombo), now)
			player.attackCombo++
		}
	}
	if input.Skill1 {
		if ability, ok := classSkill(player.class, 1); ok {
			r.useAbility(player, ability, shared.AnimationSkill1, now)
		}
	}
	if input.Skill2 {
		if ability, ok := classSkill(player.class, 2); ok {
			r.useAbility(player, ability, shared.AnimationSkill2, now)
		}
	}
	if input.Skill3 {
		if ability, ok := classSkill(player.class, 3); ok {
			r.useAbility(player, ability, shared.AnimationSkill3, now)
		}
	}
	r.processInteraction(player, now)
}

func (r *RaidRoom) simulateNPC(npc *serverNPC, now float64) {
	if npc.state.HP <= 0 {
		if npc.deadAt == 0 {
			shared.TriggerAnimation(&npc.state, shared.AnimationDeath, now)
			npc.deadAt = now + shared.AnimationDuration(npc.state, shared.AnimationDeath)
		}
		return
	}
	if npc.state.Kind == shared.EntityKindMimic && !npc.awakened {
		return
	}
	target := r.closestActivePlayerInRoom(npc.state.RoomID, shared.EntityCenter(npc.state))
	if target == nil {
		r.patrolNPC(npc)
		return
	}

	npcSolids := r.solidsForRoom(npc.state.RoomID)
	switch npc.profile.Moveset {
	case "flyer":
		r.simulateFlyingNPC(npc, target, now)
	case "jumper":
		move := math.Copysign(1, shared.EntityCenter(target.state).X-shared.EntityCenter(npc.state).X)
		jump := npc.state.Grounded && math.Abs(shared.EntityCenter(target.state).X-shared.EntityCenter(npc.state).X) < 220
		shared.SimulateEnemy(&npc.state, move, jump, npcSolids)
	default:
		move := math.Copysign(1, shared.EntityCenter(target.state).X-shared.EntityCenter(npc.state).X)
		if math.Abs(shared.EntityCenter(target.state).X-shared.EntityCenter(npc.state).X) < 40 {
			move = 0
		}
		shared.SimulateEnemy(&npc.state, move, false, npcSolids)
	}
	// NPC room stays as assigned at spawn; NPCs don't use portals.

	if now >= npc.nextAttackAt && r.canNPCAttackPlayer(npc, target) {
		npc.nextAttackAt = now + npcAttackCooldown(npc.profile.Moveset, npc.state.Kind)
		shared.TriggerAnimation(&npc.state, shared.AnimationAttack1, now)
		damage := npcDamageFor(npc)
		r.applyDamageToPlayer(target, damage, npc.state.Position, now)
	}
}

func (r *RaidRoom) patrolNPC(npc *serverNPC) {
	switch npc.profile.Moveset {
	case "flyer":
		npc.state.Position.X += npc.direction * 1.4
		npc.state.Position.Y = npc.home.Y + math.Sin(r.serverTime()*1.8+float64(len(npc.state.ID)))*26
		if npc.state.Position.X < npc.patrolMin {
			npc.direction = 1
		}
		if npc.state.Position.X > npc.patrolMax {
			npc.direction = -1
		}
	default:
		shared.SimulateEnemy(&npc.state, npc.direction, false, r.solidsForRoom(npc.state.RoomID))
		if npc.state.Position.X < npc.patrolMin {
			npc.direction = 1
		}
		if npc.state.Position.X > npc.patrolMax {
			npc.direction = -1
		}
		if math.Abs(npc.state.Velocity.X) < 2 {
			npc.direction *= -1
		}
	}
	// NPC room stays as assigned at spawn; position-based detection is
	// unreliable when rooms share the same origin (0,0).
}

func (r *RaidRoom) simulateFlyingNPC(npc *serverNPC, target *serverPlayer, now float64) {
	targetCenter := shared.EntityCenter(target.state)
	center := shared.EntityCenter(npc.state)
	dir := targetCenter.Sub(center).Normalize()
	if dir.Length() == 0 {
		dir = shared.Vec2{X: npc.direction}
	}
	npc.state.Facing = math.Copysign(1, dir.X)
	npc.state.Position.X += dir.X * 2.8
	npc.state.Position.Y += dir.Y * 1.6
	npc.state.Position.Y += math.Sin(now*2.8) * 0.9
}

func (r *RaidRoom) nextInput(player *serverPlayer) shared.InputCommand {
	if len(player.pending) == 0 {
		input := player.lastInput
		input.PrimaryAttack = false
		input.Skill1 = false
		input.Skill2 = false
		input.Skill3 = false
		input.Interact = false
		input.UseJumpLink = false
		return input
	}
	input := player.pending[0]
	player.pending = player.pending[1:]
	return input
}

func (r *RaidRoom) useAbility(player *serverPlayer, ability content.AbilityDefinition, animation shared.AnimationState, now float64) {
	if ability.ID == "" {
		return
	}
	if now < player.cooldowns[ability.ID] {
		return
	}
	player.cooldowns[ability.ID] = now + ability.Cooldown
	shared.TriggerAnimation(&player.state, animation, now)

	switch ability.ID {
	case "shield_dash":
		player.state.Position.X += player.state.Facing * 180
	case "shadow_step":
		player.state.Position.X += player.state.Facing * 220
	case "blink_leaf":
		player.state.Position.X += player.state.Facing * 200
		player.state.Position.Y -= 80
	}

	damage := abilityDamage(ability.ID)
	r.applyAbilityHitbox(player, ability.ID, animation, damage, now)
}

func (r *RaidRoom) applyAbilityHitbox(player *serverPlayer, abilityID string, animation shared.AnimationState, damage int, now float64) {
	box, ok := player.profile.HitboxFor(animation, 0.18, player.state.Facing, player.state.Position)
	if !ok {
		center := shared.EntityCenter(player.state)
		box = defaultAbilityBox(abilityID, player.state.Facing, center)
	}
	for _, other := range r.players {
		if other == player || other.status != shared.PlayerRaidStatusActive {
			continue
		}
		if other.state.RoomID != player.state.RoomID || !box.Intersects(shared.HurtboxBounds(other.state)) {
			continue
		}
		if !r.canPlayersDamageEachOther(player, other) {
			continue
		}
		r.applyDamageToPlayer(other, damage, player.state.Position, now)
	}
	for _, npc := range r.npcs {
		if npc.state.RoomID != player.state.RoomID {
			continue
		}
		if npc.state.HP <= 0 || !box.Intersects(shared.HurtboxBounds(npc.state)) {
			continue
		}
		r.applyDamageToNPC(npc, damage, player.state.Position, now)
	}
}

func (r *RaidRoom) processInteraction(player *serverPlayer, now float64) {
	center := shared.EntityCenter(player.state)
	if player.assignedExit.ID != "" && player.assignedExit.Area.ContainsPoint(center) {
		player.status = shared.PlayerRaidStatusExtracted
		return
	}
	if r.tryOpenHiddenMimic(player, center, now) {
		return
	}
	for id, drop := range r.loot {
		if drop.state.RoomID != player.state.RoomID {
			continue
		}
		if center.Sub(drop.state.Position).Length() > 84 {
			continue
		}
		player.carriedLoot += drop.state.Value
		delete(r.loot, id)
		return
	}
}

func (r *RaidRoom) tryOpenHiddenMimic(player *serverPlayer, center shared.Vec2, now float64) bool {
	for _, npc := range r.npcs {
		if npc.state.Kind != shared.EntityKindMimic || npc.awakened || npc.state.RoomID != player.state.RoomID {
			continue
		}
		if !shared.InteractionBounds(npc.state).Inflate(18, 18).ContainsPoint(center) {
			continue
		}
		npc.awakened = true
		npc.nextAttackAt = now + 0.45
		npc.state.Facing = math.Copysign(1, center.X-shared.EntityCenter(npc.state).X)
		shared.TriggerAnimation(&npc.state, shared.AnimationEmerge, now)
		return true
	}
	return false
}

func (r *RaidRoom) tryUseJumpLink(player *serverPlayer, now float64) bool {
	if player.state.Travel != nil && player.state.Travel.Active {
		return true
	}
	room, ok := r.raid.Layout.RoomByID(player.state.RoomID)
	if !ok {
		return false
	}
	center := shared.EntityCenter(player.state)
	for _, link := range room.JumpLinks {
		if !link.Area.ContainsPoint(center) {
			continue
		}
		player.state.Travel = &shared.TravelState{
			Active:     true,
			LinkID:     link.ID,
			FromRoomID: room.ID,
			ToRoomID:   link.TargetRoomID,
			Start:      player.state.Position,
			End:        link.Arrival,
			StartedAt:  now,
			EndsAt:     now + 0.78,
		}
		shared.TriggerAnimation(&player.state, shared.AnimationTravel, now)
		return true
	}
	return false
}

// tryUseRift checks whether the player is standing inside an open rift.
// If so, it increments the rift's UsedCount, removes fully-depleted rifts,
// and starts a TravelState to send the player to the rift's target room.
func (r *RaidRoom) tryUseRift(player *serverPlayer, now float64) bool {
	if player.state.Travel != nil && player.state.Travel.Active {
		return true
	}
	center := shared.EntityCenter(player.state)
	for ri := range r.raid.Layout.Rooms {
		if r.raid.Layout.Rooms[ri].ID != player.state.RoomID {
			continue
		}
		rifts := r.raid.Layout.Rooms[ri].Rifts
		for i := range rifts {
			if !rifts[i].IsOpen() {
				continue
			}
			if !rifts[i].Area.ContainsPoint(center) {
				continue
			}
			// Found a usable rift.
			r.raid.Layout.Rooms[ri].Rifts[i].UsedCount++
			rift := r.raid.Layout.Rooms[ri].Rifts[i]
			player.state.Travel = &shared.TravelState{
				Active:     true,
				LinkID:     rift.ID,
				FromRoomID: rift.RoomID,
				ToRoomID:   rift.TargetRoomID,
				Start:      player.state.Position,
				End:        rift.Arrival,
				StartedAt:  now,
				EndsAt:     now + 0.78,
			}
			shared.TriggerAnimation(&player.state, shared.AnimationTravel, now)
			return true
		}
		break
	}
	return false
}

func (r *RaidRoom) updateTravel(state *shared.EntityState, now float64) {
	if state.Travel == nil || !state.Travel.Active {
		return
	}
	travel := state.Travel
	duration := travel.EndsAt - travel.StartedAt
	if duration <= 0 {
		duration = 0.001
	}
	alpha := (now - travel.StartedAt) / duration
	if alpha >= 1 {
		state.Position = travel.End
		state.RoomID = travel.ToRoomID
		state.Travel = nil
		return
	}
	if alpha < 0 {
		alpha = 0
	}
	position := shared.LerpVec(travel.Start, travel.End, alpha)
	position.Y -= math.Sin(alpha*math.Pi) * 220
	state.Position = position
}

func (r *RaidRoom) applyDamageToPlayer(player *serverPlayer, damage int, _ shared.Vec2, now float64) {
	if player.status != shared.PlayerRaidStatusActive {
		return
	}
	player.state.HP -= damage
	if player.state.HP <= 0 {
		player.state.HP = 0
		switch r.deathPenaltyForRoom(player.state.RoomID) {
		case shared.DeathPenaltyRespawnKeepLoot:
			r.respawnPlayer(player, r.pickGreenRespawnRoomID(), now)
		case shared.DeathPenaltyRespawnHalfLoot:
			lostLoot := player.carriedLoot - player.carriedLoot/2
			r.dropLootValue(player.state.RoomID, lostLoot, player.state.Position)
			player.carriedLoot /= 2
			r.respawnPlayer(player, r.startRoomID(), now)
		default:
			player.status = shared.PlayerRaidStatusEliminated
			r.dropCarriedLoot(player, player.state.Position)
			player.state.Travel = nil
			player.state.Velocity = shared.Vec2{}
			shared.TriggerAnimation(&player.state, shared.AnimationDeath, now)
		}
		return
	}
	shared.TriggerAnimation(&player.state, shared.AnimationHit, now)
}

func (r *RaidRoom) applyDamageToNPC(npc *serverNPC, damage int, _ shared.Vec2, now float64) {
	if npc.state.HP <= 0 {
		return
	}
	if npc.state.Kind == shared.EntityKindMimic && !npc.awakened {
		return
	}
	npc.state.HP -= damage
	if npc.state.HP <= 0 {
		npc.state.HP = 0
		shared.TriggerAnimation(&npc.state, shared.AnimationDeath, now)
		npc.deadAt = now + shared.AnimationDuration(npc.state, shared.AnimationDeath)
		return
	}
	shared.TriggerAnimation(&npc.state, shared.AnimationHit, now)
}

func (r *RaidRoom) canPlayersDamageEachOther(attacker *serverPlayer, target *serverPlayer) bool {
	if attacker.state.RoomID != target.state.RoomID {
		return false
	}
	room, ok := r.raid.Layout.RoomByID(attacker.state.RoomID)
	if !ok {
		return false
	}
	if len(room.PvPZones) == 0 {
		return false
	}
	attackerCenter := shared.EntityCenter(attacker.state)
	targetCenter := shared.EntityCenter(target.state)
	for _, zone := range room.PvPZones {
		if zone.ContainsPoint(attackerCenter) && zone.ContainsPoint(targetCenter) {
			return true
		}
	}
	return false
}

func (r *RaidRoom) canNPCAttackPlayer(npc *serverNPC, player *serverPlayer) bool {
	if npc.state.RoomID != player.state.RoomID {
		return false
	}
	return shared.HurtboxBounds(npc.state).Inflate(42, 24).Intersects(shared.HurtboxBounds(player.state))
}

func (r *RaidRoom) shouldFinishRaid() bool {
	if len(r.players) == 0 {
		return false
	}
	activeCount := 0
	for _, player := range r.players {
		if player.status == shared.PlayerRaidStatusActive {
			activeCount++
		}
	}
	return activeCount == 0
}

func (r *RaidRoom) broadcastSnapshots() {
	for playerID, peer := range r.peers {
		message := shared.ServerMessage{
			Type:     "snapshot",
			Snapshot: r.snapshotFor(playerID),
		}
		select {
		case peer.send <- message:
		default:
			select {
			case <-peer.send:
			default:
			}
			peer.send <- message
		}
	}
}

func (r *RaidRoom) snapshotFor(playerID string) *shared.SnapshotMessage {
	r.ensureAllPlayerLoadouts()
	peer := r.peers[playerID]
	player := r.players[playerID]
	ackSeq := uint32(0)
	if player != nil {
		ackSeq = player.lastProcessedSeq
	}

	layout := (*shared.RaidLayoutState)(nil)
	if peer != nil && !peer.layoutSent {
		layout = &r.raid.Layout
		peer.layoutSent = true
	}

	return &shared.SnapshotMessage{
		ServerTime:       r.serverTime(),
		Tick:             r.tick,
		LocalPlayerID:    playerID,
		LastProcessedSeq: ackSeq,
		Layout:           layout,
		Entities:         r.entitiesFor(playerID),
		Loot:             r.lootStates(),
		Raid:             r.raidStateFor(playerID),
	}
}

func (r *RaidRoom) entitiesFor(_ string) []shared.EntityState {
	entities := make([]shared.EntityState, 0, len(r.players)+len(r.npcs))
	for _, player := range r.players {
		entities = append(entities, player.state.Clone())
	}
	for _, npc := range r.npcs {
		state := npc.state.Clone()
		if npc.state.Kind == shared.EntityKindMimic && !npc.awakened && npc.disguiseProfile != "" {
			state = concealedMimicState(state, npc.disguiseProfile)
		}
		entities = append(entities, state)
	}
	sort.Slice(entities, func(i int, j int) bool {
		return strings.Compare(entities[i].ID, entities[j].ID) < 0
	})
	return entities
}

func concealedMimicState(state shared.EntityState, disguiseProfile string) shared.EntityState {
	state.ID = "concealed-" + state.ID
	state.Name = ""
	state.Kind = shared.EntityKindProp
	state.Faction = shared.FactionNeutral
	state.ProfileID = disguiseProfile
	state.FamilyID = "concealed_container"
	state.HP = 0
	state.MaxHP = 0
	state.Animation = shared.AnimationIdle
	state.AnimationStartedAt = 0
	state.Travel = nil
	return state
}

func (r *RaidRoom) lootStates() []shared.LootState {
	items := make([]shared.LootState, 0, len(r.loot))
	for _, loot := range r.loot {
		items = append(items, loot.state)
	}
	sort.Slice(items, func(i int, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (r *RaidRoom) raidStateFor(localPlayerID string) *shared.RaidState {
	players := make([]shared.PlayerRaidState, 0, len(r.players))
	localStatus := shared.PlayerRaidStatusWaiting
	for _, player := range r.players {
		state := shared.PlayerRaidState{
			PlayerID:        player.state.ID,
			Name:            player.state.Name,
			Status:          player.status,
			CarriedLoot:     player.carriedLoot,
			AssignedExitID:  player.assignedExit.ID,
			AssignedExitTag: player.assignedExit.Label,
			CurrentRoomID:   player.state.RoomID,
			HP:              player.state.HP,
			MaxHP:           player.state.MaxHP,
			Cooldowns:       r.cooldownsFor(player),
		}
		if player.state.ID == localPlayerID {
			localStatus = player.status
		}
		players = append(players, state)
	}
	sort.Slice(players, func(i int, j int) bool { return players[i].PlayerID < players[j].PlayerID })
	return &shared.RaidState{
		RaidID:        r.id,
		Name:          r.name,
		Phase:         r.phase,
		TimeRemaining: r.timeRemaining(),
		Duration:      r.duration,
		LocalStatus:   localStatus,
		Seed:          r.seed,
		Players:       players,
	}
}

func (r *RaidRoom) cooldownsFor(player *serverPlayer) []shared.AbilityCooldown {
	if err := r.ensurePlayerLoadout(player, ""); err != nil {
		return nil
	}
	cooldowns := make([]shared.AbilityCooldown, 0, len(player.class.Skills))
	now := r.serverTime()
	for _, ability := range player.class.Skills {
		remaining := player.cooldowns[ability.ID] - now
		if remaining < 0 {
			remaining = 0
		}
		cooldowns = append(cooldowns, shared.AbilityCooldown{
			ID:        ability.ID,
			Name:      ability.Name,
			Remaining: remaining,
			Duration:  ability.Cooldown,
		})
	}
	return cooldowns
}

func (r *RaidRoom) summarySnapshot() shared.RaidSummary {
	return shared.RaidSummary{
		ID:             r.id,
		Name:           r.name,
		Phase:          r.phase,
		CurrentPlayers: len(r.players),
		MaxPlayers:     r.maxPlayers,
		TimeRemaining:  r.timeRemaining(),
		Duration:       r.duration,
		Seed:           r.seed,
	}
}

func (r *RaidRoom) timeRemaining() float64 {
	if r.phase == shared.RaidPhaseWaiting || r.startedAt.IsZero() {
		return r.duration
	}
	remaining := r.duration - time.Since(r.startedAt).Seconds()
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (r *RaidRoom) serverTime() float64 {
	return time.Since(r.createdAt).Seconds()
}

func (r *RaidRoom) cacheSolids() {
	r.exitRotation = nil
	r.spawnRotation = nil
	r.roomSolids = make(map[string][]shared.Rect, len(r.raid.Layout.Rooms))
	r.roomPlatforms = make(map[string][]shared.Rect, len(r.raid.Layout.Rooms))
	for _, room := range r.raid.Layout.Rooms {
		// Keep solids per-room so physics only runs against the room the entity
		// is currently in.  Merging all rooms' solids into one list causes
		// invisible-wall collisions when rooms share the same origin (0,0).
		r.roomSolids[room.ID] = room.Solids
		r.roomPlatforms[room.ID] = room.Platforms
		r.exitRotation = append(r.exitRotation, room.Exits...)
	}
	sort.Slice(r.exitRotation, func(i int, j int) bool { return r.exitRotation[i].ID < r.exitRotation[j].ID })
	if len(r.exitRotation) > 1 {
		r.rand.Shuffle(len(r.exitRotation), func(i int, j int) {
			r.exitRotation[i], r.exitRotation[j] = r.exitRotation[j], r.exitRotation[i]
		})
	}

	r.spawnRotation = append(r.spawnRotation, r.raid.PlayerSpawns...)
	if len(r.spawnRotation) == 0 {
		r.spawnRotation = append(r.spawnRotation, r.raid.PlayerSpawn)
	}
	if len(r.spawnRotation) > 1 {
		sort.Slice(r.spawnRotation, func(i int, j int) bool {
			if r.spawnRotation[i].X == r.spawnRotation[j].X {
				return r.spawnRotation[i].Y < r.spawnRotation[j].Y
			}
			return r.spawnRotation[i].X < r.spawnRotation[j].X
		})
		r.rand.Shuffle(len(r.spawnRotation), func(i int, j int) {
			r.spawnRotation[i], r.spawnRotation[j] = r.spawnRotation[j], r.spawnRotation[i]
		})
	}
}

// solidsForRoom returns the collision rects for a specific room.
// Falls back to an empty slice if the room isn't found (prevents crashing
// when an entity briefly has a stale/unknown roomID).
func (r *RaidRoom) solidsForRoom(roomID string) []shared.Rect {
	if solids, ok := r.roomSolids[roomID]; ok {
		return solids
	}
	// Fallback: use first room's solids so the player doesn't fall through.
	if len(r.raid.Layout.Rooms) > 0 {
		if solids, ok := r.roomSolids[r.raid.Layout.Rooms[0].ID]; ok {
			return solids
		}
	}
	return nil
}

// platformsForRoom returns the one-way platform rects for a specific room.
func (r *RaidRoom) platformsForRoom(roomID string) []shared.Rect {
	if platforms, ok := r.roomPlatforms[roomID]; ok {
		return platforms
	}
	if len(r.raid.Layout.Rooms) > 0 {
		if platforms, ok := r.roomPlatforms[r.raid.Layout.Rooms[0].ID]; ok {
			return platforms
		}
	}
	return nil
}

// startRoomID returns the ID of the first room — used for initial spawn.
func (r *RaidRoom) startRoomID() string {
	if len(r.raid.Layout.Rooms) > 0 {
		return r.raid.Layout.Rooms[0].ID
	}
	return ""
}

func (r *RaidRoom) spawnNPCs() {
	for _, spawn := range r.raid.NPCs {
		profile, ok := r.bundle.Manifest.Profile(spawn.ProfileID)
		if !ok {
			continue
		}
		state := profile.DefaultState()
		state.ID = spawn.ID
		state.Name = spawn.Name
		state.RoomID = spawn.RoomID
		state.Position = spawn.Position
		state.AnimationStartedAt = r.serverTime()
		npc := &serverNPC{
			profile:         profile,
			state:           state,
			home:            spawn.Position,
			patrolMin:       spawn.Position.X - 220,
			patrolMax:       spawn.Position.X + 220,
			direction:       1,
			disguiseProfile: spawn.DisguiseProfileID,
			awakened:        spawn.DisguiseProfileID == "",
		}
		if r.rand.Intn(2) == 0 {
			npc.direction = -1
		}
		r.npcs[state.ID] = npc
	}
}

func (r *RaidRoom) assignExitForJoin() shared.ExitState {
	if len(r.exitRotation) == 0 {
		return shared.ExitState{}
	}
	exit := r.exitRotation[r.exitCursor%len(r.exitRotation)]
	r.exitCursor++
	return exit
}

func (r *RaidRoom) spawnPosition(_ content.AssetProfile) shared.Vec2 {
	if len(r.spawnRotation) == 0 {
		// Fallback: single spawn with per-player X offset.
		pos := r.raid.PlayerSpawn
		pos.X += float64((r.spawnCursor % max(1, r.maxPlayers)) * 76)
		return pos
	}
	pos := r.spawnRotation[r.spawnCursor%len(r.spawnRotation)]
	r.spawnCursor++
	return pos
}

func (r *RaidRoom) deathPenaltyForRoom(roomID string) shared.DeathPenalty {
	room, ok := r.raid.Layout.RoomByID(roomID)
	if !ok {
		return shared.DeathPenaltyEliminateFullLoot
	}
	return room.EffectiveDeathPenalty(len(r.raid.Layout.Rooms))
}

func (r *RaidRoom) pickGreenRespawnRoomID() string {
	bestRoomID := r.startRoomID()
	bestCount := int(^uint(0) >> 1)
	totalRooms := len(r.raid.Layout.Rooms)
	for _, room := range r.raid.Layout.Rooms {
		if room.EffectiveRingZone(totalRooms) != shared.RingZoneGreen {
			continue
		}
		count := r.activePlayersInRoom(room.ID)
		if count < bestCount {
			bestCount = count
			bestRoomID = room.ID
		}
	}
	return bestRoomID
}

func (r *RaidRoom) activePlayersInRoom(roomID string) int {
	count := 0
	for _, other := range r.players {
		if other.status == shared.PlayerRaidStatusActive && other.state.RoomID == roomID {
			count++
		}
	}
	return count
}

func (r *RaidRoom) pickRespawnPosition(roomID string, excludePlayerID string) shared.Vec2 {
	candidates := r.spawnRotation
	if len(candidates) == 0 {
		candidates = append(candidates, r.raid.PlayerSpawn)
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	best := candidates[0]
	bestMinDist := -1.0
	for _, candidate := range candidates {
		minDist := math.MaxFloat64
		hasOthers := false
		for _, other := range r.players {
			if other.state.ID == excludePlayerID || other.status != shared.PlayerRaidStatusActive || other.state.RoomID != roomID {
				continue
			}
			hasOthers = true
			dist := shared.EntityCenter(other.state).Sub(candidate).Length()
			if dist < minDist {
				minDist = dist
			}
		}
		if !hasOthers {
			minDist = math.MaxFloat64
		}
		if minDist > bestMinDist {
			bestMinDist = minDist
			best = candidate
		}
	}
	return best
}

func (r *RaidRoom) respawnPlayer(player *serverPlayer, roomID string, now float64) {
	if roomID == "" {
		roomID = r.startRoomID()
	}
	player.state.RoomID = roomID
	player.state.Position = r.pickRespawnPosition(roomID, player.state.ID)
	player.state.Velocity = shared.Vec2{}
	player.state.Travel = nil
	player.state.HP = player.state.MaxHP
	player.attackCombo = 0
	shared.TriggerAnimation(&player.state, shared.AnimationIdle, now)
}

func (r *RaidRoom) dropLootValue(roomID string, value int, position shared.Vec2) {
	if value <= 0 {
		return
	}
	r.nextLootID++
	id := fmt.Sprintf("loot-drop-%04d", r.nextLootID)
	r.loot[id] = &lootPickup{
		state: shared.LootState{
			ID:       id,
			RoomID:   roomID,
			Position: position,
			Value:    value,
		},
	}
}

func (r *RaidRoom) dropCarriedLoot(player *serverPlayer, position shared.Vec2) {
	if player == nil || player.carriedLoot <= 0 {
		return
	}
	r.dropLootValue(player.state.RoomID, player.carriedLoot, position)
	player.carriedLoot = 0
}

func (r *RaidRoom) closestActivePlayerInRoom(roomID string, position shared.Vec2) *serverPlayer {
	var (
		best     *serverPlayer
		bestDist = math.MaxFloat64
	)
	for _, player := range r.players {
		if player.status != shared.PlayerRaidStatusActive || player.state.RoomID != roomID {
			continue
		}
		dist := shared.EntityCenter(player.state).Sub(position).Length()
		if dist < bestDist {
			best = player
			bestDist = dist
		}
	}
	return best
}

func classSkill(class content.ClassDefinition, index int) (content.AbilityDefinition, bool) {
	if index < 0 || index >= len(class.Skills) {
		return content.AbilityDefinition{}, false
	}
	return class.Skills[index], true
}

func abilityDamage(abilityID string) int {
	switch abilityID {
	case "basic_slash":
		return 24
	case "shield_dash":
		return 38
	case "guard_counter":
		return 30
	case "banner_slam":
		return 48
	}

	return 20
}

func defaultAbilityBox(abilityID string, facing float64, center shared.Vec2) shared.Rect {
	makeBox := func(offsetX float64, offsetY float64, width float64, height float64) shared.Rect {
		x := center.X + offsetX
		if facing < 0 {
			x = center.X - offsetX - width
		}
		return shared.Rect{X: x, Y: center.Y + offsetY, W: width, H: height}
	}

	switch abilityID {
	case "shield_dash":
		return makeBox(12, -12, 136, 72)
	case "guard_counter":
		return makeBox(-24, -28, 120, 96)
	case "banner_slam":
		return makeBox(-42, -42, 164, 132)
	default:
		return makeBox(10, -10, 84, 56)
	}
}

func npcDamageFor(npc *serverNPC) int {
	switch npc.state.Kind {
	case shared.EntityKindBoss:
		return 24
	case shared.EntityKindMimic:
		return 18
	default:
		return 12
	}
}

func npcAttackCooldown(moveset string, kind shared.EntityKind) float64 {
	switch kind {
	case shared.EntityKindBoss:
		return 1.25
	case shared.EntityKindMimic:
		return 0.9
	}
	if moveset == "flyer" {
		return 0.8
	}
	return 0.7
}
