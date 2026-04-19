package content

// nodeassembler.go — assembles room locations from node definitions.
//
// Each location is 800 blocks wide × 200 blocks tall.
// Nodes are placed left-to-right, their X coordinates offset by cumulative
// width.  The assembler filters by biome tag and picks compatible nodes whose
// socket heights are reachable from the previous node's right socket.
//
// Jump physics constants (matching procgen.go):
//   pgJumpUp   = 16 blocks  — max upward jump
//   pgDropDown = 28 blocks  — max safe downward drop

import (
	"math/rand"
	"sort"

	"warpedrealms/world"
)

const (
	naTargetW = 800  // target location width in blocks
	naGridH   = 200  // location height in blocks
	naGridSz  = 16   // pixels per block

	naJumpUp   = 16 // max upward reach in blocks
	naDropDown = 28 // max safe drop in blocks
)

// AssembleLocation builds one location (world.LDtkWriteLevel) from the node
// catalogue by picking and placing compatible nodes until the width is filled.
//
// biome is one of "ruins", "crystal", "forest".
// id is the LDtk level identifier (e.g. "room_01").
func AssembleLocation(rng *rand.Rand, nodes []NodeDef, biome, id string) world.LDtkWriteLevel {
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
	// Also include connector nodes for all biomes.
	for _, n := range nodes {
		isConnector := false
		for _, t := range n.Tags {
			if t == "connector" {
				isConnector = true
				break
			}
		}
		if !isConnector {
			continue
		}
		// Avoid duplicate if already in pool.
		already := false
		for _, p := range pool {
			if p.ID == n.ID {
				already = true
				break
			}
		}
		if !already {
			pool = append(pool, n)
		}
	}
	if len(pool) == 0 {
		pool = nodes // fallback: use all nodes
	}

	// ── select start node (tagged "start") ───────────────────────────────────
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

	// ── grow sequence until width is met ─────────────────────────────────────
	for totalW < naTargetW {
		prev := sequence[len(sequence)-1]
		remaining := naTargetW - totalW

		// Build candidate list: nodes compatible with prev's right socket.
		var candidates []NodeDef
		for _, n := range pool {
			if n.ID == prev.ID {
				continue // avoid direct repeat
			}
			if n.Width > remaining+80 {
				continue // too wide (allow a little overshoot)
			}
			dy := n.LeftSocket.SurfaceY - prev.RightSocket.SurfaceY
			// dy < 0 → n is higher than prev → need upward jump.
			// dy > 0 → n is lower → drop.
			if dy < -naJumpUp || dy > naDropDown {
				continue // unreachable
			}
			candidates = append(candidates, n)
		}

		if len(candidates) == 0 {
			// No compatible node — append flat connector and try again.
			connector := naPickConnector(pool, prev.RightSocket.SurfaceY)
			sequence = append(sequence, connector)
			totalW += connector.Width
			continue
		}

		// Prefer nodes that bring width closer to target.
		sort.Slice(candidates, func(i, j int) bool {
			di := abs(candidates[i].Width - (naTargetW - totalW))
			dj := abs(candidates[j].Width - (naTargetW - totalW))
			return di < dj
		})
		// Pick randomly from best third.
		top := len(candidates) / 3
		if top < 1 {
			top = 1
		}
		chosen := candidates[rng.Intn(top)]
		sequence = append(sequence, chosen)
		totalW += chosen.Width
	}

	// ── assemble solid cells and entities into a single level ────────────────
	var solidCells [][2]int
	var entities []world.LDtkWriteEntity

	offsetX := 0
	playerPlaced := false

	for ni, node := range sequence {
		for _, isl := range node.Islands {
			for dy := 0; dy < isl.H; dy++ {
				for dx := 0; dx < isl.W; dx++ {
					solidCells = append(solidCells, [2]int{
						offsetX + isl.X + dx,
						isl.Y + dy,
					})
				}
			}
		}

		for _, ent := range node.Entities {
			px := (offsetX + ent.X) * naGridSz
			py := ent.Y * naGridSz
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

	// Ensure at least one player spawn.
	if !playerPlaced {
		first := sequence[0]
		spawnX := first.Width / 2
		spawnY := first.LeftSocket.SurfaceY - 6
		if spawnY < 2 {
			spawnY = 2
		}
		entities = append(entities, world.LDtkWriteEntity{
			Identifier: "Player",
			PX:         spawnX * naGridSz,
			PY:         spawnY * naGridSz,
			W:          26,
			H:          76,
		})
	}

	// Total actual width (may overshoot slightly).
	actualW := offsetX
	if actualW < naTargetW {
		actualW = naTargetW
	}

	return world.LDtkWriteLevel{
		ID:         id,
		GridW:      actualW,
		GridH:      naGridH,
		GridSize:   naGridSz,
		SolidCells: solidCells,
		Entities:   entities,
	}
}

// naPickConnector finds the connector node whose left socket is closest in
// height to prevSurface.
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
