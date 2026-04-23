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
		RedPerRoom:   1,
		BluePerRoom:  2,
		GreenPerRoom: 3,
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
	biome     string
	portals   []pgPortal
	spawn     shared.Vec2 // pixel coords of player spawn
	spawns    []shared.Vec2
	backwalls []shared.Rect
	level     world.LDtkWriteLevel
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

	// Backwalls are generated directly by sandwich_gen and carried in runtime
	// room state. They are data-only in this stage (no physics/render usage).
	backwallsByRoomID := make(map[string][]shared.Rect, len(graph.locs))
	for i, loc := range graph.locs {
		roomID := fmt.Sprintf("room-%02d", i+1)
		backwallsByRoomID[roomID] = append([]shared.Rect(nil), loc.backwalls...)
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
		sandwich := GenerateSandwichLocation(locRNG, id, cfg)
		lvl := sandwich.Level
		nPorts := cfg.MinPortals + rng.Intn(cfg.MaxPortals-cfg.MinPortals+1)
		portals := pgPlacePortals(locRNG, lvl, nPorts, cfg)
		spawn := sandwich.PrimarySpawn
		if spawn == (shared.Vec2{}) {
			spawn = pgPickSpawnFromLevel(lvl, cfg)
		}
		spawns := append([]shared.Vec2(nil), sandwich.SpawnPoints...)
		backwalls := pgGridRectsToPixels(sandwich.BackwallRects, lvl.GridSize)
		g.locs[i] = &pgLocation{
			biome:     g.biomes[i],
			portals:   portals,
			spawn:     spawn,
			spawns:    spawns,
			backwalls: backwalls,
			level:     lvl,
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
	}

	// Scatter rifts across all rooms.
	cfg := g.cfg
	n := len(g.locs)
	for i, loc := range g.locs {
		lvl := loc.level
		// Each rift goes to a randomly chosen other room.
		riftRNG := rand.New(rand.NewSource(int64(i*7919 + 42)))

		pgScatterRifts(&levels[i], lvl, cfg.Rifts.RedPerRoom, "red", n, i, riftRNG)
		pgScatterRifts(&levels[i], lvl, cfg.Rifts.BluePerRoom, "blue", n, i, riftRNG)
		pgScatterRifts(&levels[i], lvl, cfg.Rifts.GreenPerRoom, "green", n, i, riftRNG)
	}

	return world.WriteLDtkFile(path, levels)
}

// pgMakeRevealZone creates a RevealZone entity placed just above/around a portal area.
// The reveal zone is wider than the portal itself so the player sees the target
// room's background while approaching.
func pgMakeRevealZone(portalArea shared.Rect, targetRoomID string) world.LDtkWriteEntity {
	const revealPad = 80.0
	rx := portalArea.X - revealPad
	ry := portalArea.Y - revealPad
	rw := portalArea.W + revealPad*2
	rh := portalArea.H + revealPad*2
	return world.LDtkWriteEntity{
		Identifier: "RevealZone",
		PX:         int(rx),
		PY:         int(ry),
		W:          int(rw),
		H:          int(rh),
		Fields: []world.LDtkWriteField{
			{Key: "target", Value: targetRoomID},
		},
	}
}

// pgScatterRifts places count rifts of the given kind inside lvl, targeting
// rooms other than selfIdx.
func pgScatterRifts(wl *world.LDtkWriteLevel, lvl world.LDtkWriteLevel, count int, kind string, numRooms, selfIdx int, rng *rand.Rand) {
	const block = 16
	const riftW, riftH = 32, 48

	for i := 0; i < count; i++ {
		// Pick a random target room that is not this room.
		targetIdx := rng.Intn(numRooms - 1)
		if targetIdx >= selfIdx {
			targetIdx++
		}
		targetRoom := fmt.Sprintf("room_%02d", targetIdx+1)

		// Pick a random column in the middle two-thirds of the level.
		lo := lvl.GridW / 6
		hi := lvl.GridW * 5 / 6
		if hi <= lo {
			hi = lvl.GridW - 2
		}
		col := lo + rng.Intn(hi-lo)
		surfaceY := pgFindSurface(lvl.SolidCells, col, lvl.GridW, lvl.GridH)
		px := float64(col*block) - riftW/2
		py := float64(surfaceY*block) - riftH

		// Arrival in target room — just pick center.
		arrX := float64(lvl.GridW*block) * 0.5
		arrY := float64(surfaceY*block) - 100

		wl.Entities = append(wl.Entities, world.LDtkWriteEntity{
			Identifier: "Rift",
			PX:         int(px),
			PY:         int(py),
			W:          riftW,
			H:          riftH,
			Fields: []world.LDtkWriteField{
				{Key: "target", Value: targetRoom},
				{Key: "kind", Value: kind},
				{Key: "arrival_x", Value: arrX},
				{Key: "arrival_y", Value: arrY},
			},
		})
	}
}

// ─── Portal placement ─────────────────────────────────────────────────────────

func pgPlacePortals(rng *rand.Rand, lvl world.LDtkWriteLevel, count int, cfg ProcGenConfig) []pgPortal {
	const block = 16
	stepX := (lvl.GridW * block) / (count + 1)
	portals := make([]pgPortal, 0, count)

	for i := 0; i < count; i++ {
		cx := float64(stepX*(i+1)) + float64(rng.Intn(stepX/3)-(stepX/6))
		surfaceY := pgFindSurface(lvl.SolidCells, int(cx)/block, lvl.GridW, lvl.GridH)
		py := float64(surfaceY*block) - 72
		if py < 0 {
			py = 8
		}
		portals = append(portals, pgPortal{
			area: shared.Rect{X: cx - 28, Y: py, W: 56, H: 56},
			arrival: shared.Vec2{
				X: cx,
				Y: float64(surfaceY*block) - 120,
			},
			label: pgPortalLabel(i),
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

// pgGenerateSpawnPoints scans the first location's solid cells and returns
// N evenly-spaced spawn points along the top surface of the left portion of
// the level.  N = cfg.NumRooms (one slot per expected player; more than enough
// for typical session sizes).
//
// Each point is placed above the topmost solid cell in a column, at a height
// that clears the player collider (cfg.Player.ColliderH pixels).
func pgGenerateSpawnPoints(lvl world.LDtkWriteLevel, cfg ProcGenConfig) []shared.Vec2 {
	const block = 16

	// Scan the left third of the level for solid surface columns.
	scanEnd := lvl.GridW / 3
	if scanEnd < 20 {
		scanEnd = lvl.GridW
	}

	n := cfg.NumRooms // one spawn per player slot
	if n < 2 {
		n = 2
	}

	// stride between spawn columns
	stride := scanEnd / (n + 1)
	if stride < 4 {
		stride = 4
	}

	spawns := make([]shared.Vec2, 0, n)
	for i := 0; i < n; i++ {
		colX := stride * (i + 1)
		surfaceY := pgFindSurface(lvl.SolidCells, colX, lvl.GridW, lvl.GridH)
		// Place player feet at surfaceY, then lift by collider height + small margin.
		clearancePx := cfg.Player.ColliderH + 8
		py := float64(surfaceY*block) - clearancePx
		if py < 0 {
			py = 8
		}
		spawns = append(spawns, shared.Vec2{
			X: float64(colX * block),
			Y: py,
		})
	}
	return spawns
}

func pgPortalLabel(index int) string {
	labels := []string{"portal", "shortcut", "secret passage", "exit", "gateway"}
	if index < len(labels) {
		return labels[index]
	}
	return "portal"
}

// ─── Utility ──────────────────────────────────────────────────────────────────

func pgClamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
