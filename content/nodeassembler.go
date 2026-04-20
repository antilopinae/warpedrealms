package content

// nodeassembler.go — assembles room locations from node definitions.
//
// Node definitions use their own internal coordinate space:
//   naNodeH = 200 blocks tall, variable width (60-150 blocks).
//
// The assembler maps that internal space onto the target grid supplied via
// ProcGenConfig (default 600 × 300).  Y-coordinates are scaled proportionally:
//   scaledY = nodeY * gridH / naNodeH
//
// After assembly a "scatter" pass fills vertical gaps with small stepping-stone
// platforms, controlled by ProcGenConfig.Density (0 = none, 1 = max).
//
// Jump physics constants (in node-space, naNodeH = 200):
//   naJumpUp   = 16 blocks  — max upward jump
//   naDropDown = 28 blocks  — max safe downward drop

import (
	"math/rand"
	"sort"

	"warpedrealms/world"
)

const (
	naNodeH  = 200 // node native height — coordinate space of NodeDef islands
	naGridSz = 16  // pixels per block (fixed)
)

// AssembleLocation builds one location (world.LDtkWriteLevel) from the node
// catalogue.  biome is one of "ruins", "crystal", "forest".
// id is the LDtk level identifier (e.g. "room_01").
// cfg controls target dimensions, player physics, and scatter density.
func AssembleLocation(rng *rand.Rand, nodes []NodeDef, biome, id string, cfg ProcGenConfig) world.LDtkWriteLevel {
	targetW := cfg.GridW
	gridH := cfg.GridH

	// Compute socket-compatibility thresholds from player physics.
	jumpUp := cfg.nodeSpaceJumpUp(naNodeH)
	dropDown := cfg.nodeSpaceDropDown(naNodeH)

	// ── filter nodes for this biome ──────────────────────────────────────────
	var pool []NodeDef
	for _, n := range nodes {
		for _, t := range n.Tags {
			if t == biome {
				pool = append(pool, n)
				break
			}
		}
	}
	// Include universal connector nodes (avoid duplicates).
	for _, n := range nodes {
		isConn := false
		for _, t := range n.Tags {
			if t == "connector" {
				isConn = true
				break
			}
		}
		if !isConn {
			continue
		}
		dup := false
		for _, p := range pool {
			if p.ID == n.ID {
				dup = true
				break
			}
		}
		if !dup {
			pool = append(pool, n)
		}
	}
	if len(pool) == 0 {
		pool = nodes
	}

	// ── select start node ────────────────────────────────────────────────────
	var startNodes []NodeDef
	for _, n := range pool {
		for _, t := range n.Tags {
			if t == "start" {
				startNodes = append(startNodes, n)
				break
			}
		}
	}
	if len(startNodes) == 0 {
		startNodes = pool
	}
	sequence := []NodeDef{startNodes[rng.Intn(len(startNodes))]}
	totalW := sequence[0].Width

	// ── grow sequence until width target met ─────────────────────────────────
	for totalW < targetW {
		prev := sequence[len(sequence)-1]
		remaining := targetW - totalW

		var candidates []NodeDef
		for _, n := range pool {
			if n.ID == prev.ID {
				continue
			}
			if n.Width > remaining+80 {
				continue
			}
			dy := n.LeftSocket.SurfaceY - prev.RightSocket.SurfaceY
			if dy < -jumpUp || dy > dropDown {
				continue
			}
			candidates = append(candidates, n)
		}

		if len(candidates) == 0 {
			connector := naPickConnector(pool, prev.RightSocket.SurfaceY)
			sequence = append(sequence, connector)
			totalW += connector.Width
			continue
		}

		sort.Slice(candidates, func(i, j int) bool {
			di := abs(candidates[i].Width - (targetW - totalW))
			dj := abs(candidates[j].Width - (targetW - totalW))
			return di < dj
		})
		top := len(candidates) / 3
		if top < 1 {
			top = 1
		}
		chosen := candidates[rng.Intn(top)]
		sequence = append(sequence, chosen)
		totalW += chosen.Width
	}

	// ── assemble solid cells, scaling Y from naNodeH → gridH ─────────────────
	type cellXY = [2]int
	solidSet := make(map[cellXY]bool, totalW*gridH/8)

	addCell := func(col, row int) {
		solidSet[cellXY{col, row}] = true
	}

	var entities []world.LDtkWriteEntity
	offsetX := 0
	playerPlaced := false

	for ni, node := range sequence {
		for _, isl := range node.Islands {
			// Scale Y from node-space to grid-space.
			scaledY := isl.Y * gridH / naNodeH
			scaledH := isl.H * gridH / naNodeH
			if scaledH < 1 {
				scaledH = 1
			}
			// GND islands (bottom 6% of grid, i.e. scaledY ≥ 94% of gridH) keep
			// up to 3 blocks of thickness so the floor looks solid.
			// All other islands (L1–L7 floating platforms) are capped at 1 block.
			// This frees rows 275–281 between L1 (row 274) and GND (row 282) for
			// visible near-ground scatter platforms.
			if scaledY >= gridH*94/100 {
				if scaledH > 3 {
					scaledH = 3
				}
			} else {
				scaledH = 1
			}
			for dy := 0; dy < scaledH; dy++ {
				for dx := 0; dx < isl.W; dx++ {
					addCell(offsetX+isl.X+dx, scaledY+dy)
				}
			}
		}

		for _, ent := range node.Entities {
			px := (offsetX + ent.X) * naGridSz
			py := ent.Y * gridH / naNodeH * naGridSz
			switch ent.Type {
			case "Player":
				if ni == 0 && !playerPlaced {
					entities = append(entities, world.LDtkWriteEntity{
						Identifier: "Player",
						PX:         px,
						PY:         py,
						W:          26,
						H:          76,
					})
					playerPlaced = true
				}
			case "Rat":
				entities = append(entities, world.LDtkWriteEntity{
					Identifier: "Rat",
					PX:         px,
					PY:         py,
					W:          20,
					H:          30,
				})
			}
		}

		offsetX += node.Width
	}

	// Ensure player spawn.
	if !playerPlaced {
		first := sequence[0]
		spawnX := first.Width / 2
		spawnY := first.LeftSocket.SurfaceY * gridH / naNodeH
		if spawnY < 2 {
			spawnY = 2
		}
		entities = append(entities, world.LDtkWriteEntity{
			Identifier: "Player",
			PX:         spawnX * naGridSz,
			PY:         (spawnY - 6) * naGridSz,
			W:          26,
			H:          76,
		})
	}

	actualW := offsetX
	if actualW < targetW {
		actualW = targetW
	}

	// ── scatter pass: add stepping stones in sparse vertical gaps ─────────────
	if cfg.Density > 0 {
		naScatter(rng, solidSet, actualW, gridH, cfg.Density)
	}

	// Flatten set → slice.
	solidCells := make([][2]int, 0, len(solidSet))
	for c := range solidSet {
		solidCells = append(solidCells, c)
	}

	return world.LDtkWriteLevel{
		ID:         id,
		GridW:      actualW,
		GridH:      gridH,
		GridSize:   naGridSz,
		SolidCells: solidCells,
		Entities:   entities,
	}
}

// naScatter builds the full stepping-stone layout in two passes:
//
//  1. naGrowPlatformGraph — generates platforms as a connected jump-graph
//     grown from the floor.  Every platform placed has a parent it is reachable
//     from in one jump, so the graph is connected by construction.
//
//  2. naBridgeIsolated — safety net that catches node-island platforms that
//     were not reached during growth (e.g. platforms near the top) and inserts
//     small bridge stones to connect them to the main graph.
func naScatter(rng *rand.Rand, cells map[[2]int]bool, gridW, gridH int, density float64) {
	// Pass 1: ground terrain bumps — short platforms flush with the floor,
	// merged into the terrain.  Player can walk over them and jump from their
	// tops to reach higher stepping stones.
	naPlantGroundBumps(rng, cells, gridW, gridH)
	// Pass 2: first floating stepping stones just above the floor.
	naPlantGroundSteps(rng, cells, gridW, gridH)
	// Pass 3: connected platform graph grown upward from floor + all seeds.
	naGrowPlatformGraph(rng, cells, gridW, gridH, density)
	// Pass 4: merge 1-block horizontal and vertical gaps.
	naMergeClose(cells, gridW, gridH)
	// Pass 5: safety-net bridge for any still-isolated node-island platforms.
	naBridgeIsolated(cells, gridW, gridH)
}

// naFloorSurface returns the topmost solid row in the bottom 40 % of the
// level — i.e. the actual walkable floor surface that node-island assembly
// produced.  Columns are sampled every 4 blocks for speed.
// Falls back to gridH-3 if the level has no solid cells in that zone.
func naFloorSurface(cells map[[2]int]bool, gridW, gridH int) int {
	surface := gridH - 1
	// Node levels in grid space (gridH=300, naNodeH=200):
	//   L1=183 → row 274, 3 blocks thick → occupies rows 274–276 (bottom=276).
	//   GND=188 → row 282 exactly (94% of 300 = 282).
	// 93% = row 279 → still finds ground bumps (at row 281) or L1 bottom (276)
	//   on subsequent calls after naPlantGroundBumps has run.
	// 94% = row 282 = GND top.  Bumps sit at row 281 < 282 = lo → out of scan
	//   range → never picked up.  All L1–L7 are above row 282 → never picked up.
	//   This gives a stable floor=282 regardless of call order.
	lo := gridH * 94 / 100
	for col := 0; col < gridW; col += 4 {
		for row := lo; row < gridH; row++ {
			if cells[[2]int{col, row}] {
				if row < surface {
					surface = row
				}
				break
			}
		}
	}
	if surface >= gridH-1 {
		surface = gridH - 3 // fallback
	}
	return surface
}

// naPlantGroundBumps places short platforms directly on (or 1–2 rows above)
// the floor surface, creating terrain bumps that merge with the ground via
// naMergeClose.  The bumps serve as the first rung of the climbing ladder:
// the player walks onto a bump (or jumps to its top) and from there can reach
// the floating stepping stones placed by naPlantGroundSteps.
//
// Design rules:
//   height  1 → platform at floor-1, merges into a 2-block-tall bump.
//   height  2 → platform at floor-2, merges via the vertical gap fill into
//               a 3-block-tall bump.  Player can jump over (maxJump = 8 rows).
//   walkGap — minimum clear ground between consecutive bumps so the player
//             can always walk left-to-right at ground level between bumps.
//   The bump tops are also picked up by naGrowPlatformGraph as frontier seeds,
//   so the graph connects each bump upward to higher platforms.
func naPlantGroundBumps(rng *rand.Rand, cells map[[2]int]bool, gridW, gridH int) {
	const (
		// height=1 only: bump sits at floor-1 (row 281 when floor=282).
		// Player standing on it occupies rows 277-280 — safely below L1 (274-276).
		// height=2 would put the bump at floor-2 (row 280); player body reaches
		// row 276 = L1 bottom → collision.  Keep height=1 always.
		minHeight = 1  // rows above floor
		maxHeight = 1  // rows above floor (keep same as min to stay safe)
		minWidth  = 5  // blocks
		maxWidth  = 10 // blocks
		walkGap   = 18 // minimum clear ground between bumps (blocks)
		jitter    = 8  // random extra spacing so bumps aren't mechanical
	)

	floor := naFloorSurface(cells, gridW, gridH)

	col := 3 + rng.Intn(8)
	for col+maxWidth < gridW-3 {
		width := minWidth + rng.Intn(maxWidth-minWidth+1)
		height := minHeight + rng.Intn(maxHeight-minHeight+1)
		row := floor - height
		if row >= 2 {
			for dx := 0; dx < width; dx++ {
				if c := col + dx; c >= 0 && c < gridW {
					cells[[2]int{c, row}] = true
				}
			}
		}
		col += width + walkGap + rng.Intn(jitter)
	}
}

// naPlantGroundSteps places small stepping-stone platforms very close to the
// floor surface (1–3 rows above) so the player can jump from ground level to
// higher platforms.  They serve as the first rung of the vertical ladder.
//
// Design rules:
//   minAbove/maxAbove — 1–3 rows above the floor.  The player steps ON these
//               stones, not under them, so walk-under clearance is not needed.
//               The isClear check (playerH rows above the stone) naturally
//               rejects only positions where the player can't stand (head hits
//               a ceiling); with stones ≤ 3 rows above GND the L1 node-island
//               platforms at row 274 are out of the check window (GND≈282,
//               stone at 279–281, head at 275–277, L1 at 274 → no collision).
//   spacing   — horizontal distance (stoneW + walkGap) keeps ≥ 15 blocks of
//               open floor between stones so the player can always walk through.
//
// These stones are written into cells BEFORE naGrowPlatformGraph runs, so the
// grower picks them up as frontier seeds and connects each one upward.
func naPlantGroundSteps(rng *rand.Rand, cells map[[2]int]bool, gridW, gridH int) {
	const (
		playerH  = 4
		stoneW   = 5
		// With L1 islands now 1 block thick (only row 274), the free zone between
		// L1 and GND is rows 275–281.
		// Player standing on row R occupies rows R-playerH to R-1.
		// Row 278 (above=4): player head at 274 = L1 → collision.
		// Row 279 (above=3): player head at 275, L1 at 274 → safe ✓
		// Row 280 (above=2): player at 276-279, L1 at 274 → safe ✓
		// Rows 279–280 are clearly visible above the floor; the isClear check
		// already rejects positions where L1 would cause a body collision.
		minAbove = 2 // floor-2 = row 280: visible above floor
		maxAbove = 3 // floor-3 = row 279: slightly higher, still near ground
		walkGap  = 15
		jitter   = 4
	)

	floor := naFloorSurface(cells, gridW, gridH)
	spacing := stoneW + walkGap // min center-to-center start distance

	col := stoneW + 1 + rng.Intn(jitter)
	for col+stoneW < gridW-2 {
		above := minAbove + rng.Intn(maxAbove-minAbove+1)
		row := floor - above
		if row >= 2 && row < gridH-2 {
			// Check playerH rows above + the stone row itself are clear.
			clear := true
			for r := row - playerH; r <= row && clear; r++ {
				for dx := -1; dx < stoneW+1 && clear; dx++ {
					c := col + dx
					if c >= 0 && c < gridW && cells[[2]int{c, r}] {
						clear = false
					}
				}
			}
			if clear {
				for dx := 0; dx < stoneW; dx++ {
					if c := col + dx; c >= 0 && c < gridW {
						cells[[2]int{c, row}] = true
					}
				}
			}
		}
		col += spacing + rng.Intn(jitter*2+1)
	}
}

// naGrowPlatformGraph generates stepping-stone platforms as a connected
// jump-graph grown upward from the floor.
//
// Every platform placed has a "parent" platform it is reachable from in one
// jump, so the entire generated set is connected by construction — no
// post-processing needed.
//
// Algorithm:
//   - Virtual floor (full level width, row gridH-1) is the root.
//   - All existing node-island platforms are seeded into the frontier so the
//     growth process extends from them too.
//   - Each step: pick a frontier platform via tournament selection (prefer
//     platforms lower on screen with fewer children), then spawn a child
//     shifted horizontally by minHShift..maxHShift blocks and vertically by
//     minStep..maxStep rows upward.
//   - Horizontal shift is mandatory: child is NEVER directly above parent,
//     which prevents vertical stacking.
//   - isClear enforces playerHBlocks headroom above every new platform.
//   - Growth continues until 'target' platforms are placed or no frontier
//     node can spawn children.
func naGrowPlatformGraph(rng *rand.Rand, cells map[[2]int]bool, gridW, gridH int, density float64) {
	const (
		playerH   = 4 // ColliderH / gridSize — rows player occupies
		stoneW    = 5 // placed platform width (blocks)
		minStep   = 5 // min vertical rows between parent and child
		maxStep   = 8 // max vertical rows = max jump height
		minHShift = 4 // min horizontal shift — guarantees child ≠ above parent
		maxHShift = 9 // max horizontal shift — stays within jump range
		// Horizontal buffer in isClear: prevents two platforms from adjacent
		// chains ending up at nearly the same height with a 1-block gap.
		clearBuf = stoneW + 3 // blocks checked left and right of the new stone
		maxKids  = 3          // max children per frontier node before retirement
		topRow   = 4          // upper boundary: scatter platforms stay ≥ 4 rows from top
	)

	// isClear returns true when placing stoneW cells at (col, row) leaves
	// playerH rows of headroom above and doesn't overlap any existing solid.
	isClear := func(col, row int) bool {
		for r := row - playerH; r <= row; r++ {
			if r < 0 || r >= gridH {
				continue
			}
			for dx := -clearBuf; dx < stoneW+clearBuf; dx++ {
				c := col + dx
				if c >= 0 && c < gridW && cells[[2]int{c, r}] {
					return false
				}
			}
		}
		return true
	}

	placePlat := func(col, row int) naPlatform {
		hi := col + stoneW - 1
		for dx := 0; dx < stoneW; dx++ {
			c := col + dx
			if c >= 0 && c < gridW {
				cells[[2]int{c, row}] = true
			} else if c >= gridW {
				hi = gridW - 1
			}
		}
		return naPlatform{row, col, hi}
	}

	// frontierNode wraps a platform together with its child count.
	type frontierNode struct {
		p     naPlatform
		nKids int
	}

	// Seed the frontier: virtual floor at the ACTUAL ground surface (not
	// gridH-1 which is underground inside the solid node-island mass) +
	// all existing platforms already written into cells.
	floor := naPlatform{Row: naFloorSurface(cells, gridW, gridH), ColLo: 0, ColHi: gridW - 1}
	frontier := []frontierNode{{floor, 0}}
	for _, p := range naExtractPlatforms(cells, gridW, gridH, 3) {
		frontier = append(frontier, frontierNode{p, 0})
	}

	// Target platform count: roughly one platform every 16 blocks horizontally
	// across the usable height range, scaled by density.
	// Capped at gridW/4 to keep generation fast.
	avgStep := (minStep + maxStep) / 2
	floorRow := naFloorSurface(cells, gridW, gridH)
	usableH := floorRow - topRow
	target := int(float64((gridW/16)*(usableH/avgStep)) * density * 0.35)
	if target < 8 {
		target = 8
	}
	if target > gridW/4 {
		target = gridW / 4
	}

	placed := 0
	stuckCount := 0
	for attempt := 0; attempt < target*20 && placed < target; attempt++ {
		if len(frontier) == 0 {
			break
		}

		// Tournament selection over 3 random candidates.
		// Score = platform row (higher = lower on screen = closer to floor)
		//         minus penalty for many children already spawned.
		// We prefer nodes that are lower and have few children: they cover
		// the bottom of the level first and spread the tree evenly.
		best := rng.Intn(len(frontier))
		for k := 0; k < 2; k++ {
			j := rng.Intn(len(frontier))
			sb := frontier[best].p.Row - frontier[best].nKids*maxStep
			sj := frontier[j].p.Row - frontier[j].nKids*maxStep
			if sj > sb {
				best = j
			}
		}
		node := &frontier[best]

		// Retire nodes that are too high or maxed out.
		if node.p.Row-maxStep < topRow || node.nKids >= maxKids {
			frontier = append(frontier[:best], frontier[best+1:]...)
			continue
		}

		// Try spawning a child in a random direction; fall back to the other.
		dir := 1
		if rng.Intn(2) == 0 {
			dir = -1
		}
		childPlaced := false
		for try := 0; try < 2 && !childPlaced; try++ {
			hShift := minHShift + rng.Intn(maxHShift-minHShift+1)
			vStep := minStep + rng.Intn(maxStep-minStep+1)

			childCol := naMidCol(node.p) + dir*hShift - stoneW/2
			childRow := node.p.Row - vStep

			// Clamp to level bounds.
			if childCol < 1 {
				childCol = 1
			}
			if childCol+stoneW >= gridW {
				childCol = gridW - stoneW - 1
			}
			if childRow < topRow {
				dir = -dir
				continue
			}

			if isClear(childCol, childRow) {
				child := placePlat(childCol, childRow)
				frontier = append(frontier, frontierNode{child, 0})
				node.nKids++
				placed++
				childPlaced = true
			}
			dir = -dir
		}

		// Count failed attempts against the node so stuck nodes eventually retire.
		if !childPlaced {
			node.nKids++
			stuckCount++
		} else {
			stuckCount = 0
		}
		// Early exit: if 200 consecutive attempts all failed the level is too
		// crowded to place more platforms.
		if stuckCount > 200 {
			break
		}
	}
}

// naPlaceStones is the inner loop: scans columns, finds the largest gap
// between consecutive solid rows in [rowLo, rowHi], and places a stone.
func naPlaceStones(
	rng *rand.Rand,
	cells map[[2]int]bool,
	colRows map[int][]int,
	gridW, gridH int,
	density float64,
	minGap, rowLo, rowHi int,
) {
	const stoneW = 6
	const colStride = 4

	for col := colStride; col < gridW-stoneW-colStride; col += colStride {
		if rng.Float64() > density {
			continue
		}
		rows, ok := colRows[col]
		if !ok || len(rows) < 2 {
			continue
		}
		// Find the largest gap between consecutive solid rows within [rowLo, rowHi].
		bestGap := 0
		bestMid := 0
		for i := 1; i < len(rows); i++ {
			if rows[i] < rowLo || rows[i-1] >= rowHi {
				continue
			}
			gap := rows[i] - rows[i-1]
			if gap > bestGap {
				bestGap = gap
				bestMid = (rows[i-1] + rows[i]) / 2
			}
		}
		if bestGap < minGap {
			continue
		}
		// Place a stone near the mid-point of the gap with a small random jitter.
		jitter := bestGap / 8
		if jitter < 1 {
			jitter = 1
		}
		row := bestMid + rng.Intn(jitter*2) - jitter
		if row < 2 || row >= gridH-2 {
			continue
		}
		// Skip if the 4 rows above are already occupied (player would be blocked).
		const playerH = 4
		aboveCrowded := false
		for check := row - playerH; check <= row-1 && !aboveCrowded; check++ {
			if check < 0 {
				continue
			}
			for dx := 0; dx < stoneW; dx++ {
				if cells[[2]int{col - stoneW/2 + dx, check}] {
					aboveCrowded = true
					break
				}
			}
		}
		if aboveCrowded {
			continue
		}
		// Skip if any target cell is already solid.
		startCol := col - stoneW/2 + rng.Intn(colStride)
		placed := false
		for dx := 0; dx < stoneW; dx++ {
			c := col + startCol%colStride + dx
			if c < 0 || c >= gridW {
				continue
			}
			if cells[[2]int{c, row}] {
				placed = false
				break
			}
			placed = true
		}
		if !placed {
			continue
		}
		for dx := 0; dx < stoneW; dx++ {
			c := col + startCol%colStride + dx
			if c >= 0 && c < gridW {
				cells[[2]int{c, row}] = true
			}
		}
	}
}

// ── connectivity graph ────────────────────────────────────────────────────────

// naPlatform is a horizontal run of consecutive solid cells on one row.
type naPlatform struct {
	Row   int
	ColLo int // first solid column (inclusive)
	ColHi int // last solid column (inclusive)
}

func naMidCol(p naPlatform) int { return (p.ColLo + p.ColHi) / 2 }

// naExtractPlatforms scans all rows and returns every horizontal run of
// consecutive solid cells that is at least minW blocks wide.
func naExtractPlatforms(cells map[[2]int]bool, gridW, gridH, minW int) []naPlatform {
	var out []naPlatform
	for row := 0; row < gridH; row++ {
		start := -1
		for col := 0; col <= gridW; col++ {
			solid := col < gridW && cells[[2]int{col, row}]
			if solid && start < 0 {
				start = col
			} else if !solid && start >= 0 {
				if col-start >= minW {
					out = append(out, naPlatform{row, start, col - 1})
				}
				start = -1
			}
		}
	}
	return out
}

// naCanReach returns true if a player standing on platform a can jump to
// platform b (upward or downward drop).
func naCanReach(a, b naPlatform) bool {
	const (
		maxJumpRows = 8  // max rows upward a player can jump
		maxDropRows = 12 // max rows downward a player can safely drop
		maxHorizGap = 10 // max horizontal gap (blocks) bridgeable during a jump
	)
	dRow := a.Row - b.Row // positive = b is higher on screen (smaller row = higher)
	if dRow < -maxDropRows || dRow > maxJumpRows {
		return false
	}
	// Horizontal gap between the two spans (0 if they overlap).
	gap := 0
	if b.ColLo > a.ColHi {
		gap = b.ColLo - a.ColHi
	} else if a.ColLo > b.ColHi {
		gap = a.ColLo - b.ColHi
	}
	return gap <= maxHorizGap
}

// naFindComponents assigns each platform a component ID via BFS and returns
// the component ID of the largest component (the "main" reachable set).
func naFindComponents(platforms []naPlatform) (comp []int, mainComp int) {
	comp = make([]int, len(platforms))
	for i := range comp {
		comp[i] = -1
	}
	nextID := 0
	for i := range platforms {
		if comp[i] >= 0 {
			continue
		}
		queue := []int{i}
		comp[i] = nextID
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for j := range platforms {
				if comp[j] >= 0 {
					continue
				}
				if naCanReach(platforms[cur], platforms[j]) || naCanReach(platforms[j], platforms[cur]) {
					comp[j] = nextID
					queue = append(queue, j)
				}
			}
		}
		nextID++
	}
	// Largest component = main reachable set.
	size := make(map[int]int, nextID)
	for _, c := range comp {
		size[c]++
	}
	mainComp = 0
	best := 0
	for c, sz := range size {
		if sz > best {
			best = sz
			mainComp = c
		}
	}
	return comp, mainComp
}

// naMergeClose eliminates 1-block gaps between platforms in both directions:
//
//   Horizontal pass — for each row, fills gaps of ≤ 1 empty column between
//   consecutive solid runs so two almost-touching platforms on the same row
//   become one.
//
//   Vertical pass — for each column, if two solid cells are separated by
//   exactly 1 empty row, fills that row.  This removes the thin 1-block
//   "step" that appears when the graph places a child platform 1 row offset
//   from an adjacent platform edge.
func naMergeClose(cells map[[2]int]bool, gridW, gridH int) {
	// Horizontal pass.
	for row := 0; row < gridH; row++ {
		type run struct{ lo, hi int }
		var runs []run
		start := -1
		for col := 0; col <= gridW; col++ {
			solid := col < gridW && cells[[2]int{col, row}]
			if solid && start < 0 {
				start = col
			} else if !solid && start >= 0 {
				runs = append(runs, run{start, col - 1})
				start = -1
			}
		}
		for i := 1; i < len(runs); i++ {
			if gap := runs[i].lo - runs[i-1].hi - 1; gap <= 1 {
				for col := runs[i-1].hi + 1; col < runs[i].lo; col++ {
					cells[[2]int{col, row}] = true
				}
			}
		}
	}

	// Vertical pass: fill 1-row gaps within the same column.
	for col := 0; col < gridW; col++ {
		for row := 1; row < gridH-1; row++ {
			if !cells[[2]int{col, row}] &&
				cells[[2]int{col, row - 1}] &&
				cells[[2]int{col, row + 1}] {
				cells[[2]int{col, row}] = true
			}
		}
	}
}

// naBridgeIsolated is a safety-net pass that runs after naGrowPlatformGraph.
// It finds platform components that are still disconnected from the main graph
// (typically node-island platforms the growth pass didn't reach) and inserts
// small bridge stones at the midpoint between the closest pair of platforms
// across the gap.
//
// The loop runs until the graph is fully connected.  The only exit condition
// other than full connectivity is a failed bridge placement — if every
// candidate row around the midpoint is already occupied, we skip that pair and
// try again on the next iteration (the component structure will have changed).
// In practice this converges quickly because each bridge reduces the number of
// components by at least one.
func naBridgeIsolated(cells map[[2]int]bool, gridW, gridH int) {
	const (
		bridgeW  = 4
		playerH  = 4
		minPlatW = 3
	)

	// canPlace returns true when bridgeW cells at (col, row) are free and
	// have playerH rows of headroom above.
	canPlace := func(col, row int) bool {
		for r := row - playerH; r <= row; r++ {
			if r < 0 || r >= gridH {
				continue
			}
			for dx := 0; dx < bridgeW; dx++ {
				c := col + dx
				if c >= 0 && c < gridW && cells[[2]int{c, r}] {
					return false
				}
			}
		}
		return true
	}

	for {
		platforms := naExtractPlatforms(cells, gridW, gridH, minPlatW)
		if len(platforms) < 2 {
			break
		}
		comp, mainComp := naFindComponents(platforms)

		// Find the disconnected platform closest to the main component.
		bestDist := 1 << 30
		bestI, bestJ := -1, -1
		for i, ci := range comp {
			if ci == mainComp {
				continue
			}
			for j, cj := range comp {
				if cj != mainComp {
					continue
				}
				dRow := abs(platforms[i].Row - platforms[j].Row)
				dCol := abs(naMidCol(platforms[i]) - naMidCol(platforms[j]))
				d := dRow*3 + dCol
				if d < bestDist {
					bestDist = d
					bestI, bestJ = i, j
				}
			}
		}
		if bestI < 0 {
			break // fully connected — done
		}

		pi, pj := platforms[bestI], platforms[bestJ]
		midRow := (pi.Row + pj.Row) / 2
		midCol := (naMidCol(pi)+naMidCol(pj))/2 - bridgeW/2
		if midCol < 1 {
			midCol = 1
		}
		if midCol+bridgeW >= gridW {
			midCol = gridW - bridgeW - 1
		}

		// Try midRow and expanding offsets until we find a clear spot.
		// If nothing fits (extremely dense area) move the column slightly and
		// retry — this guarantees we always make progress toward connectivity.
		bridgePlaced := false
		for colShift := 0; colShift <= gridW/4 && !bridgePlaced; colShift += bridgeW + 1 {
			for _, cs := range []int{0, colShift, -colShift} {
				col := midCol + cs
				if col < 1 {
					col = 1
				}
				if col+bridgeW >= gridW {
					col = gridW - bridgeW - 1
				}
				for offset := 0; offset <= gridH/4 && !bridgePlaced; offset++ {
					for _, sign := range []int{0, 1, -1} {
						tryRow := midRow + sign*offset
						if tryRow < 2 || tryRow >= gridH-2 {
							continue
						}
						if canPlace(col, tryRow) {
							for dx := 0; dx < bridgeW; dx++ {
								cells[[2]int{col + dx, tryRow}] = true
							}
							bridgePlaced = true
							break
						}
					}
				}
			}
		}

		// If even with a wide search we could not place a bridge, the level
		// geometry makes this gap unresolvable — stop to avoid an infinite loop.
		if !bridgePlaced {
			break
		}
	}
}

// naPickConnector finds the connector node closest in socket height.
func naPickConnector(pool []NodeDef, prevSurface int) NodeDef {
	best := pool[0]
	bestDy := 1 << 20
	for _, n := range pool {
		for _, t := range n.Tags {
			if t == "connector" {
				dy := abs(n.LeftSocket.SurfaceY - prevSurface)
				if dy < bestDy {
					bestDy = dy
					best = n
				}
			}
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
