package clientapp

import (
	"container/heap"
	"math"

	"warpedrealms/shared"
)

// A* platformer pathfinding constants.
// The bot body occupies astarBW × astarBH grid cells (3 wide, 5 tall).
const (
	astarBlock = 16  // world pixels per grid cell
	astarBW    = 3   // body width in cells (48 px)
	astarBH    = 5   // body height in cells (80 px)
	astarBHalf = 1   // half of body width (floor(3/2))

	astarJumpH  = 8  // max jump height in cells (~131 px = 8.2 blocks)
	astarJumpDX = 10 // max horizontal range during a jump
	astarDropDY = 8  // max fall distance considered in one step
)

// ─── Grid ─────────────────────────────────────────────────────────────────────

// botGrid is a grid of solid and one-way-platform cells built from a room's
// physics rects.  Used only for A* pathfinding; the actual movement uses the
// same SimulatePlayer physics as the real player.
type botGrid struct {
	solid  [][]bool
	oneway [][]bool // one-way platforms (passable from below)
	cols   int
	rows   int
	ox, oy float64 // world origin (= room Bounds top-left)
}

func newBotGrid(room shared.RoomState, solids, platforms []shared.Rect) *botGrid {
	cols := max(1, int(math.Ceil(room.Bounds.W/astarBlock)))
	rows := max(1, int(math.Ceil(room.Bounds.H/astarBlock)))

	solid := make([][]bool, rows)
	oneway := make([][]bool, rows)
	for r := range solid {
		solid[r] = make([]bool, cols)
		oneway[r] = make([]bool, cols)
	}

	ox, oy := room.Bounds.X, room.Bounds.Y

	markRect := func(s shared.Rect, grid [][]bool) {
		c0 := int(math.Floor((s.X - ox) / astarBlock))
		r0 := int(math.Floor((s.Y - oy) / astarBlock))
		c1 := int(math.Ceil((s.X + s.W - ox) / astarBlock))
		r1 := int(math.Ceil((s.Y + s.H - oy) / astarBlock))
		for r := max(0, r0); r < min(rows, r1); r++ {
			for c := max(0, c0); c < min(cols, c1); c++ {
				grid[r][c] = true
			}
		}
	}

	for _, s := range solids {
		markRect(s, solid)
	}
	for _, p := range platforms {
		markRect(p, oneway)
	}

	return &botGrid{solid: solid, oneway: oneway, cols: cols, rows: rows, ox: ox, oy: oy}
}

// cellOf returns the grid cell (col, row) that contains world position (x, y).
func (g *botGrid) cellOf(x, y float64) (col, row int) {
	col = int(math.Floor((x - g.ox) / astarBlock))
	row = int(math.Floor((y - g.oy) / astarBlock))
	return
}

// cellFeetWorld returns the world position of the center-bottom of cell (col, row).
// "Feet" means the bottom edge of the cell — the y-position the entity would
// have if standing on a surface at that row.
func (g *botGrid) cellFeetWorld(col, row int) (x, y float64) {
	x = g.ox + float64(col)*astarBlock + astarBlock*0.5
	y = g.oy + float64(row+1)*astarBlock
	return
}

func (g *botGrid) inBounds(col, row int) bool {
	return col >= 0 && col < g.cols && row >= 0 && row < g.rows
}

func (g *botGrid) isSolid(col, row int) bool {
	if col < 0 || col >= g.cols || row < 0 || row >= g.rows {
		return true // walls / ceiling / floor boundaries are treated as solid
	}
	return g.solid[row][col]
}

func (g *botGrid) isFloorAt(col, row int) bool {
	if col < 0 || col >= g.cols || row < 0 || row >= g.rows {
		return false
	}
	return g.solid[row][col] || g.oneway[row][col]
}

// standable returns true if the bot can stand with its centre column at col and
// its FEET row at row (body cells: [col-1..col+1] × [row-(BH-1)..row] are all
// clear, and at least one cell in [col-1..col+1] × [row+1] is a floor).
func (g *botGrid) standable(col, row int) bool {
	if !g.inBounds(col, row) {
		return false
	}
	// Body must be clear.
	for dc := -astarBHalf; dc <= astarBHalf; dc++ {
		for dr := -(astarBH - 1); dr <= 0; dr++ {
			if g.isSolid(col+dc, row+dr) {
				return false
			}
		}
	}
	// Floor must exist below the body.
	for dc := -astarBHalf; dc <= astarBHalf; dc++ {
		if g.isFloorAt(col+dc, row+1) {
			return true
		}
	}
	return false
}

// snapToStandable searches from (col, row) outward for the nearest standable
// cell.  Returns (-1, -1) if nothing found within 8 cells.
func (g *botGrid) snapToStandable(col, row int) (int, int) {
	if g.standable(col, row) {
		return col, row
	}
	for radius := 1; radius <= 8; radius++ {
		for dr := -radius; dr <= radius; dr++ {
			for dc := -radius; dc <= radius; dc++ {
				if iabs(dr) != radius && iabs(dc) != radius {
					continue
				}
				if g.standable(col+dc, row+dr) {
					return col + dc, row + dr
				}
			}
		}
	}
	return -1, -1
}

// ─── A* ───────────────────────────────────────────────────────────────────────

type astarNode struct {
	col, row int
	g, f     float64
	prev     *astarNode
	index    int // position in the heap
}

type astarHeap []*astarNode

func (h astarHeap) Len() int            { return len(h) }
func (h astarHeap) Less(i, j int) bool  { return h[i].f < h[j].f }
func (h astarHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *astarHeap) Push(x any) {
	n := x.(*astarNode)
	n.index = len(*h)
	*h = append(*h, n)
}
func (h *astarHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	x.index = -1
	return x
}

type astarKey [2]int

// FindPath returns world-space foot positions leading from (sx, sy) to (gx, gy).
// Positions are snapped to the nearest standable grid cell.
// Returns nil when no path exists or start == goal.
func (g *botGrid) FindPath(sx, sy, gx, gy float64) []shared.Vec2 {
	sc, sr := g.cellOf(sx, sy-1) // -1: standing on the ground, feet at sy
	gc, gr := g.cellOf(gx, gy-1)

	sc, sr = g.snapToStandable(sc, sr)
	gc, gr = g.snapToStandable(gc, gr)
	if sc < 0 || gc < 0 {
		return nil
	}
	if sc == gc && sr == gr {
		return nil
	}

	open := &astarHeap{}
	heap.Init(open)

	costs := make(map[astarKey]float64, 512)
	pmap := make(map[astarKey]*astarNode, 512)

	startKey := astarKey{sc, sr}
	startNode := &astarNode{col: sc, row: sr, g: 0}
	startNode.f = g.heuristic(sc, sr, gc, gr)
	heap.Push(open, startNode)
	costs[startKey] = 0

	const maxIter = 6000
	iter := 0

	for open.Len() > 0 && iter < maxIter {
		iter++
		cur := heap.Pop(open).(*astarNode)
		curKey := astarKey{cur.col, cur.row}

		if cur.col == gc && cur.row == gr {
			return g.reconstructPath(cur, pmap)
		}

		if best, ok := costs[curKey]; ok && cur.g > best+1e-9 {
			continue // stale entry
		}

		for _, nb := range g.neighbors(cur.col, cur.row) {
			nc, nr := nb[0], nb[1]
			nbKey := astarKey{nc, nr}
			moveCost := g.moveCost(cur.col, cur.row, nc, nr)
			newG := cur.g + moveCost
			if prev, ok := costs[nbKey]; ok && prev <= newG {
				continue
			}
			costs[nbKey] = newG
			node := &astarNode{col: nc, row: nr, g: newG}
			node.f = newG + g.heuristic(nc, nr, gc, gr)
			heap.Push(open, node)
			pmap[nbKey] = cur
		}
	}
	return nil
}

func (g *botGrid) reconstructPath(goal *astarNode, pmap map[astarKey]*astarNode) []shared.Vec2 {
	// Trace back through pmap and collect cells.
	cells := make([]astarKey, 0, 32)
	key := astarKey{goal.col, goal.row}
	for {
		cells = append(cells, key)
		parent, ok := pmap[key]
		if !ok {
			break
		}
		pk := astarKey{parent.col, parent.row}
		if pk == key {
			break // safety
		}
		key = pk
	}
	// Reverse.
	for i, j := 0, len(cells)-1; i < j; i, j = i+1, j-1 {
		cells[i], cells[j] = cells[j], cells[i]
	}
	path := make([]shared.Vec2, 0, len(cells))
	for _, c := range cells {
		wx, wy := g.cellFeetWorld(c[0], c[1])
		path = append(path, shared.Vec2{X: wx, Y: wy})
	}
	return path
}

func (g *botGrid) heuristic(c1, r1, c2, r2 int) float64 {
	dc, dr := float64(c2-c1), float64(r2-r1)
	return math.Sqrt(dc*dc + dr*dr)
}

func (g *botGrid) moveCost(c1, r1, c2, r2 int) float64 {
	dc := math.Abs(float64(c2 - c1))
	dr := math.Abs(float64(r2 - r1))
	if dr > 0 {
		return math.Sqrt(dc*dc+dr*dr) * 2.0 // jumps and falls cost more
	}
	return dc
}

// neighbors returns all standable cells reachable from (col, row) in one move:
// walk left/right, fall off an edge, or jump to a higher platform.
func (g *botGrid) neighbors(col, row int) [][2]int {
	result := make([][2]int, 0, 16)

	// ── Walk L / R ────────────────────────────────────────────────────────────
	for _, dc := range [2]int{-1, 1} {
		nc := col + dc
		if g.standable(nc, row) {
			result = append(result, [2]int{nc, row})
		}
		// Walk then fall: step sideways and drop to the next floor below.
		for dropR := 1; dropR <= astarDropDY; dropR++ {
			nr := row + dropR
			if g.isSolid(nc, nr) {
				break // blocked by ceiling while falling
			}
			if g.standable(nc, nr) {
				// Check the fall column is clear of solids.
				ok := true
				for dr := 1; dr < dropR; dr++ {
					if g.isSolid(nc, row+dr) {
						ok = false
						break
					}
				}
				if ok {
					result = append(result, [2]int{nc, nr})
				}
				break
			}
		}
	}

	// ── Fall straight down ────────────────────────────────────────────────────
	for dr := 1; dr <= astarDropDY; dr++ {
		nr := row + dr
		if g.isSolid(col, nr) {
			break
		}
		if g.standable(col, nr) {
			result = append(result, [2]int{col, nr})
			break
		}
	}

	// ── Jump ─────────────────────────────────────────────────────────────────
	// Reach platforms that are higher (lower row number) and within physics range.
	for dy := -astarJumpH; dy <= -1; dy++ {
		for dx := -astarJumpDX; dx <= astarJumpDX; dx++ {
			// Elliptic feasibility filter: (dy/JH)² + (dx/JDX)² <= 1
			dyn := float64(dy) / float64(astarJumpH)
			dxn := float64(dx) / float64(astarJumpDX)
			if dyn*dyn+dxn*dxn > 1.0 {
				continue
			}
			nc, nr := col+dx, row+dy
			if !g.standable(nc, nr) {
				continue
			}
			if g.jumpArcClear(col, row, nc, nr) {
				result = append(result, [2]int{nc, nr})
			}
		}
	}

	return result
}

// jumpArcClear checks whether a parabolic arc from (c1,r1) to (c2,r2) clears
// all solid cells (body-width awareness: checks astarBW columns).
func (g *botGrid) jumpArcClear(c1, r1, c2, r2 int) bool {
	dx, dy := c2-c1, r2-r1
	steps := (iabs(dx) + iabs(dy)) * 3
	if steps < 6 {
		steps = 6
	}
	peakH := -dy // positive = jumping upward
	if peakH < 0 {
		peakH = 0
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		col := c1 + int(math.Round(float64(dx)*t))
		row := r1 + int(math.Round(float64(dy)*t))
		arc := int(math.Round(float64(peakH) * 4.0 * t * (1.0 - t)))
		row -= arc
		for dc := -astarBHalf; dc <= astarBHalf; dc++ {
			for dr := -(astarBH - 1); dr <= 0; dr++ {
				if g.isSolid(col+dc, row+dr) {
					return false
				}
			}
		}
	}
	return true
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
