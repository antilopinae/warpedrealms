package clientapp

import (
	"math"

	"warpedrealms/shared"
)

// Bot navigation constants.
const (
	botWaitDuration   = 1.5  // seconds to stand still at a boss
	botStuckTimeout   = 0.4  // seconds with no X movement → trigger a jump
	botJumpCooldown   = 0.18 // minimum seconds between jumps
	botArrivalRadiusX = 30.0 // horizontal arrival threshold (pixels)
	botArrivalRadiusY = 60.0 // vertical arrival threshold (pixels)

	// A* path following: how close the bot needs to get to a waypoint
	// before advancing to the next one.
	botWaypointRadiusX = float64(astarBlock) * 0.9 // ~14px
	botWaypointRadiusY = float64(astarBlock) * 1.5 // ~24px

	// If the bot hasn't advanced a waypoint in this many seconds, replan.
	botPathStaleTime = 1.8
)

type botPhase int

const (
	botPhaseMoving  botPhase = iota
	botPhaseWaiting          // standing still for botWaitDuration after reaching a boss
)

// LocalBot is a purely client-side simulated bot player.  It uses the same
// SimulatePlayer physics as the local player and navigates to boss spawn points
// in its assigned room in a round-robin order (nearest unvisited first).
//
// Navigation uses A* on a grid built from the room's solid/platform rects,
// with a body model of astarBW × astarBH blocks (3 wide, 5 tall).
type LocalBot struct {
	entity shared.EntityState
	phase  botPhase

	homeRoomID  string
	initialized bool

	visitedBosses map[int]bool
	currentTarget int     // index into room.BossSpawns; -1 = none chosen
	waitUntil     float64 // server-time when waiting phase ends

	// A* pathfinding state.
	grid       *botGrid      // cached grid built from room solids/platforms
	path       []shared.Vec2 // current A* waypoints (world-space feet positions)
	pathIdx    int           // next waypoint to head toward
	pathTarget int           // BossSpawn index the current path is for

	// Fallback direct-navigation state (used when A* finds no path).
	stuckX       float64
	stuckTimer   float64
	jumpCooldown float64

	// A* staleness tracking: replan if no waypoint progress for botPathStaleTime.
	lastPathIdx    int
	pathStaleTimer float64
}

// NewLocalBot creates a bot with the given ID and display name.
func NewLocalBot(id, name string) *LocalBot {
	return &LocalBot{
		entity: shared.EntityState{
			ID:     id,
			Name:   name,
			Kind:   shared.EntityKindPlayer,
			Facing: 1,
			HP:     2000,
			MaxHP:  2000,
		},
		visitedBosses: make(map[int]bool),
		currentTarget: -1,
		pathTarget:    -1,
	}
}

// AssignRoom (re)initialises the bot for a new room.
// template provides collider/sprite/position data from the local player so the
// bot looks, collides, and spawns identically.
func (b *LocalBot) AssignRoom(roomID string, room shared.RoomState, template shared.EntityState, now float64, solids, platforms []shared.Rect) {
	if b.homeRoomID == roomID && b.initialized {
		return
	}
	b.homeRoomID = roomID
	b.initialized = true
	b.phase = botPhaseMoving
	b.visitedBosses = make(map[int]bool)
	b.currentTarget = -1
	b.pathTarget = -1
	b.path = nil
	b.pathIdx = 0
	b.jumpCooldown = 0
	b.stuckTimer = 0
	b.waitUntil = 0
	b.lastPathIdx = 0
	b.pathStaleTimer = 0

	// Mirror visual + physics properties from the local player exactly.
	b.entity.RoomID = roomID
	b.entity.Collider = template.Collider
	b.entity.Hurtbox = template.Hurtbox
	b.entity.SpriteSize = template.SpriteSize
	b.entity.SpriteOffset = template.SpriteOffset
	b.entity.ProfileID = template.ProfileID
	b.entity.Scale = template.Scale
	b.entity.ClassID = template.ClassID
	b.entity.Velocity = shared.Vec2{}
	b.entity.Grounded = false
	b.entity.Travel = nil

	// Spawn location: always use the player's current world-space position.
	// • Same room → bot spawns right where the player stands.
	// • Background room → same world coordinates put the bot at the centre of
	//   the background viewport (both rooms share the same coordinate origin),
	//   so the bot is always visible. Physics then resolves any solid overlap.
	b.entity.Position = template.Position
	b.stuckX = b.entity.Position.X

	// Ensure the bot is always rendered at a visible size even if the template
	// entity hasn't received its scale from the server yet.
	if b.entity.Scale <= 0 {
		b.entity.Scale = 1.0
	}

	// Build the A* grid for this room.
	b.grid = newBotGrid(room, solids, platforms)
}

// pickNearestUnvisited returns the index of the nearest unvisited BossSpawn,
// resetting the visited set when all have been seen.
func (b *LocalBot) pickNearestUnvisited(spawns []shared.BossSpawn) int {
	if len(spawns) == 0 {
		return -1
	}
	allDone := true
	for i := range spawns {
		if !b.visitedBosses[i] {
			allDone = false
			break
		}
	}
	if allDone {
		b.visitedBosses = make(map[int]bool)
	}
	bx, by := b.entity.Position.X, b.entity.Position.Y
	bestIdx := -1
	bestDist := math.MaxFloat64
	for i, s := range spawns {
		if b.visitedBosses[i] {
			continue
		}
		dx := s.X - bx
		dy := s.Y - by
		d := math.Sqrt(dx*dx + dy*dy)
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return bestIdx
}

// Update runs one AI + physics tick for the bot.
// now is the estimated server time in seconds.
func (b *LocalBot) Update(now float64, room shared.RoomState, solids, platforms []shared.Rect) {
	if !b.initialized {
		return
	}
	spawns := room.BossSpawns
	idle := shared.InputCommand{}

	switch b.phase {
	// ── Waiting at a boss ──────────────────────────────────────────────────────
	case botPhaseWaiting:
		shared.SimulatePlayer(&b.entity, idle, solids, platforms)
		if now >= b.waitUntil {
			if b.currentTarget >= 0 {
				b.visitedBosses[b.currentTarget] = true
			}
			b.currentTarget = b.pickNearestUnvisited(spawns)
			b.path = nil // force new A* search
			b.pathIdx = 0
			b.phase = botPhaseMoving
		}

	// ── Moving toward the target boss ─────────────────────────────────────────
	case botPhaseMoving:
		if b.currentTarget < 0 || b.currentTarget >= len(spawns) {
			b.currentTarget = b.pickNearestUnvisited(spawns)
			b.path = nil
			b.pathIdx = 0
		}
		if b.currentTarget < 0 {
			shared.SimulatePlayer(&b.entity, idle, solids, platforms)
			return
		}

		target := spawns[b.currentTarget]

		// Arrival check.
		dx := target.X - b.entity.Position.X
		dy := target.Y - b.entity.Position.Y
		if math.Abs(dx) < botArrivalRadiusX && math.Abs(dy) < botArrivalRadiusY {
			b.phase = botPhaseWaiting
			b.waitUntil = now + botWaitDuration
			b.path = nil
			shared.SimulatePlayer(&b.entity, idle, solids, platforms)
			return
		}

		// Compute A* path if we have no valid one for this target.
		if b.path == nil || b.pathTarget != b.currentTarget {
			if b.grid != nil {
				b.path = b.grid.FindPath(
					b.entity.Position.X, b.entity.Position.Y,
					target.X, target.Y,
				)
			}
			b.pathIdx = 0
			b.pathTarget = b.currentTarget
		}

		// Follow the A* path if available; otherwise fall back to direct nav.
		var moveX float64
		jump := false

		dt := shared.FixedDeltaSeconds
		b.jumpCooldown -= dt

		if b.path != nil && b.pathIdx < len(b.path) {
			moveX, jump = b.followAStarPath()

			// Stale-path detection: if the waypoint index hasn't advanced in
			// botPathStaleTime seconds the bot is stuck — force a new A* search.
			if b.pathIdx > b.lastPathIdx {
				b.lastPathIdx = b.pathIdx
				b.pathStaleTimer = 0
			} else {
				b.pathStaleTimer += dt
				if b.pathStaleTimer >= botPathStaleTime {
					b.path = nil // triggers replan next tick
					b.pathStaleTimer = 0
					b.lastPathIdx = 0
				}
			}
		} else {
			// Fallback: direct navigation with stuck detection.
			moveX, jump = b.directNav(dx, dy, dt)
			b.pathStaleTimer = 0
		}

		if jump {
			b.jumpCooldown = botJumpCooldown
		}
		shared.SimulatePlayer(&b.entity, shared.InputCommand{MoveX: moveX, Jump: jump}, solids, platforms)
	}
}

// followAStarPath moves the bot toward the current waypoint and returns
// the (moveX, jump) inputs for this tick.
func (b *LocalBot) followAStarPath() (moveX float64, jump bool) {
	if b.pathIdx >= len(b.path) {
		return 0, false
	}
	wp := b.path[b.pathIdx]

	dx := wp.X - b.entity.Position.X
	dy := wp.Y - b.entity.Position.Y // positive = waypoint is below (lower on screen)

	// Advance waypoint if close enough.
	if math.Abs(dx) < botWaypointRadiusX && math.Abs(dy) < botWaypointRadiusY {
		b.pathIdx++
		if b.pathIdx >= len(b.path) {
			return 0, false
		}
		wp = b.path[b.pathIdx]
		dx = wp.X - b.entity.Position.X
		dy = wp.Y - b.entity.Position.Y
	}

	// Horizontal movement — always run toward the waypoint.
	if dx > 4 {
		moveX = 1
	} else if dx < -4 {
		moveX = -1
	}

	// Jump when the current waypoint is even slightly above the bot (≥ half a
	// block).  The old threshold of 1.5 blocks missed single-block step-ups.
	if b.entity.Grounded && b.jumpCooldown <= 0 && dy < -float64(astarBlock)*0.5 {
		jump = true
	}

	// Lookahead: if the NEXT waypoint requires climbing and we're nearly aligned
	// horizontally, pre-jump now so we have upward momentum on arrival.
	if !jump && b.entity.Grounded && b.jumpCooldown <= 0 && b.pathIdx+1 < len(b.path) {
		nextWP := b.path[b.pathIdx+1]
		nextDy := nextWP.Y - b.entity.Position.Y
		nextDx := nextWP.X - b.entity.Position.X
		if nextDy < -float64(astarBlock) && math.Abs(nextDx) < float64(astarBlock)*3 {
			jump = true
		}
	}

	return moveX, jump
}

// directNav computes (moveX, jump) using simple direct-to-target navigation
// with stuck detection.  Used as fallback when A* finds no path.
func (b *LocalBot) directNav(dx, dy, dt float64) (moveX float64, jump bool) {
	if dx > 4 {
		moveX = 1
	} else if dx < -4 {
		moveX = -1
	}

	b.stuckTimer += dt
	if b.entity.Grounded && b.jumpCooldown <= 0 {
		if dy < -48 {
			jump = true
		}
		if b.stuckTimer >= botStuckTimeout {
			if math.Abs(b.entity.Position.X-b.stuckX) < 8 {
				jump = true
			}
			b.stuckTimer = 0
			b.stuckX = b.entity.Position.X
		}
	}
	return moveX, jump
}

// Entity returns a copy of the bot's current EntityState for rendering.
func (b *LocalBot) Entity() shared.EntityState {
	return b.entity
}

// IsActive reports whether the bot has been assigned to a room and is ready.
func (b *LocalBot) IsActive() bool {
	return b.initialized && b.homeRoomID != ""
}

// Deactivate removes the bot from its current room so it is no longer rendered
// or updated. Call when another real player enters the bot's room.
func (b *LocalBot) Deactivate() {
	b.homeRoomID = ""
	b.initialized = false
	b.grid = nil
	b.path = nil
}
