package content

// procgen.go — procedural session generator.
//
// Produces N locations per session connected as a graph (spanning tree +
// optional extra edges).  Each location is assembled by sandwich_gen.go
// using HK-style cave hubs (rooms + corridors + vertical ledges).
//
// Default location size: 200 × 150 blocks at 16 px/block = 3 200 × 2 400 px.
//
// Entry point:  bundle.GenerateRaidProcGen(seed int64) → *GeneratedRaid
//               bundle.GenerateRaidProcGenWith(seed int64, cfg ProcGenConfig) → *GeneratedRaid

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	"warpedrealms/shared"
	"warpedrealms/world"
)

// ─── Config ───────────────────────────────────────────────────────────────────

// PlayerPhysicsConfig describes the player size and jump capability used when
// computing platform spacing.  All values are in pixels.
//
// Default player collider (from assets_manifest.json): 24 × 72 px.
// Design size the user declared: 1.5 × 3 blocks = 24 × 48 px.
// Max jump height = JumpSpeed² / (2 × Gravity).
type PlayerPhysicsConfig struct {
	// Collider size in pixels (width × height).  Used to validate clearances.
	ColliderW float64 // default 24 px  (1.5 blocks)
	ColliderH float64 // default 48 px  (3 blocks)

	// Jump physics matching shared/sim.go MovementConfig.
	JumpSpeed float64 // px/s initial upward velocity (default 735)
	Gravity   float64 // px/s² downward acceleration   (default 2050)

	// MaxDropBlocks is the maximum safe drop height in real grid-blocks.
	// Platforms further apart than this should not be the only route downward.
	MaxDropBlocks int // default 15  (= ~240 px, comfortable free-fall)
}

// MaxJumpPx returns the theoretical maximum upward jump height in pixels.
func (p PlayerPhysicsConfig) MaxJumpPx() float64 {
	if p.Gravity <= 0 {
		return 0
	}
	return (p.JumpSpeed * p.JumpSpeed) / (2 * p.Gravity)
}

// RiftConfig controls how many transient rifts are generated per room.
// Rifts are one-way portals with finite capacity; they carry no reveal zone.
type RiftConfig struct {
	RedPerRoom   int // capacity-5 rifts per room (default 1)
	BluePerRoom  int // capacity-2 rifts per room (default 2)
	GreenPerRoom int // capacity-1 rifts per room (default 3)
}

// ProcGenConfig holds all tunable parameters for the procedural generator.
// Zero values fall back to the defaults in DefaultProcGenConfig.
type ProcGenConfig struct {
	// Location dimensions in blocks (pixel size = blocks × 16).
	GridW int // default 200
	GridH int // default 150

	// Session graph.
	NumRooms   int // locations per session (default 10)
	MinPortals int // portals per room, min (default 4)
	MaxPortals int // portals per room, max (default 7)
	// ExtraEdges is the number of shortcuts added on top of the base graph.
	// -1 means random 1-2.
	ExtraEdges int // default 0

	// Platform scatter density added after node assembly: 0.0 = none, 1.0 = max.
	// Applied as a fraction of the grid columns that get an extra stepping stone.
	Density float64 // default 0.5

	// Player physics used to compute socket jump-compatibility thresholds.
	// If zero-value, DefaultPlayerPhysics is used.
	Player PlayerPhysicsConfig

	// Rifts controls the transient inter-room portal density.
	Rifts RiftConfig

	NodesDir string // path to node JSON files (default "gamedata/nodes")

	// Zone controls zone-specific parameters (max boss level, etc.).
	// If empty it defaults to the generator's per-room zone assignment.
	Zone shared.RingZone
}

// DefaultPlayerPhysics matches the declared design size (1.5×4 blocks) and
// the physics constants from shared/sim.go (default class).
var DefaultPlayerPhysics = PlayerPhysicsConfig{
	ColliderW:     24,   // 1.5 blocks × 16 px
	ColliderH:     64,   // 4 blocks × 16 px
	JumpSpeed:     735,  // px/s  (from sim.go default class)
	Gravity:       2050, // px/s²
	MaxDropBlocks: 15,
}

// DefaultProcGenConfig is the baseline configuration used by GenerateRaidProcGen.
var DefaultProcGenConfig = ProcGenConfig{
	GridW:      400,
	GridH:      300,
	NumRooms:   shared.SessionRingTotalRooms,
	MinPortals: 4,
	MaxPortals: 7,
	ExtraEdges: 0,
	Density:    0.5,
	Player:     DefaultPlayerPhysics,
	Rifts: RiftConfig{
		RedPerRoom:   8,
		BluePerRoom:  5,
		GreenPerRoom: 4,
	},
	NodesDir: "gamedata/nodes",
}

// fill replaces zero-value fields with defaults.
func (c ProcGenConfig) fill() ProcGenConfig {
	d := DefaultProcGenConfig
	if c.GridW <= 0 {
		c.GridW = d.GridW
	}
	if c.GridH <= 0 {
		c.GridH = d.GridH
	}
	if c.NumRooms <= 0 {
		c.NumRooms = d.NumRooms
	}
	if c.MinPortals <= 0 {
		c.MinPortals = d.MinPortals
	}
	if c.MaxPortals <= 0 {
		c.MaxPortals = d.MaxPortals
	}
	if c.ExtraEdges == 0 {
		c.ExtraEdges = d.ExtraEdges
	}
	if c.Density == 0 {
		c.Density = d.Density
	}
	if c.Player.JumpSpeed == 0 {
		c.Player = d.Player
	}
	if c.Rifts.RedPerRoom == 0 && c.Rifts.BluePerRoom == 0 && c.Rifts.GreenPerRoom == 0 {
		c.Rifts = d.Rifts
	}
	if c.NodesDir == "" {
		c.NodesDir = d.NodesDir
	}
	return c
}

// nodeSpaceJumpUp returns the max upward jump in node-space units,
// derived from player physics and the Y-scale factor (gridH / naNodeH).
// Used by the assembler for socket compatibility.
func (c ProcGenConfig) nodeSpaceJumpUp(naNodeH int) int {
	// px per node-unit = (gridH / naNodeH) * blockPx
	pxPerUnit := float64(c.GridH) / float64(naNodeH) * 16.0
	maxJump := c.Player.MaxJumpPx()
	units := int(maxJump / pxPerUnit)
	if units < 1 {
		units = 1
	}
	return units
}

// nodeSpaceDropDown returns the max safe drop in node-space units.
func (c ProcGenConfig) nodeSpaceDropDown(naNodeH int) int {
	pxPerUnit := float64(c.GridH) / float64(naNodeH) * 16.0
	maxDrop := float64(c.Player.MaxDropBlocks) * 16.0
	units := int(maxDrop / pxPerUnit)
	if units < 1 {
		units = 1
	}
	return units
}

// ─── Biome ────────────────────────────────────────────────────────────────────

var pgBiomeNames = [3]string{"ruins", "crystal", "forest"}

// ─── Graph structures ─────────────────────────────────────────────────────────

type pgPortal struct {
	area    shared.Rect
	arrival shared.Vec2
	label   string
}

type pgLocation struct {
	biome       string
	portals     []pgPortal
	portalZones []shared.Rect // pixel-space rects of each portal (for map rendering)
	spawn       shared.Vec2   // pixel coords of player spawn
	spawns      []shared.Vec2
	backwalls   []shared.Rect
	platforms   []shared.Rect // one-way platform rects (pixel coords)
	bossSpawns  []shared.BossSpawn
	riftZones   []shared.Rect // all candidate zones (sky + underground) where rifts may spawn
	splitY      int           // block row separating sky (y<splitY) from underground (y>=splitY)
	level       world.LDtkWriteLevel
}

type pgEdge struct {
	a, b         int
	portA, portB int
}

type pgGraph struct {
	cfg    ProcGenConfig
	biomes []string
	zones  []shared.RingZone
	locs   []*pgLocation
	edges  []pgEdge
}

// ─── Public entry points ──────────────────────────────────────────────────────

// GenerateRaidProcGen creates a full procedural session with default config.
func (b *Bundle) GenerateRaidProcGen(seed int64) (*GeneratedRaid, error) {
	return b.GenerateRaidProcGenWith(seed, DefaultProcGenConfig)
}

// GenerateRaidProcGenWith creates a full procedural session with custom config.
//  1. Ensures node JSON files exist in cfg.NodesDir.
//  2. Loads nodes and assembles NumRooms biome-flavoured locations.
//  3. Builds a graph connecting them via portals.
//  4. Writes a LDtk session file to gamedata/sessions/.
//  5. Loads it back and builds a *GeneratedRaid.
func (b *Bundle) GenerateRaidProcGenWith(seed int64, cfg ProcGenConfig) (*GeneratedRaid, error) {
	cfg = cfg.fill()

	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	graph := pgBuildGraph(rng, cfg)

	// Write session LDtk.
	if err := os.MkdirAll("gamedata/sessions", 0o755); err != nil {
		return nil, fmt.Errorf("procgen mkdir sessions: %w", err)
	}
	ldtkPath := fmt.Sprintf("gamedata/sessions/session_%d.ldtk", seed)
	if err := pgWriteSession(graph, ldtkPath); err != nil {
		return nil, fmt.Errorf("procgen write ldtk: %w", err)
	}

	// Load it back.
	maps, err := world.LoadLDtk(ldtkPath)
	if err != nil {
		return nil, fmt.Errorf("procgen load ldtk: %w", err)
	}

	raid, err := b.buildRaidFromMaps(maps)
	if err != nil {
		return nil, err
	}
	raid.Layout.Seed = seed

	// Override player spawns with inlet-based points from the first location.
	if len(graph.locs) > 0 {
		raid.PlayerSpawns = append([]shared.Vec2(nil), graph.locs[0].spawns...)
	}
	if len(raid.PlayerSpawns) > 0 {
		raid.PlayerSpawn = raid.PlayerSpawns[0]
		raid.Layout.PlayerSpawns = raid.PlayerSpawns
	} else if len(graph.locs) > 0 {
		raid.PlayerSpawn = graph.locs[0].spawn
		raid.PlayerSpawns = []shared.Vec2{raid.PlayerSpawn}
		raid.Layout.PlayerSpawns = raid.PlayerSpawns
	}

	// Backwalls, platforms, and boss spawns are generated directly by sandwich_gen
	// and carried in runtime room state.
	backwallsByRoomID := make(map[string][]shared.Rect, len(graph.locs))
	platformsByRoomID := make(map[string][]shared.Rect, len(graph.locs))
	bossSpawnsByRoomID := make(map[string][]shared.BossSpawn, len(graph.locs))
	riftZonesByRoomID := make(map[string][]shared.Rect, len(graph.locs))
	portalZonesByRoomID := make(map[string][]shared.Rect, len(graph.locs))
	for i, loc := range graph.locs {
		roomID := fmt.Sprintf("room-%02d", i+1)
		backwallsByRoomID[roomID] = append([]shared.Rect(nil), loc.backwalls...)
		platformsByRoomID[roomID] = append([]shared.Rect(nil), loc.platforms...)
		bossSpawnsByRoomID[roomID] = append([]shared.BossSpawn(nil), loc.bossSpawns...)
		riftZonesByRoomID[roomID] = append([]shared.Rect(nil), loc.riftZones...)
		portalZonesByRoomID[roomID] = append([]shared.Rect(nil), loc.portalZones...)
	}

	// Annotate rooms with biome and wire the background-room chain.
	// BelowRoomID makes each room display the next room as its background,
	// which is the core visual feature: you see another sublocation behind you.
	n := cfg.NumRooms
	for i := range raid.Layout.Rooms {
		if i < n {
			if backwalls, ok := backwallsByRoomID[raid.Layout.Rooms[i].ID]; ok {
				raid.Layout.Rooms[i].Backwalls = append([]shared.Rect(nil), backwalls...)
			}
			if platforms, ok := platformsByRoomID[raid.Layout.Rooms[i].ID]; ok {
				raid.Layout.Rooms[i].Platforms = append([]shared.Rect(nil), platforms...)
			}
			if bs, ok := bossSpawnsByRoomID[raid.Layout.Rooms[i].ID]; ok {
				raid.Layout.Rooms[i].BossSpawns = append([]shared.BossSpawn(nil), bs...)
			}
			if rz, ok := riftZonesByRoomID[raid.Layout.Rooms[i].ID]; ok {
				raid.Layout.Rooms[i].RiftZones = append([]shared.Rect(nil), rz...)
			}
			if pz, ok := portalZonesByRoomID[raid.Layout.Rooms[i].ID]; ok {
				raid.Layout.Rooms[i].PortalZones = append([]shared.Rect(nil), pz...)
			}
			bio := graph.biomes[i]
			raid.Layout.Rooms[i].Biome = bio
			raid.Layout.Rooms[i].BackgroundID = "cave"
			raid.Layout.Rooms[i].TileStyleID = ""
			if i+1 < n {
				raid.Layout.Rooms[i].BelowRoomID = raid.Layout.Rooms[i+1].ID
				raid.Layout.Rooms[i+1].AboveRoomID = raid.Layout.Rooms[i].ID
			}
		}
	}
	return raid, nil
}

// ─── Graph construction ───────────────────────────────────────────────────────

func pgBuildGraph(rng *rand.Rand, cfg ProcGenConfig) *pgGraph {
	n := cfg.NumRooms
	g := &pgGraph{
		cfg:    cfg,
		biomes: make([]string, n),
		zones:  make([]shared.RingZone, n),
		locs:   make([]*pgLocation, n),
	}

	// Assign room risk belts + biomes first so generation can follow session depth.
	for i := 0; i < n; i++ {
		zone := shared.RingZoneForRoom(i, n)
		g.zones[i] = zone
		g.biomes[i] = pgBiomeForZone(rng, zone)
	}

	// Generate all locations.
	for i := 0; i < n; i++ {
		locRNG := rand.New(rand.NewSource(rng.Int63()))
		id := fmt.Sprintf("room_%02d", i+1)
		locCfg := cfg
		locCfg.Zone = g.zones[i] // pass zone so generator can scale boss difficulty
		sandwich := GenerateSandwichLocation(locRNG, id, locCfg)
		lvl := sandwich.Level

		// Scale portal and rift counts with location area relative to the base
		// 400×300 grid so larger rooms feel proportionally populated.
		baseArea := float64(400 * 300)
		locArea := float64(locCfg.GridW * locCfg.GridH)
		areaScale := locArea / baseArea
		if areaScale < 1 {
			areaScale = 1
		}

		nPorts := int(math.Round(float64(cfg.MinPortals+rng.Intn(cfg.MaxPortals-cfg.MinPortals+1)) * areaScale))
		portals := pgPlacePortalsOnGround(locRNG, sandwich, nPorts)

		// Collect all portal areas for map rendering.
		pzones := make([]shared.Rect, len(portals))
		for pi, p := range portals {
			pzones[pi] = p.area
		}

		spawn := sandwich.PrimarySpawn
		if spawn == (shared.Vec2{}) {
			spawn = pgPickSpawnFromLevel(lvl, cfg)
		}
		spawns := append([]shared.Vec2(nil), sandwich.SpawnPoints...)
		backwalls := pgGridRectsToPixels(sandwich.BackwallRects, lvl.GridSize)
		platforms := pgGridRectsToPixels(sandwich.PlatformRects, lvl.GridSize)
		g.locs[i] = &pgLocation{
			biome:       g.biomes[i],
			portals:     portals,
			portalZones: pzones,
			spawn:       spawn,
			spawns:      spawns,
			backwalls:   backwalls,
			platforms:   platforms,
			bossSpawns:  append([]shared.BossSpawn(nil), sandwich.BossSpawns...),
			riftZones:   append([]shared.Rect(nil), sandwich.RiftZones...),
			splitY:      sandwich.SplitY,
			level:       lvl,
		}
	}

	// Canonical 10-room raid follows ring hierarchy to the center:
	// green(4) -> red(2) -> black(3) -> throne(1).
	if n == shared.SessionRingTotalRooms {
		pgBuildCanonicalRingEdges(g)
	} else {
		// Fallback for custom room counts: connected spanning tree.
		inTree := []int{0}
		pending := make([]int, n-1)
		for i := range pending {
			pending[i] = i + 1
		}
		for len(pending) > 0 {
			ri := rng.Intn(len(pending))
			b := pending[ri]
			pending = append(pending[:ri], pending[ri+1:]...)
			a := inTree[rng.Intn(len(inTree))]
			g.edges = append(g.edges, pgMakeEdge(g, a, b))
			inTree = append(inTree, b)
		}
	}

	// Optional extra edges for loops / shortcuts.
	extras := 0
	switch {
	case cfg.ExtraEdges == -1:
		extras = 1 + rng.Intn(2)
	case cfg.ExtraEdges > 0:
		extras = cfg.ExtraEdges
	}
	for e := 0; e < extras; e++ {
		for try := 0; try < 40; try++ {
			a := rng.Intn(n)
			b := rng.Intn(n)
			if a == b || pgEdgeExists(g.edges, a, b) {
				continue
			}
			pa := pgFreePortal(g.locs[a], g.edges, a)
			pb := pgFreePortal(g.locs[b], g.edges, b)
			if pa < 0 || pb < 0 {
				continue
			}
			g.edges = append(g.edges, pgEdge{a: a, b: b, portA: pa, portB: pb})
			break
		}
	}
	return g
}

func pgBuildCanonicalRingEdges(g *pgGraph) {
	// Green -> Red
	pgAddEdge(g, 0, 4)
	pgAddEdge(g, 1, 4)
	pgAddEdge(g, 2, 5)
	pgAddEdge(g, 3, 5)

	// Red -> Black
	pgAddEdge(g, 4, 6)
	pgAddEdge(g, 4, 7)
	pgAddEdge(g, 5, 7)
	pgAddEdge(g, 5, 8)

	// Black -> Throne
	pgAddEdge(g, 6, 9)
	pgAddEdge(g, 7, 9)
	pgAddEdge(g, 8, 9)
}

func pgAddEdge(g *pgGraph, a, b int) {
	if a < 0 || b < 0 || a >= len(g.locs) || b >= len(g.locs) || a == b {
		return
	}
	if pgEdgeExists(g.edges, a, b) {
		return
	}
	g.edges = append(g.edges, pgMakeEdge(g, a, b))
}

func pgBiomeForZone(rng *rand.Rand, zone shared.RingZone) string {
	switch zone {
	case shared.RingZoneGreen:
		return "forest"
	case shared.RingZoneRed:
		options := []string{"crystal", "ruins"}
		return options[rng.Intn(len(options))]
	case shared.RingZoneBlack:
		options := []string{"ruins", "crystal"}
		return options[rng.Intn(len(options))]
	case shared.RingZoneThrone:
		return "ruins"
	default:
		return pgBiomeNames[rng.Intn(len(pgBiomeNames))]
	}
}

func pgGridRectsToPixels(rects []shared.Rect, gridSize int) []shared.Rect {
	if len(rects) == 0 {
		return nil
	}
	if gridSize <= 0 {
		gridSize = 16
	}
	scale := float64(gridSize)
	out := make([]shared.Rect, 0, len(rects))
	for _, r := range rects {
		out = append(out, shared.Rect{
			X: r.X * scale,
			Y: r.Y * scale,
			W: r.W * scale,
			H: r.H * scale,
		})
	}
	return out
}

func pgMakeEdge(g *pgGraph, a, b int) pgEdge {
	pa := pgFreePortal(g.locs[a], g.edges, a)
	pb := pgFreePortal(g.locs[b], g.edges, b)
	if pa < 0 {
		pa = 0
	}
	if pb < 0 {
		pb = 0
	}
	return pgEdge{a: a, b: b, portA: pa, portB: pb}
}

func pgEdgeExists(edges []pgEdge, a, b int) bool {
	for _, e := range edges {
		if (e.a == a && e.b == b) || (e.a == b && e.b == a) {
			return true
		}
	}
	return false
}

func pgFreePortal(loc *pgLocation, edges []pgEdge, nodeIdx int) int {
	used := map[int]bool{}
	for _, e := range edges {
		if e.a == nodeIdx {
			used[e.portA] = true
		}
		if e.b == nodeIdx {
			used[e.portB] = true
		}
	}
	for i := range loc.portals {
		if !used[i] {
			return i
		}
	}
	return -1
}

// ─── LDtk session writer ──────────────────────────────────────────────────────

func pgWriteSession(g *pgGraph, path string) error {
	levels := make([]world.LDtkWriteLevel, len(g.locs))
	for i, loc := range g.locs {
		levels[i] = loc.level
	}

	// Track which portals have been assigned a RevealZone via a JumpLink edge,
	// so we can add fallback RevealZones for any remaining unconnected portals.
	// revealCovered[locIdx][portalIdx] = true once covered.
	revealCovered := make([]map[int]bool, len(g.locs))
	for i := range revealCovered {
		revealCovered[i] = make(map[int]bool)
	}

	// Wire portal pairs from graph edges.
	for _, edge := range g.edges {
		aRoomID := fmt.Sprintf("room_%02d", edge.a+1)
		bRoomID := fmt.Sprintf("room_%02d", edge.b+1)
		ap := g.locs[edge.a].portals[edge.portA]
		bp := g.locs[edge.b].portals[edge.portB]

		// JumpLink A → B
		levels[edge.a].Entities = append(levels[edge.a].Entities, world.LDtkWriteEntity{
			Identifier: "JumpLink",
			PX:         int(ap.area.X),
			PY:         int(ap.area.Y),
			W:          int(ap.area.W),
			H:          int(ap.area.H),
			Fields: []world.LDtkWriteField{
				{Key: "target", Value: bRoomID},
				{Key: "label", Value: bp.label},
				{Key: "arrival_x", Value: bp.arrival.X},
				{Key: "arrival_y", Value: bp.arrival.Y},
			},
		})
		// RevealZone A (shows room B behind the portal)
		levels[edge.a].Entities = append(levels[edge.a].Entities, pgMakeRevealZone(ap.area, bRoomID))
		revealCovered[edge.a][edge.portA] = true

		// JumpLink B → A
		levels[edge.b].Entities = append(levels[edge.b].Entities, world.LDtkWriteEntity{
			Identifier: "JumpLink",
			PX:         int(bp.area.X),
			PY:         int(bp.area.Y),
			W:          int(bp.area.W),
			H:          int(bp.area.H),
			Fields: []world.LDtkWriteField{
				{Key: "target", Value: aRoomID},
				{Key: "label", Value: ap.label},
				{Key: "arrival_x", Value: ap.arrival.X},
				{Key: "arrival_y", Value: ap.arrival.Y},
			},
		})
		// RevealZone B (shows room A behind the portal)
		levels[edge.b].Entities = append(levels[edge.b].Entities, pgMakeRevealZone(bp.area, aRoomID))
		revealCovered[edge.b][edge.portB] = true
	}

	// Give every portal that didn't get a graph-edge JumpLink its own JumpLink +
	// RevealZone, so all portals are interactive and look identical.
	// Target: pick the room already connected to this room via the first edge,
	// or fall back to any other room.
	for i, loc := range g.locs {
		// Collect rooms that are already linked to room i via graph edges.
		linkedTargets := make([]int, 0, 4)
		for _, e := range g.edges {
			if e.a == i {
				linkedTargets = append(linkedTargets, e.b)
			} else if e.b == i {
				linkedTargets = append(linkedTargets, e.a)
			}
		}
		// Fallback: first room that isn't i.
		fallbackIdx := -1
		for j := range g.locs {
			if j != i {
				fallbackIdx = j
				break
			}
		}

		for pi, p := range loc.portals {
			if revealCovered[i][pi] {
				continue // already covered by a graph-edge JumpLink
			}
			// Pick a target: rotate through linkedTargets so consecutive unconnected
			// portals fan out to different rooms, then fall back.
			targetIdx := fallbackIdx
			if len(linkedTargets) > 0 {
				targetIdx = linkedTargets[pi%len(linkedTargets)]
			}
			if targetIdx < 0 {
				continue
			}
			targetRoomID := fmt.Sprintf("room_%02d", targetIdx+1)
			arrivalX := g.locs[targetIdx].spawn.X
			arrivalY := g.locs[targetIdx].spawn.Y

			// JumpLink — makes the portal interactive (player can enter it).
			levels[i].Entities = append(levels[i].Entities, world.LDtkWriteEntity{
				Identifier: "JumpLink",
				PX:         int(p.area.X),
				PY:         int(p.area.Y),
				W:          int(p.area.W),
				H:          int(p.area.H),
				Fields: []world.LDtkWriteField{
					{Key: "target", Value: targetRoomID},
					{Key: "label", Value: p.label},
					{Key: "arrival_x", Value: arrivalX},
					{Key: "arrival_y", Value: arrivalY},
				},
			})
			// RevealZone — shows the target room in the background as player approaches.
			levels[i].Entities = append(levels[i].Entities, pgMakeRevealZone(p.area, targetRoomID))
		}
	}

	// Scatter rifts across all rooms using the pre-computed rift zones.
	// Distribution: 60% underground, 35% sky platforms, 5% ground level.
	cfg := g.cfg
	n := len(g.locs)

	baseArea := float64(400 * 300)
	for i, loc := range g.locs {
		riftRNG := rand.New(rand.NewSource(int64(i*7919 + 42)))
		locArea := float64(loc.level.GridW * loc.level.GridH)
		areaScale := locArea / baseArea
		if areaScale < 1 {
			areaScale = 1
		}
		red := int(math.Round(float64(cfg.Rifts.RedPerRoom) * areaScale))
		blue := int(math.Round(float64(cfg.Rifts.BluePerRoom) * areaScale))
		green := int(math.Round(float64(cfg.Rifts.GreenPerRoom) * areaScale))

		// Split zones into sky (y < splitY) and underground (y >= splitY).
		splitYPx := float64(loc.splitY * 16)
		var skyZones, undergroundZones []shared.Rect
		for _, z := range loc.riftZones {
			if z.Y+z.H <= splitYPx {
				skyZones = append(skyZones, z)
			} else {
				undergroundZones = append(undergroundZones, z)
			}
		}
		// Ground zones: portal positions (at the sky/underground boundary).
		groundZones := loc.portalZones

		pgScatterRiftsFromZones(&levels[i], skyZones, undergroundZones, groundZones, red, "red", n, i, riftRNG)
		pgScatterRiftsFromZones(&levels[i], skyZones, undergroundZones, groundZones, blue, "blue", n, i, riftRNG)
		pgScatterRiftsFromZones(&levels[i], skyZones, undergroundZones, groundZones, green, "green", n, i, riftRNG)

		// Emit rift zones as LDtk zone overlay entities.
		for _, rz := range loc.riftZones {
			levels[i].Entities = append(levels[i].Entities, world.LDtkWriteEntity{
				Identifier: "RiftZone",
				PX:         int(rz.X),
				PY:         int(rz.Y),
				W:          int(rz.W),
				H:          int(rz.H),
			})
		}

		// Emit portal zones as LDtk zone overlay entities.
		for _, pz := range loc.portalZones {
			levels[i].Entities = append(levels[i].Entities, world.LDtkWriteEntity{
				Identifier: "PortalZone",
				PX:         int(pz.X),
				PY:         int(pz.Y),
				W:          int(pz.W),
				H:          int(pz.H),
			})
		}
	}

	return world.WriteLDtkFile(path, levels)
}

// pgMakeRevealZone creates a RevealZone entity centred on the portal, body
// extending upward from the portal's mid-point ("по середине и чуть выше").
//
// LDtk entity pivot is (0.5, 1):
//
//	PX  = horizontal centre of the entity  (pivot X = 0.5)
//	PY  = bottom edge of the entity        (pivot Y = 1.0)
//
// Portal geometry:
//
//	top    = portalArea.Y
//	bottom = portalArea.Y + portalArea.H
//	centerX = portalArea.X + portalArea.W/2
//	centerY = portalArea.Y + portalArea.H/2
//
// Desired zone:
//   - horizontally centred on the portal     → PX = centerX
//   - bottom of zone at portal's centre Y    → PY = centerY
//   - zone extends upward from portal centre → top = centerY - H
//   - same width and height as the portal
//
// Result: zone sits in the upper half of the portal + slightly above it.
// pgMakeRevealZone creates a RevealZone entity for a portal.
//
// The zone is large (2× portal width, 2.5× portal height), horizontally centred
// on the portal, with its TOP starting slightly above the portal's top edge.
// This lets the reveal effect trigger as the player approaches from any direction.
//
// LDtk px stores top-left pixel position (our loader treats it that way).
func pgMakeRevealZone(portalArea shared.Rect, targetRoomID string) world.LDtkWriteEntity {
	rw := portalArea.W * 2.0                       // 2× portal width
	rh := portalArea.H * 2.5                       // 2.5× portal height
	px := portalArea.X + portalArea.W*0.5 - rw*0.5 // centred on portal
	// Zone bottom = ground level (portal bottom), zone extends upward into sky.
	// portalArea.Y + portalArea.H = splitY * block (ground surface).
	groundY := portalArea.Y + portalArea.H
	py := groundY - rh // top of zone

	return world.LDtkWriteEntity{
		Identifier: "RevealZone",
		PX:         int(px),
		PY:         int(py),
		W:          int(rw),
		H:          int(rh),
		Fields: []world.LDtkWriteField{
			{Key: "target", Value: targetRoomID},
		},
	}
}

// pgScatterRiftsFromZones places count rifts of the given kind.
// Distribution: 60% underground, 35% sky platforms, 5% ground level.
// Rifts sit ON the floor/platform surface (bottom of rift rect = zone bottom).
func pgScatterRiftsFromZones(wl *world.LDtkWriteLevel, skyZones, undergroundZones, groundZones []shared.Rect, count int, kind string, numRooms, selfIdx int, rng *rand.Rand) {
	const block = 16
	const riftW, riftH = 32, 48

	// Compute per-category counts: 60% underground, 35% sky, 5% ground.
	nUnder := int(math.Round(float64(count) * 0.60))
	nSky := int(math.Round(float64(count) * 0.35))
	nGround := count - nUnder - nSky
	if nGround < 0 {
		nGround = 0
	}

	type entry struct {
		zones []shared.Rect
		n     int
	}
	plan := []entry{
		{undergroundZones, nUnder},
		{skyZones, nSky},
		{groundZones, nGround},
	}

	placeRift := func(z shared.Rect) {
		targetIdx := rng.Intn(max(1, numRooms-1))
		if targetIdx >= selfIdx {
			targetIdx++
		}
		targetRoom := fmt.Sprintf("room_%02d", targetIdx+1)

		// Rift sits on the zone floor: bottom of rift = bottom of zone (z.Y + z.H).
		// px: random horizontal position within zone, clamped so rift stays inside.
		maxPX := z.X + z.W - float64(riftW)
		if maxPX < z.X {
			maxPX = z.X
		}
		px := z.X + rng.Float64()*math.Max(1, maxPX-z.X)
		py := z.Y + z.H - float64(riftH) // rift top; bottom aligns with zone floor

		wl.Entities = append(wl.Entities, world.LDtkWriteEntity{
			Identifier: "Rift",
			PX:         int(px),
			PY:         int(py),
			W:          riftW,
			H:          riftH,
			Fields: []world.LDtkWriteField{
				{Key: "target", Value: fmt.Sprintf("room_%02d", targetIdx+1)},
				{Key: "kind", Value: kind},
				{Key: "arrival_x", Value: float64(wl.GridW*block) * 0.5},
				{Key: "arrival_y", Value: py - 50},
			},
		})
		_ = targetRoom // used above via Sprintf
	}

	for _, e := range plan {
		placed := 0
		// Try to draw from the designated zone list; fall back to any zone or surface.
		for placed < e.n {
			if len(e.zones) > 0 {
				placeRift(e.zones[rng.Intn(len(e.zones))])
			} else if len(undergroundZones) > 0 {
				placeRift(undergroundZones[rng.Intn(len(undergroundZones))])
			} else if len(skyZones) > 0 {
				placeRift(skyZones[rng.Intn(len(skyZones))])
			} else {
				// Last resort: surface column.
				col := wl.GridW/6 + rng.Intn(max(1, wl.GridW*2/3))
				surfY := pgFindSurface(wl.SolidCells, col, wl.GridW, wl.GridH)
				z := shared.Rect{
					X: float64(col * block),
					Y: float64((surfY - 2) * block),
					W: float64(block * 2),
					H: float64(block * 2),
				}
				placeRift(z)
			}
			placed++
		}
	}
}

// ─── Portal placement ─────────────────────────────────────────────────────────

// pgPlacePortalsOnGround places portals firmly on the solid ground surface at the
// sky/underground boundary (splitY). Portals require:
//   - Solid ground beneath the full portal width AND a clearance margin on each side.
//   - Clear sky (no solid: no hub platform, no stairchain) in a square zone above.
//
// Portals are placed preferentially just outside inlet shaft openings (where there
// is naturally lots of open flat ground) and fall back to any other valid position.
func pgPlacePortalsOnGround(rng *rand.Rand, sandwich SandwichLocation, count int) []pgPortal {
	const block = 16
	const portalW, portalH = 56, 72
	const portalWB = 4 // portal width in blocks: ceil(56/16) = 4
	const clearB = 5   // minimum clear blocks on each side (= 80 px buffer)

	if count <= 0 {
		return nil
	}

	splitY := sandwich.SplitY
	lvl := sandwich.Level
	gridW := lvl.GridW
	inlets := sandwich.Inlets

	// Build lookup: which columns at splitY have solid ground, and which sky cells
	// are blocked (by hub platforms, stairchains, etc.).
	solidAtSplit := make([]bool, gridW)
	type xy struct{ x, y int }
	skyBlocked := make(map[xy]bool, len(lvl.SolidCells)/2)
	for _, c := range lvl.SolidCells {
		x, y := c[0], c[1]
		if x < 0 || x >= gridW {
			continue
		}
		if y == splitY {
			solidAtSplit[x] = true
		} else if y >= 0 && y < splitY {
			skyBlocked[xy{x, y}] = true
		}
	}

	// skyH: rows above splitY that must be free of solid (portal body height + headroom).
	skyH := portalH/block + 2 // ≈ 6 rows

	// isValidLeft(col): can we place a portal whose left edge starts at col?
	isValidLeft := func(col int) bool {
		lo := col - clearB
		hi := col + portalWB + clearB
		if lo < 0 || hi > gridW {
			return false
		}
		// All ground columns in [lo, hi) must be solid at splitY.
		for x := lo; x < hi; x++ {
			if !solidAtSplit[x] {
				return false
			}
		}
		// All sky cells in [lo, hi) × [splitY-skyH, splitY) must be air.
		for y := max(0, splitY-skyH); y < splitY; y++ {
			for x := lo; x < hi; x++ {
				if skyBlocked[xy{x, y}] {
					return false
				}
			}
		}
		return true
	}

	// Collect all valid left-edge columns.
	valid := make([]bool, gridW)
	for col := 0; col < gridW; col++ {
		valid[col] = isValidLeft(col)
	}

	portals := make([]pgPortal, 0, count)
	// usedRange marks columns that are "taken" (portal + clearance + spacing).
	usedRange := make([]bool, gridW)

	emit := func(col int) bool {
		if !valid[col] {
			return false
		}
		lo := col - clearB
		hi := col + portalWB + clearB
		for x := max(0, lo); x < min(gridW, hi); x++ {
			if usedRange[x] {
				return false
			}
		}
		cx := float64((col + portalWB/2) * block)
		py := float64(splitY*block) - float64(portalH)
		portals = append(portals, pgPortal{
			area:    shared.Rect{X: cx - float64(portalW)/2, Y: py, W: float64(portalW), H: float64(portalH)},
			arrival: shared.Vec2{X: cx, Y: py - 50},
			label:   pgPortalLabel(len(portals)),
		})
		// Widen exclusion zone so portals are nicely spaced apart.
		spacing := portalWB + clearB
		for x := max(0, col-spacing); x < min(gridW, col+portalWB+spacing); x++ {
			usedRange[x] = true
		}
		return true
	}

	// Pass 1: try positions just outside each inlet shaft opening.
	if len(inlets) > 0 {
		order := rng.Perm(len(inlets))
		for _, i := range order {
			if len(portals) >= count {
				break
			}
			inlet := inlets[i]
			half := inlet.Width/2 + 1
			// Candidates: ground just to the left and right of the shaft hole.
			leftEdge := inlet.CenterX - half - portalWB - clearB
			rightEdge := inlet.CenterX + half + clearB
			sides := []int{leftEdge, rightEdge}
			rng.Shuffle(len(sides), func(a, b int) { sides[a], sides[b] = sides[b], sides[a] })
			for _, col := range sides {
				if emit(col) {
					break
				}
				// Search outward a bit if the exact edge is blocked.
				for d := 1; d <= clearB*2; d++ {
					if emit(col-d) || emit(col+d) {
						break
					}
				}
			}
		}
	}

	// Pass 2: fill any remaining slots from valid columns in random order.
	if len(portals) < count {
		perm := rng.Perm(gridW)
		for _, col := range perm {
			if len(portals) >= count {
				break
			}
			emit(col)
		}
	}

	// Если после всех проверок не найдено ни одного места для портала (слишком плотная застройка),
	// принудительно ставим один портал в центре, игнорируя проверки на свободное небо.
	if len(portals) == 0 {
		// Ищем любую колонку, где на уровне splitY есть твердая земля
		fallbackCol := gridW / 2
		foundGround := false
		for d := 0; d < gridW/2; d++ {
			for _, side := range []int{1, -1} {
				checkCol := gridW/2 + d*side
				if checkCol >= 0 && checkCol < gridW && solidAtSplit[checkCol] {
					fallbackCol = checkCol
					foundGround = true
					break
				}
			}
			if foundGround {
				break
			}
		}

		// Создаем портал в найденной колонке (или просто в центре, если земли вообще нет)
		cx := float64((fallbackCol + portalWB/2) * block)
		py := float64(splitY*block) - float64(portalH)
		portals = append(portals, pgPortal{
			area:    shared.Rect{X: cx - float64(portalW)/2, Y: py, W: float64(portalW), H: float64(portalH)},
			arrival: shared.Vec2{X: cx, Y: py - 50},
			label:   "emergency exit",
		})
	}

	return portals
}

// pgFindSurface scans a column near colX and returns the Y block of the
// topmost solid cell that has open air above it.
func pgFindSurface(cells [][2]int, colX, gridW, gridH int) int {
	occupied := map[[2]int]bool{}
	for _, c := range cells {
		occupied[c] = true
	}
	bestY := gridH - 4
	for dx := -5; dx <= 5; dx++ {
		cx := colX + dx
		if cx < 0 || cx >= gridW {
			continue
		}
		for row := 0; row < gridH; row++ {
			if !occupied[[2]int{cx, row}] {
				continue
			}
			if row > 0 && occupied[[2]int{cx, row - 1}] {
				continue
			}
			if row < bestY {
				bestY = row
			}
			break
		}
	}
	return bestY
}

func pgPickSpawnFromLevel(lvl world.LDtkWriteLevel, cfg ProcGenConfig) shared.Vec2 {
	const block = 16
	colX := cfg.GridW / 10
	surfaceY := pgFindSurface(lvl.SolidCells, colX, lvl.GridW, lvl.GridH)
	return shared.Vec2{
		X: float64(colX) * block,
		Y: float64(surfaceY*block) - 110,
	}
}

func pgPortalLabel(index int) string {
	labels := []string{"portal", "shortcut", "secret passage", "exit", "gateway"}
	if index < len(labels) {
		return labels[index]
	}
	return "portal"
}
