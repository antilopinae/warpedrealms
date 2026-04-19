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
	classID    shared.PlayerClass
	conn       *websocket.Conn
	send       chan shared.ServerMessage
	room       *RaidRoom
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

	players      map[string]*serverPlayer
	peers        map[string]*Peer
	npcs         map[string]*serverNPC
	loot         map[string]*lootPickup
	roomSolids   map[string][]shared.Rect // roomID → its own solid rects
	exitRotation []shared.ExitState
	spawnIndex   int
	nextLootID   int

	tick      uint64
	createdAt time.Time
	startedAt time.Time
	phase     shared.RaidPhase
	rand      *rand.Rand
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
	room.spawnInitialLoot()
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
		if r.forceKnight {
			peer.classID = shared.PlayerClassKnight
		}
		classDef, ok := r.bundle.Manifest.Class(peer.classID)
		if !ok {
			classDef = r.bundle.Manifest.Classes[0]
			peer.classID = classDef.ID
		}
		profile, ok := r.bundle.Manifest.Profile(classDef.ProfileID)
		if !ok {
			return fmt.Errorf("class profile %s missing", classDef.ProfileID)
		}
		state := profile.DefaultState()
		state.ID = peer.playerID
		state.Name = peer.playerName
		state.ClassID = classDef.ID
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

func (r *RaidRoom) simulateTick() {
	r.tick++
	now := r.serverTime()

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
				r.dropCarriedLoot(player, player.state.Position)
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
}

func (r *RaidRoom) simulatePlayer(player *serverPlayer, input shared.InputCommand, now float64) {
	if player.state.Travel == nil || !player.state.Travel.Active {
		shared.SimulatePlayer(&player.state, input, r.solidsForRoom(player.state.RoomID))
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
	}
	if input.PrimaryAttack {
		r.useAbility(player, player.class.Skills[0], shared.NextAttackAnimation(player.state, player.attackCombo), now)
		player.attackCombo++
	}
	if input.Skill1 {
		r.useAbility(player, player.class.Skills[1], shared.AnimationSkill1, now)
	}
	if input.Skill2 {
		r.useAbility(player, player.class.Skills[2], shared.AnimationSkill2, now)
	}
	if input.Skill3 {
		r.useAbility(player, player.class.Skills[3], shared.AnimationSkill3, now)
	}
	r.processInteraction(player, now)
}

func (r *RaidRoom) simulateNPC(npc *serverNPC, now float64) {
	if npc.state.HP <= 0 {
		if npc.deadAt == 0 {
			shared.TriggerAnimation(&npc.state, shared.AnimationDeath, now)
			npc.deadAt = now + shared.AnimationDuration(npc.state, shared.AnimationDeath)
			r.spawnLootForNPC(npc)
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

	damage := abilityDamage(player.class.ID, ability.ID)
	r.applyAbilityHitbox(player, ability.ID, animation, damage, now)
}

func (r *RaidRoom) applyAbilityHitbox(player *serverPlayer, abilityID string, animation shared.AnimationState, damage int, now float64) {
	box, ok := player.profile.HitboxFor(animation, 0.18, player.state.Facing, player.state.Position)
	if !ok {
		center := shared.EntityCenter(player.state)
		box = defaultAbilityBox(player.class.ID, abilityID, player.state.Facing, center)
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

func (r *RaidRoom) applyDamageToPlayer(player *serverPlayer, damage int, dropPos shared.Vec2, now float64) {
	if player.status != shared.PlayerRaidStatusActive {
		return
	}
	player.state.HP -= damage
	if player.state.HP <= 0 {
		player.state.HP = 0
		player.status = shared.PlayerRaidStatusEliminated
		r.dropCarriedLoot(player, dropPos)
		player.carriedLoot = 0
		shared.TriggerAnimation(&player.state, shared.AnimationDeath, now)
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
	player := r.players[playerID]
	ackSeq := uint32(0)
	if player != nil {
		ackSeq = player.lastProcessedSeq
	}
	return &shared.SnapshotMessage{
		ServerTime:       r.serverTime(),
		Tick:             r.tick,
		LocalPlayerID:    playerID,
		LastProcessedSeq: ackSeq,
		Layout:           &r.raid.Layout,
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
	state.ClassID = ""
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
			ClassID:         player.state.ClassID,
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
	r.roomSolids = make(map[string][]shared.Rect, len(r.raid.Layout.Rooms))
	for _, room := range r.raid.Layout.Rooms {
		// Keep solids per-room so physics only runs against the room the entity
		// is currently in.  Merging all rooms' solids into one list causes
		// invisible-wall collisions when rooms share the same origin (0,0).
		r.roomSolids[room.ID] = room.Solids
		r.exitRotation = append(r.exitRotation, room.Exits...)
	}
	sort.Slice(r.exitRotation, func(i int, j int) bool { return r.exitRotation[i].ID < r.exitRotation[j].ID })
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

func (r *RaidRoom) spawnInitialLoot() {
	for _, drop := range r.raid.Loot {
		r.loot[drop.ID] = &lootPickup{
			state: shared.LootState{
				ID:        drop.ID,
				Kind:      drop.Kind,
				ProfileID: drop.ProfileID,
				RoomID:    drop.RoomID,
				Position:  drop.Position,
				Value:     drop.Value,
			},
		}
	}
}

func (r *RaidRoom) spawnLootAt(position shared.Vec2, roomID string, value int, kind shared.LootKind, profileID string) {
	r.nextLootID++
	id := fmt.Sprintf("loot-drop-%03d", r.nextLootID)
	r.loot[id] = &lootPickup{
		state: shared.LootState{
			ID:        id,
			Kind:      kind,
			ProfileID: profileID,
			RoomID:    roomID,
			Position:  position,
			Value:     value,
		},
	}
}

func (r *RaidRoom) dropCarriedLoot(player *serverPlayer, position shared.Vec2) {
	if player.carriedLoot <= 0 {
		return
	}
	r.spawnLootAt(position, player.state.RoomID, player.carriedLoot, shared.LootKindBag, "loot_coin")
}

func (r *RaidRoom) spawnLootForNPC(npc *serverNPC) {
	switch npc.state.Kind {
	case shared.EntityKindBoss:
		r.spawnLootAt(npc.state.Position, npc.state.RoomID, 20+r.rand.Intn(10), shared.LootKindRelic, "loot_relic")
	case shared.EntityKindMimic:
		r.spawnLootAt(npc.state.Position, npc.state.RoomID, 10+r.rand.Intn(6), shared.LootKindGem, "loot_gem")
	default:
		profileID := "loot_coin"
		kind := shared.LootKindCoin
		value := 2 + r.rand.Intn(4)
		if r.rand.Intn(3) == 0 {
			profileID = "loot_gem"
			kind = shared.LootKindGem
			value = 5 + r.rand.Intn(4)
		}
		r.spawnLootAt(npc.state.Position, npc.state.RoomID, value, kind, profileID)
	}
}

func (r *RaidRoom) assignExitForJoin() shared.ExitState {
	if len(r.exitRotation) == 0 {
		return shared.ExitState{}
	}
	exit := r.exitRotation[r.spawnIndex%len(r.exitRotation)]
	r.spawnIndex++
	return exit
}

func (r *RaidRoom) spawnPosition(profile content.AssetProfile) shared.Vec2 {
	position := r.raid.PlayerSpawn
	position.X += float64((r.spawnIndex % max(1, r.maxPlayers)) * 76)
	return position
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

func abilityDamage(classID shared.PlayerClass, abilityID string) int {
	switch classID {
	case shared.PlayerClassKnight:
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
	case shared.PlayerClassArcherAssassin:
		switch abilityID {
		case "quick_shot":
			return 18
		case "volley":
			return 32
		case "shadow_step":
			return 26
		case "marked_burst":
			return 40
		}
	case shared.PlayerClassForestCaster:
		switch abilityID {
		case "seed_bolt":
			return 20
		case "root_cage":
			return 30
		case "blink_leaf":
			return 24
		case "wild_bloom":
			return 44
		}
	}
	return 20
}

func defaultAbilityBox(classID shared.PlayerClass, abilityID string, facing float64, center shared.Vec2) shared.Rect {
	makeBox := func(offsetX float64, offsetY float64, width float64, height float64) shared.Rect {
		x := center.X + offsetX
		if facing < 0 {
			x = center.X - offsetX - width
		}
		return shared.Rect{X: x, Y: center.Y + offsetY, W: width, H: height}
	}

	switch classID {
	case shared.PlayerClassKnight:
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
	case shared.PlayerClassArcherAssassin:
		switch abilityID {
		case "volley":
			return makeBox(14, -18, 192, 68)
		case "shadow_step":
			return makeBox(10, -12, 112, 54)
		case "marked_burst":
			return makeBox(16, -20, 236, 72)
		default:
			return makeBox(12, -10, 160, 36)
		}
	case shared.PlayerClassForestCaster:
		switch abilityID {
		case "root_cage":
			return makeBox(6, -24, 132, 92)
		case "blink_leaf":
			return makeBox(-18, -24, 112, 92)
		case "wild_bloom":
			return makeBox(-36, -46, 164, 144)
		default:
			return makeBox(14, -20, 170, 44)
		}
	default:
		return makeBox(12, -12, 84, 56)
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
