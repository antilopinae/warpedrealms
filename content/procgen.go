package content

// procgen.go — node-based procedural session generator.
//
// Produces 6 locations per session connected as a graph (spanning tree +
// 1-2 extra edges).  Each location is assembled from pre-authored node files
// stored in gamedata/nodes/ by combining compatible room chunks side-by-side.
//
// Location size: 800 × 200 blocks at 16 px/block = 12 800 × 3 200 px.
// Style: Dead-Cells aerial — floating islands, open air, multiple paths.
//
// Entry point:  bundle.GenerateRaidProcGen(seed int64) → *GeneratedRaid

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"warpedrealms/shared"
	"warpedrealms/world"
)

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	pgGW    = 800 // grid width  in blocks
	pgGH    = 200 // grid height in blocks
	pgBlock = 16  // pixels per block

	pgLocW = pgGW * pgBlock // 12 800 px
	pgLocH = pgGH * pgBlock //  3 200 px

	pgRooms   = 6 // locations per session
	pgMinPort = 3
	pgMaxPort = 5

	pgNodesDir = "gamedata/nodes"
)

// ─── Biome ────────────────────────────────────────────────────────────────────

var pgBiomeNames = [3]string{"ruins", "crystal", "forest"}

// ─── Graph structures ─────────────────────────────────────────────────────────

type pgPortal struct {
	area    shared.Rect
	arrival shared.Vec2
	label   string
}

type pgLocation struct {
	biome   string
	portals []pgPortal
	spawn   shared.Vec2       // pixel coords of player spawn
	level   world.LDtkWriteLevel
}

type pgEdge struct {
	a, b         int
	portA, portB int
}

type pgGraph struct {
	biomes [pgRooms]string
	locs   [pgRooms]*pgLocation
	edges  []pgEdge
}

// ─── Public entry point ───────────────────────────────────────────────────────

// GenerateRaidProcGen creates a full procedural session:
//  1. Ensures node JSON files exist in gamedata/nodes/.
//  2. Loads nodes and assembles 6 biome-flavoured locations.
//  3. Builds a graph connecting them via portals.
//  4. Writes a LDtk session file to gamedata/sessions/.
//  5. Loads it back and builds a *GeneratedRaid.
func (b *Bundle) GenerateRaidProcGen(seed int64) (*GeneratedRaid, error) {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	// Ensure node files are up to date.
	if err := EnsureNodes(pgNodesDir); err != nil {
		return nil, fmt.Errorf("procgen ensureNodes: %w", err)
	}

	nodes, err := LoadNodes(pgNodesDir)
	if err != nil {
		return nil, fmt.Errorf("procgen loadNodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("procgen: no node definitions found in %s", pgNodesDir)
	}

	graph := pgBuildGraph(rng, nodes)

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

	// Annotate rooms with biome and wire the background-room chain.
	// BelowRoomID makes each room display the next room as its background,
	// which is the core visual feature: you see another sublocation behind you.
	for i := range raid.Layout.Rooms {
		if i < pgRooms {
			bio := graph.biomes[i]
			raid.Layout.Rooms[i].Biome = bio
			raid.Layout.Rooms[i].BackgroundID = "cave" // fallback to existing bg
			raid.Layout.Rooms[i].TileStyleID = ""      // no tile style — renders solid rects
			// Chain: room i shows room (i+1) in the background.
			if i+1 < pgRooms {
				raid.Layout.Rooms[i].BelowRoomID = raid.Layout.Rooms[i+1].ID
				raid.Layout.Rooms[i+1].AboveRoomID = raid.Layout.Rooms[i].ID
			}
		}
	}
	return raid, nil
}

// ─── Graph construction ───────────────────────────────────────────────────────

func pgBuildGraph(rng *rand.Rand, nodes []NodeDef) *pgGraph {
	g := &pgGraph{}

	// Two of each biome in shuffled order.
	pool := []string{
		pgBiomeNames[0], pgBiomeNames[0],
		pgBiomeNames[1], pgBiomeNames[1],
		pgBiomeNames[2], pgBiomeNames[2],
	}
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	copy(g.biomes[:], pool)

	// Generate all locations (each gets its own RNG stream so biome variation
	// is independent of graph wiring).
	for i := 0; i < pgRooms; i++ {
		locRNG := rand.New(rand.NewSource(rng.Int63()))
		id := fmt.Sprintf("room_%02d", i+1)
		lvl := AssembleLocation(locRNG, nodes, g.biomes[i], id)
		nPorts := pgMinPort + rng.Intn(pgMaxPort-pgMinPort+1)
		portals := pgPlacePortals(locRNG, lvl, nPorts)
		spawn := pgPickSpawnFromLevel(lvl)
		g.locs[i] = &pgLocation{
			biome:   g.biomes[i],
			portals: portals,
			spawn:   spawn,
			level:   lvl,
		}
	}

	// Spanning tree (Prim-style from node 0).
	inTree := []int{0}
	pending := []int{1, 2, 3, 4, 5}
	for len(pending) > 0 {
		ri := rng.Intn(len(pending))
		b := pending[ri]
		pending = append(pending[:ri], pending[ri+1:]...)
		a := inTree[rng.Intn(len(inTree))]
		g.edges = append(g.edges, pgMakeEdge(g, a, b))
		inTree = append(inTree, b)
	}

	// Add 1–2 extra edges for loops / shortcuts.
	extras := 1 + rng.Intn(2)
	for e := 0; e < extras; e++ {
		for try := 0; try < 40; try++ {
			a := rng.Intn(pgRooms)
			b := rng.Intn(pgRooms)
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
	levels := make([]world.LDtkWriteLevel, pgRooms)
	for i, loc := range g.locs {
		levels[i] = loc.level
		// Player spawn is already embedded in loc.level.Entities by AssembleLocation
		// (from the node's own Player entity).  Do NOT add a second spawn here.
	}

	// Wire portal pairs from graph edges.
	for _, edge := range g.edges {
		aRoomID := fmt.Sprintf("room_%02d", edge.a+1)
		bRoomID := fmt.Sprintf("room_%02d", edge.b+1)
		ap := g.locs[edge.a].portals[edge.portA]
		bp := g.locs[edge.b].portals[edge.portB]

		// Portal in room A → teleports to room B.
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

		// Portal in room B → teleports to room A.
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
	}

	return world.WriteLDtkFile(path, levels)
}

// ─── Portal placement (from assembled level) ──────────────────────────────────

// pgPlacePortals chooses islands in the assembled level to host portals.
// It scans the solid cells for clusters and picks spaced-out ones.
func pgPlacePortals(rng *rand.Rand, lvl world.LDtkWriteLevel, count int) []pgPortal {
	// Collect candidate X positions spaced across the level width.
	stepX := (lvl.GridW * pgBlock) / (count + 1)
	portals := make([]pgPortal, 0, count)

	for i := 0; i < count; i++ {
		cx := float64(stepX*(i+1)) + float64(rng.Intn(stepX/3)-(stepX/6))
		// Find the topmost solid cell near this X.
		surfaceY := pgFindSurface(lvl.SolidCells, int(cx)/pgBlock, lvl.GridW, lvl.GridH)
		py := float64(surfaceY*pgBlock) - 72 // portal hovers above surface
		if py < 0 {
			py = 8
		}
		portals = append(portals, pgPortal{
			area: shared.Rect{X: cx - 28, Y: py, W: 56, H: 56},
			arrival: shared.Vec2{
				X: cx,
				Y: float64(surfaceY*pgBlock) - 120,
			},
			label: pgPortalLabel(i),
		})
	}
	return portals
}

// pgFindSurface scans a column near colX and returns the Y block of the
// highest solid cell (topmost solid = lowest Y value) that has open air above.
func pgFindSurface(cells [][2]int, colX, gridW, gridH int) int {
	// Build column occupancy for nearby columns (±5 blocks).
	occupied := map[[2]int]bool{}
	for _, c := range cells {
		occupied[c] = true
	}

	bestY := gridH - 4 // fallback: near bottom
	for dx := -5; dx <= 5; dx++ {
		cx := colX + dx
		if cx < 0 || cx >= gridW {
			continue
		}
		for row := 0; row < gridH; row++ {
			if !occupied[[2]int{cx, row}] {
				continue
			}
			// Check that the row above is air.
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

// pgPickSpawnFromLevel finds a good player spawn above the first solid surface
// near the left edge of the assembled level.
func pgPickSpawnFromLevel(lvl world.LDtkWriteLevel) shared.Vec2 {
	surfaceY := pgFindSurface(lvl.SolidCells, pgGW/10, lvl.GridW, lvl.GridH)
	return shared.Vec2{
		X: float64(pgGW/10) * pgBlock,
		Y: float64(surfaceY*pgBlock) - 110,
	}
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
