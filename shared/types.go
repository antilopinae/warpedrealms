// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package shared

import "math"

const (
	DefaultAssetManifestPath   = "data/content/assets_manifest.json"
	DefaultRoomsDir            = "data/rooms"
	SimulationTickRate         = 60.0
	SnapshotTickRate           = 20.0
	FixedDeltaSeconds          = 1.0 / SimulationTickRate
	InterpolationBackTime      = 0.10
	ScreenWidth                = 1280
	ScreenHeight               = 720
	DefaultRaidDuration        = 4800.0
	DefaultRaidMaxPlayers      = 4
	DefaultGeneratedRoomWidth  = 16800.0
	DefaultGeneratedRoomHeight = 4800.0
	SessionRingGreenRooms      = 4
	SessionRingRedRooms        = 2
	SessionRingBlackRooms      = 3
	SessionRingThroneRooms     = 1
	SessionRingTotalRooms      = SessionRingGreenRooms + SessionRingRedRooms + SessionRingBlackRooms + SessionRingThroneRooms
	GeneratedRoomCountMin      = SessionRingTotalRooms
	GeneratedRoomCountMax      = SessionRingTotalRooms
)

const (
	PlayerClassKnight         = "knight"
	PlayerClassArcherAssassin = "archer_assassin"
	PlayerClassForestCaster   = "forest_caster"
)

type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (v Vec2) Add(other Vec2) Vec2 {
	return Vec2{X: v.X + other.X, Y: v.Y + other.Y}
}

func (v Vec2) Sub(other Vec2) Vec2 {
	return Vec2{X: v.X - other.X, Y: v.Y - other.Y}
}

func (v Vec2) Mul(scale float64) Vec2 {
	return Vec2{X: v.X * scale, Y: v.Y * scale}
}

func (v Vec2) Div(scale float64) Vec2 {
	if scale == 0 {
		return Vec2{}
	}
	return Vec2{X: v.X / scale, Y: v.Y / scale}
}

func (v Vec2) Length() float64 {
	return math.Hypot(v.X, v.Y)
}

func (v Vec2) Normalize() Vec2 {
	length := v.Length()
	if length == 0 {
		return Vec2{}
	}
	return v.Div(length)
}

func LerpVec(a Vec2, b Vec2, alpha float64) Vec2 {
	return Vec2{
		X: a.X + (b.X-a.X)*alpha,
		Y: a.Y + (b.Y-a.Y)*alpha,
	}
}

type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

func (r Rect) Right() float64 {
	return r.X + r.W
}

func (r Rect) Bottom() float64 {
	return r.Y + r.H
}

func (r Rect) Center() Vec2 {
	return Vec2{X: r.X + r.W*0.5, Y: r.Y + r.H*0.5}
}

func (r Rect) Intersects(other Rect) bool {
	return r.X < other.Right() &&
		r.Right() > other.X &&
		r.Y < other.Bottom() &&
		r.Bottom() > other.Y
}

func (r Rect) ContainsPoint(point Vec2) bool {
	return point.X >= r.X &&
		point.X <= r.Right() &&
		point.Y >= r.Y &&
		point.Y <= r.Bottom()
}

func (r Rect) Translate(delta Vec2) Rect {
	return Rect{
		X: r.X + delta.X,
		Y: r.Y + delta.Y,
		W: r.W,
		H: r.H,
	}
}

func (r Rect) Inflate(x float64, y float64) Rect {
	return Rect{
		X: r.X - x,
		Y: r.Y - y,
		W: r.W + x*2,
		H: r.H + y*2,
	}
}

type EntityKind string

const (
	EntityKindPlayer EntityKind = "player"
	EntityKindMob    EntityKind = "mob"
	EntityKindBoss   EntityKind = "boss"
	EntityKindMimic  EntityKind = "mimic"
	EntityKindProp   EntityKind = "prop"
	EntityKindRat    EntityKind = EntityKindMob
)

type Faction string

const (
	FactionPlayers  Faction = "players"
	FactionMonsters Faction = "monsters"
	FactionNeutral  Faction = "neutral"
)

type RaidPhase string

const (
	RaidPhaseWaiting  RaidPhase = "waiting"
	RaidPhaseActive   RaidPhase = "active"
	RaidPhaseFinished RaidPhase = "finished"
)

type PlayerRaidStatus string

const (
	PlayerRaidStatusWaiting    PlayerRaidStatus = "waiting"
	PlayerRaidStatusActive     PlayerRaidStatus = "active"
	PlayerRaidStatusExtracted  PlayerRaidStatus = "extracted"
	PlayerRaidStatusEliminated PlayerRaidStatus = "eliminated"
	PlayerRaidStatusExpired    PlayerRaidStatus = "expired"
)

type RingZone string

const (
	RingZoneGreen  RingZone = "green"
	RingZoneRed    RingZone = "red"
	RingZoneBlack  RingZone = "black"
	RingZoneThrone RingZone = "throne"
)

type DeathPenalty string

const (
	DeathPenaltyRespawnKeepLoot   DeathPenalty = "respawn_keep_loot"
	DeathPenaltyRespawnHalfLoot   DeathPenalty = "respawn_half_loot"
	DeathPenaltyEliminateFullLoot DeathPenalty = "eliminate_full_loot"
)

func DeathPenaltyForZone(zone RingZone) DeathPenalty {
	switch zone {
	case RingZoneGreen:
		return DeathPenaltyRespawnKeepLoot
	case RingZoneRed:
		return DeathPenaltyRespawnHalfLoot
	default:
		return DeathPenaltyEliminateFullLoot
	}
}

// RingZoneForRoom maps a room index to its risk belt.
// For canonical 10-room raids it is 4 green, 2 red, 3 black, 1 throne.
// For smaller raids we scale bands proportionally and keep the last room throne.
func RingZoneForRoom(index int, total int) RingZone {
	if total <= 0 {
		return RingZoneGreen
	}
	if index < 0 {
		index = 0
	}
	if index >= total {
		index = total - 1
	}
	if total >= SessionRingTotalRooms {
		switch {
		case index < SessionRingGreenRooms:
			return RingZoneGreen
		case index < SessionRingGreenRooms+SessionRingRedRooms:
			return RingZoneRed
		case index >= total-SessionRingThroneRooms:
			return RingZoneThrone
		default:
			return RingZoneBlack
		}
	}

	throneIndex := total - 1
	if index >= throneIndex {
		return RingZoneThrone
	}

	// Split non-throne rooms into 40% green, 20% red, 40% black.
	nonThrone := total - 1
	greenUntil := int(math.Ceil(float64(nonThrone) * 0.4))
	redUntil := int(math.Ceil(float64(nonThrone) * 0.6))
	if greenUntil < 1 {
		greenUntil = 1
	}
	if redUntil <= greenUntil {
		redUntil = greenUntil + 1
	}
	if redUntil > throneIndex {
		redUntil = throneIndex
	}

	switch {
	case index < greenUntil:
		return RingZoneGreen
	case index < redUntil:
		return RingZoneRed
	default:
		return RingZoneBlack
	}
}

type AbilityCooldown struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Remaining float64 `json:"remaining"`
	Duration  float64 `json:"duration"`
}

type TravelState struct {
	Active     bool    `json:"active"`
	LinkID     string  `json:"link_id,omitempty"`
	FromRoomID string  `json:"from_room_id,omitempty"`
	ToRoomID   string  `json:"to_room_id,omitempty"`
	Start      Vec2    `json:"start"`
	End        Vec2    `json:"end"`
	StartedAt  float64 `json:"started_at,omitempty"`
	EndsAt     float64 `json:"ends_at,omitempty"`
}

type EntityState struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Kind               EntityKind     `json:"kind"`
	Faction            Faction        `json:"faction"`
	ClassID            string         `json:"class_id,omitempty"`
	ProfileID          string         `json:"profile_id"`
	FamilyID           string         `json:"family_id,omitempty"`
	RoomID             string         `json:"room_id,omitempty"`
	Position           Vec2           `json:"position"`
	Velocity           Vec2           `json:"velocity"`
	Facing             float64        `json:"facing"`
	Grounded           bool           `json:"grounded"`
	HP                 int            `json:"hp"`
	MaxHP              int            `json:"max_hp"`
	Animation          AnimationState `json:"animation,omitempty"`
	AnimationStartedAt float64        `json:"animation_started_at,omitempty"`
	SpriteSize         Vec2           `json:"sprite_size"`
	SpriteOffset       Vec2           `json:"sprite_offset"`
	Collider           Rect           `json:"collider"`
	Hurtbox            Rect           `json:"hurtbox"`
	InteractionBox     Rect           `json:"interaction_box"`
	Scale              float64        `json:"scale,omitempty"`
	Travel             *TravelState   `json:"travel,omitempty"`
	DashTimer          float64        `json:"dash_timer"`
	DashCooldown       float64        `json:"dash_cooldown"`
}

func (s EntityState) Clone() EntityState {
	clone := s
	if s.Travel != nil {
		copyTravel := *s.Travel
		clone.Travel = &copyTravel
	}
	return clone
}

func EntityBounds(state EntityState) Rect {
	return Rect{
		X: state.Position.X + state.Collider.X,
		Y: state.Position.Y + state.Collider.Y,
		W: state.Collider.W,
		H: state.Collider.H,
	}
}

func HurtboxBounds(state EntityState) Rect {
	box := state.Hurtbox
	if box.W == 0 || box.H == 0 {
		return EntityBounds(state)
	}
	return Rect{
		X: state.Position.X + box.X,
		Y: state.Position.Y + box.Y,
		W: box.W,
		H: box.H,
	}
}

func InteractionBounds(state EntityState) Rect {
	box := state.InteractionBox
	if box.W == 0 || box.H == 0 {
		return EntityBounds(state).Inflate(24, 24)
	}
	return Rect{
		X: state.Position.X + box.X,
		Y: state.Position.Y + box.Y,
		W: box.W,
		H: box.H,
	}
}

func EntityCenter(state EntityState) Vec2 {
	return EntityBounds(state).Center()
}

type LootState struct {
	ID        string `json:"id"`
	ProfileID string `json:"profile_id,omitempty"`
	RoomID    string `json:"room_id,omitempty"`
	Position  Vec2   `json:"position"`
	Value     int    `json:"value"`
}

type ExitState struct {
	ID               string `json:"id"`
	RoomID           string `json:"room_id,omitempty"`
	Label            string `json:"label"`
	Area             Rect   `json:"area"`
	AssignedPlayerID string `json:"assigned_player_id,omitempty"`
}

type PlacedAssetState struct {
	ID         string  `json:"id"`
	ProfileID  string  `json:"profile_id"`
	RoomID     string  `json:"room_id"`
	Position   Vec2    `json:"position"`
	Scale      float64 `json:"scale,omitempty"`
	DrawOffset Vec2    `json:"draw_offset"`
	Layer      string  `json:"layer,omitempty"`
	Alpha      float64 `json:"alpha,omitempty"`
	Bounds     Rect    `json:"bounds"`
}

type JumpLinkState struct {
	ID           string `json:"id"`
	RoomID       string `json:"room_id"`
	TargetRoomID string `json:"target_room_id"`
	Label        string `json:"label"`
	Area         Rect   `json:"area"`
	Arrival      Vec2   `json:"arrival"`
	PreviewRect  Rect   `json:"preview_rect"`
}

type RevealZoneState struct {
	ID           string `json:"id"`
	RoomID       string `json:"room_id"`
	TargetRoomID string `json:"target_room_id"`
	Area         Rect   `json:"area"`
}

type RiftKind string

const (
	RiftKindRed   RiftKind = "red"   // 5 uses
	RiftKindBlue  RiftKind = "blue"  // 2 uses
	RiftKindGreen RiftKind = "green" // 1 use
)

func RiftCapacity(kind RiftKind) int {
	switch kind {
	case RiftKindRed:
		return 5
	case RiftKindBlue:
		return 2
	case RiftKindGreen:
		return 1
	}
	return 1
}

type RiftState struct {
	ID           string   `json:"id"`
	RoomID       string   `json:"room_id"`
	TargetRoomID string   `json:"target_room_id"`
	Area         Rect     `json:"area"`
	Arrival      Vec2     `json:"arrival"`
	Kind         RiftKind `json:"kind"`
	Capacity     int      `json:"capacity"`
	UsedCount    int      `json:"used_count"`
}

func (rs RiftState) IsOpen() bool {
	return rs.UsedCount < rs.Capacity
}

// BossSpawn marks a planned boss encounter at a pixel position.
// Level 1 = mini boss, 2 = boss, 3 = super boss.
// Flying = true means the boss hovers (no floor required).
type BossSpawn struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Level  int     `json:"level"` // 1 mini, 2 boss, 3 super boss
	Flying bool    `json:"flying,omitempty"`
}

type RoomState struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	TemplateID   string       `json:"template_id,omitempty"`
	Biome        string       `json:"biome"`
	RingZone     RingZone     `json:"ring_zone,omitempty"`
	DeathPenalty DeathPenalty `json:"death_penalty,omitempty"`
	IsThrone     bool         `json:"is_throne,omitempty"`
	Index        int          `json:"index"`
	Bounds       Rect         `json:"bounds"`
	BackgroundID string       `json:"background_id"`
	TileStyleID  string       `json:"tile_style_id,omitempty"`
	BelowRoomID  string       `json:"below_room_id,omitempty"`
	AboveRoomID  string       `json:"above_room_id,omitempty"`
	Solids       []Rect       `json:"solids"`
	// Platforms are one-way collision rects: solid from above, passable from below.
	// The player can jump through them upward and land on top when falling.
	Platforms   []Rect             `json:"platforms,omitempty"`
	Backwalls   []Rect             `json:"backwalls,omitempty"`
	Decorations []PlacedAssetState `json:"decorations,omitempty"`
	JumpLinks   []JumpLinkState    `json:"jump_links,omitempty"`
	RevealZones []RevealZoneState  `json:"reveal_zones,omitempty"`
	Rifts       []RiftState        `json:"rifts,omitempty"`
	// RiftZones lists the rectangles where rifts are allowed to materialise.
	// The server picks among these every 60 s; the client renders them as dim zones.
	RiftZones []Rect `json:"rift_zones,omitempty"`
	// PortalZones lists the rectangles occupied by static portals at ground level.
	// Rendered on the map as portal markers even before a JumpLink is assigned.
	PortalZones []Rect      `json:"portal_zones,omitempty"`
	PvPZones    []Rect      `json:"pvp_zones,omitempty"`
	Exits       []ExitState `json:"exits,omitempty"`
	// BossSpawns lists planned boss encounters generated by the procedural generator.
	// Each entry has a pixel position and a difficulty level (1 mini, 2 boss, 3 super boss).
	BossSpawns []BossSpawn `json:"boss_spawns,omitempty"`
}

type RaidLayoutState struct {
	Seed         int64       `json:"seed"`
	Rooms        []RoomState `json:"rooms"`
	PlayerSpawns []Vec2      `json:"player_spawns,omitempty"`
}

func (layout RaidLayoutState) RoomByID(roomID string) (RoomState, bool) {
	for _, room := range layout.Rooms {
		if room.ID == roomID {
			return room, true
		}
	}
	return RoomState{}, false
}

func (room RoomState) EffectiveRingZone(totalRooms int) RingZone {
	if room.RingZone != "" {
		return room.RingZone
	}
	return RingZoneForRoom(room.Index, totalRooms)
}

func (room RoomState) EffectiveDeathPenalty(totalRooms int) DeathPenalty {
	if room.DeathPenalty != "" {
		return room.DeathPenalty
	}
	return DeathPenaltyForZone(room.EffectiveRingZone(totalRooms))
}

type PlayerRaidState struct {
	PlayerID        string            `json:"player_id"`
	Name            string            `json:"name"`
	Status          PlayerRaidStatus  `json:"status"`
	CarriedLoot     int               `json:"carried_loot"`
	AssignedExitID  string            `json:"assigned_exit_id,omitempty"`
	AssignedExitTag string            `json:"assigned_exit_tag,omitempty"`
	CurrentRoomID   string            `json:"current_room_id,omitempty"`
	HP              int               `json:"hp"`
	MaxHP           int               `json:"max_hp"`
	Cooldowns       []AbilityCooldown `json:"cooldowns,omitempty"`
}

type RaidState struct {
	RaidID        string            `json:"raid_id"`
	Name          string            `json:"name"`
	Phase         RaidPhase         `json:"phase"`
	TimeRemaining float64           `json:"time_remaining"`
	Duration      float64           `json:"duration"`
	LocalStatus   PlayerRaidStatus  `json:"local_status"`
	Seed          int64             `json:"seed"`
	Players       []PlayerRaidState `json:"players"`
}

type RaidSummary struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Phase          RaidPhase `json:"phase"`
	CurrentPlayers int       `json:"current_players"`
	MaxPlayers     int       `json:"max_players"`
	TimeRemaining  float64   `json:"time_remaining"`
	Duration       float64   `json:"duration"`
	Seed           int64     `json:"seed"`
}
