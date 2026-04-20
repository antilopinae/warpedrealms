package content

// nodelib.go — pre-authored room node definitions.
//
// Node coordinate space: H = 200 blocks (naNodeH), width varies 60–150 blocks.
// The assembler scales Y to the target gridH (default 300) proportionally:
//   actualY = nodeY × gridH / naNodeH
//
// Player physics (measured in node-space units, scale = gridH/naNodeH = 1.5):
//   Max jump height = v²/(2g) = 735²/(2×2050) ≈ 132 px ÷ (1.5 × 16) ≈ 5.5 units
//   Player collider = 24 × 72 px = 1.5 × 4.5 blocks
//
// DESIGN RULES enforced in every node:
//   • Adjacent platforms in a traversal chain: ΔY ≤ 5 node-units (one jump).
//   • All platforms reachable from the node's entry socket via ≤5-unit hops.
//   • No thin tall vertical walls (kind="trunk") — horizontal surfaces only.
//   • Socket SurfaceY: Y of the platform surface the player stands on at entry/exit.
//
// Platform bands used throughout (in node-space Y, top=0, bottom=200):
//   GND  = 188   (ground level)
//   L1   = 183   (+5 above GND, one jump)
//   L2   = 178   (+5 above L1)
//   L3   = 173   (+5 above L2)
//   L4   = 168   (+5 above L3)
//   L5   = 163   (+5 above L4)
//   L6   = 158   (+5 above L5)
//   L7   = 153   (+5 above L6, secret zone)
//   L8   = 148   (+5 above L7, deep secret)

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ─── Types ────────────────────────────────────────────────────────────────────

// NodeIsland is a solid rectangular block area within a node (block coords).
type NodeIsland struct {
	X, Y, W, H int
	Kind        string // "main" | "branch" | "step" | "secret"
}

// NodeSocket describes the passable edge of a node side.
// SurfaceY is the Y-block (from top) where the player stands at that edge.
type NodeSocket struct {
	SurfaceY int    // block row of the surface the player stands on at this edge
	Type     string // "open" | "platform" | "high"
}

// NodeEntity is a pre-placed entity within a node.
type NodeEntity struct {
	Type string // "Player" | "Rat"
	X, Y int    // block coords within node
}

// NodeDef is the complete definition of one room node.
type NodeDef struct {
	ID          string       `json:"id"`
	Width       int          `json:"width"`  // blocks (node-space)
	Height      int          `json:"height"` // always 200 (node-space; assembler scales to target gridH)
	Tags        []string     `json:"tags"`   // biomes + roles, e.g. ["ruins","start"]
	LeftSocket  NodeSocket   `json:"leftSocket"`
	RightSocket NodeSocket   `json:"rightSocket"`
	Islands     []NodeIsland `json:"islands"`
	Entities    []NodeEntity `json:"entities"`
}

// ─── Node catalogue ───────────────────────────────────────────────────────────

// AllNodes returns the complete built-in node catalogue.
// These are written to gamedata/nodes/ by EnsureNodes().
func AllNodes() []NodeDef {
	const H = 200 // node-space height — DO NOT CHANGE (assembler scales to gridH)

	// Platform bands (node-space Y, ΔY=5 between bands = one jump = ~120 px actual)
	const (
		GND = 188 // ground
		L1  = 183 // 1 jump above ground
		L2  = 178 // 2 jumps
		L3  = 173 // 3 jumps
		L4  = 168 // 4 jumps
		L5  = 163 // 5 jumps
		L6  = 158 // 6 jumps
		L7  = 153 // 7 jumps (secret zone)
		L8  = 148 // 8 jumps (deep secret)
	)

	return []NodeDef{

		// ══════════════════════════════════════════════════════════════════════
		// RUINS BIOME
		// All platforms form unbroken chains with ΔY=5 between each rung.
		// Max platform H = 3 blocks.
		// ══════════════════════════════════════════════════════════════════════

		{
			// ruins_entry — wide entry, zigzag ascent GND→L5 (secret).
			// Chain: GND(188) → L1(183) → L2(178) → L3(173) → L4(168) → L5(163)
			ID: "ruins_entry", Width: 120, Height: H,
			Tags:        []string{"ruins", "start"},
			LeftSocket:  NodeSocket{SurfaceY: GND, Type: "open"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: GND, W: 48, H: 3, Kind: "main"},   // ground left
				{X: 85, Y: L1, W: 33, H: 3, Kind: "main"},   // right exit (L1)
				{X: 52, Y: L1, W: 22, H: 2, Kind: "step"},   // centre step L1
				{X: 8, Y: L2, W: 28, H: 2, Kind: "branch"},  // left branch L2
				{X: 80, Y: L2, W: 28, H: 2, Kind: "branch"}, // right branch L2
				{X: 42, Y: L3, W: 30, H: 2, Kind: "branch"}, // centre L3
				{X: 5, Y: L4, W: 25, H: 2, Kind: "branch"},  // left L4
				{X: 88, Y: L4, W: 25, H: 2, Kind: "branch"}, // right L4
				{X: 38, Y: L5, W: 40, H: 2, Kind: "secret"}, // secret L5 (5 above L4)
			},
			Entities: []NodeEntity{
				{Type: "Player", X: 22, Y: GND},
				{Type: "Rat", X: 60, Y: L1},
			},
		},

		{
			// ruins_arch — arch shape, continuous chain L1→L2→L3→L4→L5→L6(secret).
			ID: "ruins_arch", Width: 100, Height: H,
			Tags:        []string{"ruins"},
			LeftSocket:  NodeSocket{SurfaceY: L1, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L1, W: 30, H: 3, Kind: "main"},    // left floor
				{X: 68, Y: L1, W: 30, H: 3, Kind: "main"},   // right floor
				{X: 35, Y: L2, W: 28, H: 2, Kind: "step"},   // centre bridge L2
				{X: 8, Y: L3, W: 20, H: 2, Kind: "branch"},  // left L3
				{X: 72, Y: L3, W: 20, H: 2, Kind: "branch"}, // right L3
				{X: 35, Y: L4, W: 28, H: 2, Kind: "branch"}, // keystone L4
				{X: 5, Y: L5, W: 22, H: 2, Kind: "branch"},  // left high L5
				{X: 73, Y: L5, W: 22, H: 2, Kind: "branch"}, // right high L5
				{X: 30, Y: L6, W: 40, H: 2, Kind: "secret"}, // crown L6 (5 above L5)
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 50, Y: L2},
				{Type: "Rat", X: 78, Y: L1},
			},
		},

		{
			// ruins_tower — strict zigzag GND→L1→…→L6→L7(secret), no gaps.
			ID: "ruins_tower", Width: 80, Height: H,
			Tags:        []string{"ruins"},
			LeftSocket:  NodeSocket{SurfaceY: GND, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: GND, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: GND, W: 30, H: 3, Kind: "main"},    // ground left
				{X: 48, Y: GND, W: 30, H: 3, Kind: "main"},   // ground right
				{X: 5, Y: L1, W: 28, H: 2, Kind: "step"},     // L1 left
				{X: 47, Y: L2, W: 28, H: 2, Kind: "step"},    // L2 right
				{X: 5, Y: L3, W: 28, H: 2, Kind: "step"},     // L3 left
				{X: 47, Y: L4, W: 28, H: 2, Kind: "step"},    // L4 right
				{X: 5, Y: L5, W: 28, H: 2, Kind: "step"},     // L5 left
				{X: 47, Y: L6, W: 28, H: 2, Kind: "branch"},  // L6 right
				{X: 18, Y: L7, W: 42, H: 3, Kind: "secret"},  // tower crown L7
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 15, Y: GND},
				{Type: "Rat", X: 55, Y: GND},
			},
		},

		{
			// ruins_hall — wide hall, full chain L1→L2→L3→L4→L5→L6(secret).
			// GND dip in centre (drop from L1 = 5 units, one safe drop).
			ID: "ruins_hall", Width: 150, Height: H,
			Tags:        []string{"ruins"},
			LeftSocket:  NodeSocket{SurfaceY: L1, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L1, W: 48, H: 3, Kind: "main"},     // left floor
				{X: 100, Y: L1, W: 48, H: 3, Kind: "main"},   // right floor
				{X: 55, Y: GND, W: 40, H: 3, Kind: "main"},   // hall floor dip (GND, 5 below L1)
				{X: 5, Y: L2, W: 38, H: 2, Kind: "branch"},   // balcony left L2
				{X: 107, Y: L2, W: 38, H: 2, Kind: "branch"}, // balcony right L2
				{X: 55, Y: L2, W: 38, H: 2, Kind: "branch"},  // mid balcony L2
				{X: 8, Y: L3, W: 32, H: 2, Kind: "branch"},   // upper left L3
				{X: 58, Y: L3, W: 32, H: 2, Kind: "branch"},  // upper mid L3
				{X: 110, Y: L3, W: 32, H: 2, Kind: "branch"}, // upper right L3
				{X: 35, Y: L4, W: 30, H: 2, Kind: "step"},    // bridge step L4
				{X: 90, Y: L4, W: 30, H: 2, Kind: "step"},    // bridge step L4
				{X: 35, Y: L5, W: 78, H: 2, Kind: "branch"},  // grand bridge L5
				{X: 50, Y: L6, W: 52, H: 2, Kind: "secret"},  // secret L6 (5 above L5)
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 25, Y: L1},
				{Type: "Rat", X: 118, Y: L1},
				{Type: "Rat", X: 65, Y: GND},
			},
		},

		// ══════════════════════════════════════════════════════════════════════
		// CRYSTAL BIOME
		// ══════════════════════════════════════════════════════════════════════

		{
			// crystal_entry — ascending from GND, dual paths merging at L5(secret).
			// Main path: GND→L1→L2. Side path: L2→L3→L4→L5.
			ID: "crystal_entry", Width: 100, Height: H,
			Tags:        []string{"crystal", "start"},
			LeftSocket:  NodeSocket{SurfaceY: GND, Type: "open"},
			RightSocket: NodeSocket{SurfaceY: L2, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: GND, W: 28, H: 3, Kind: "main"},   // anchor left GND
				{X: 32, Y: L1, W: 12, H: 2, Kind: "main"},   // crystal step L1
				{X: 46, Y: L2, W: 12, H: 2, Kind: "main"},   // crystal step L2
				{X: 72, Y: L2, W: 26, H: 3, Kind: "main"},   // right exit L2
				{X: 8, Y: L2, W: 12, H: 1, Kind: "step"},    // side L2
				{X: 22, Y: L3, W: 12, H: 1, Kind: "step"},   // side L3
				{X: 36, Y: L4, W: 12, H: 1, Kind: "step"},   // side L4
				{X: 52, Y: L4, W: 12, H: 1, Kind: "step"},   // side L4 (bridge)
				{X: 68, Y: L4, W: 14, H: 2, Kind: "branch"}, // side L4 shelf
				{X: 20, Y: L5, W: 28, H: 2, Kind: "branch"}, // high left L5
				{X: 60, Y: L5, W: 28, H: 2, Kind: "branch"}, // high right L5
				{X: 30, Y: L6, W: 40, H: 2, Kind: "secret"}, // secret L6 (5 above L5)
			},
			Entities: []NodeEntity{
				{Type: "Player", X: 12, Y: GND},
				{Type: "Rat", X: 48, Y: L2},
			},
		},

		{
			// crystal_spires — two spires, chain L2→L3→L4→L5→L6→L7(secret).
			ID: "crystal_spires", Width: 120, Height: H,
			Tags:        []string{"crystal"},
			LeftSocket:  NodeSocket{SurfaceY: L2, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L2, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L2, W: 20, H: 3, Kind: "main"},    // left anchor L2
				{X: 98, Y: L2, W: 20, H: 3, Kind: "main"},   // right anchor L2
				{X: 50, Y: L3, W: 20, H: 2, Kind: "main"},   // centre shelf L3
				{X: 22, Y: L3, W: 10, H: 1, Kind: "step"},   // left chain L3
				{X: 88, Y: L3, W: 10, H: 1, Kind: "step"},   // right chain L3
				{X: 33, Y: L4, W: 10, H: 1, Kind: "step"},   // L4
				{X: 76, Y: L4, W: 10, H: 1, Kind: "step"},   // L4
				{X: 44, Y: L5, W: 10, H: 1, Kind: "step"},   // L5
				{X: 64, Y: L5, W: 10, H: 1, Kind: "step"},   // L5
				{X: 42, Y: L5, W: 36, H: 2, Kind: "branch"}, // top crystal L5
				{X: 15, Y: L6, W: 30, H: 2, Kind: "branch"}, // high left L6
				{X: 75, Y: L6, W: 30, H: 2, Kind: "branch"}, // high right L6
				{X: 40, Y: L7, W: 40, H: 2, Kind: "secret"}, // secret L7 (5 above L6)
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 55, Y: L3},
				{Type: "Rat", X: 22, Y: L6},
			},
		},

		{
			// crystal_cave — dense grotto, chain GND→L1→L2→L3→L4→L5→L6(secret).
			ID: "crystal_cave", Width: 90, Height: H,
			Tags:        []string{"crystal"},
			LeftSocket:  NodeSocket{SurfaceY: GND, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: GND, W: 25, H: 3, Kind: "main"},   // floor left GND
				{X: 63, Y: L1, W: 25, H: 3, Kind: "main"},   // floor right L1
				{X: 28, Y: L1, W: 10, H: 2, Kind: "main"},   // crystal mid L1
				{X: 40, Y: L2, W: 10, H: 2, Kind: "main"},   // crystal L2
				{X: 52, Y: L1, W: 10, H: 2, Kind: "main"},   // crystal L1
				{X: 5, Y: L2, W: 14, H: 2, Kind: "step"},    // left chain L2
				{X: 71, Y: L2, W: 14, H: 2, Kind: "step"},   // right chain L2
				{X: 5, Y: L3, W: 14, H: 2, Kind: "step"},    // L3
				{X: 71, Y: L3, W: 14, H: 2, Kind: "step"},   // L3
				{X: 10, Y: L4, W: 26, H: 2, Kind: "branch"}, // upper left L4
				{X: 54, Y: L4, W: 26, H: 2, Kind: "branch"}, // upper right L4
				{X: 22, Y: L5, W: 46, H: 2, Kind: "branch"}, // crystal crown L5
				{X: 28, Y: L6, W: 34, H: 2, Kind: "secret"}, // secret L6 (5 above L5)
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 38, Y: L1},
				{Type: "Rat", X: 38, Y: L3},
			},
		},

		{
			// crystal_peak — rising peak, full chain GND/L1→L2→L3→L4→L5→L6(secret).
			ID: "crystal_peak", Width: 100, Height: H,
			Tags:        []string{"crystal"},
			LeftSocket:  NodeSocket{SurfaceY: L1, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L1, W: 24, H: 3, Kind: "main"},    // left base L1
				{X: 74, Y: L1, W: 24, H: 3, Kind: "main"},   // right base L1
				{X: 36, Y: GND, W: 28, H: 3, Kind: "main"},  // peak base GND (5 below L1)
				{X: 40, Y: L2, W: 20, H: 2, Kind: "main"},   // peak L2
				{X: 42, Y: L3, W: 16, H: 2, Kind: "branch"}, // peak L3
				{X: 44, Y: L4, W: 12, H: 2, Kind: "branch"}, // peak L4
				{X: 46, Y: L5, W: 8, H: 2, Kind: "branch"},  // peak L5
				{X: 18, Y: L2, W: 14, H: 2, Kind: "step"},   // side access L2
				{X: 68, Y: L2, W: 14, H: 2, Kind: "step"},   // side access L2
				{X: 22, Y: L3, W: 14, H: 2, Kind: "step"},   // side L3
				{X: 64, Y: L3, W: 14, H: 2, Kind: "step"},   // side L3
				{X: 5, Y: L4, W: 22, H: 2, Kind: "branch"},  // side high L4
				{X: 73, Y: L4, W: 22, H: 2, Kind: "branch"}, // side high L4
				{X: 36, Y: L6, W: 28, H: 2, Kind: "secret"}, // peak secret L6 (5 above L5)
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 47, Y: L2},
				{Type: "Rat", X: 47, Y: L4},
			},
		},

		// ══════════════════════════════════════════════════════════════════════
		// FOREST BIOME
		// ══════════════════════════════════════════════════════════════════════

		{
			// forest_canopy — layered canopy, chain GND→L1→L2→L3→L4→L5→L6(secret).
			ID: "forest_canopy", Width: 120, Height: H,
			Tags:        []string{"forest", "start"},
			LeftSocket:  NodeSocket{SurfaceY: GND, Type: "open"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: GND, W: 36, H: 3, Kind: "main"},   // ground left
				{X: 85, Y: L1, W: 33, H: 3, Kind: "main"},   // ground right L1
				{X: 4, Y: L1, W: 22, H: 2, Kind: "main"},    // canopy L1
				{X: 40, Y: L1, W: 24, H: 2, Kind: "main"},   // canopy L1
				{X: 14, Y: L2, W: 20, H: 2, Kind: "main"},   // canopy L2
				{X: 54, Y: L2, W: 22, H: 2, Kind: "main"},   // canopy L2
				{X: 92, Y: L2, W: 22, H: 2, Kind: "main"},   // canopy L2
				{X: 30, Y: L2, W: 10, H: 2, Kind: "step"},   // bridge step L2
				{X: 72, Y: L2, W: 10, H: 2, Kind: "step"},   // bridge step L2
				{X: 6, Y: L3, W: 18, H: 2, Kind: "branch"},  // canopy L3
				{X: 44, Y: L3, W: 20, H: 2, Kind: "branch"}, // canopy L3
				{X: 88, Y: L3, W: 18, H: 2, Kind: "branch"}, // canopy L3
				{X: 24, Y: L4, W: 20, H: 2, Kind: "step"},   // step L4 (bridges L3→L5)
				{X: 72, Y: L4, W: 20, H: 2, Kind: "step"},   // step L4
				{X: 25, Y: L5, W: 68, H: 2, Kind: "branch"}, // high canopy L5
				{X: 40, Y: L6, W: 42, H: 2, Kind: "secret"}, // sky secret L6 (5 above L5)
			},
			Entities: []NodeEntity{
				{Type: "Player", X: 16, Y: GND},
				{Type: "Rat", X: 50, Y: L1},
				{Type: "Rat", X: 95, Y: L2},
			},
		},

		{
			// forest_trunk — climbing tree, zigzag L1→L2→L3→L4→L5→L6→L7(secret).
			ID: "forest_trunk", Width: 80, Height: H,
			Tags:        []string{"forest"},
			LeftSocket:  NodeSocket{SurfaceY: L1, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L1, W: 24, H: 3, Kind: "main"},    // base left L1
				{X: 54, Y: L1, W: 24, H: 3, Kind: "main"},   // base right L1
				{X: 26, Y: GND, W: 10, H: 2, Kind: "step"},  // moss step GND (5 below L1)
				{X: 44, Y: GND, W: 10, H: 2, Kind: "step"},  // moss step GND
				{X: 8, Y: L2, W: 28, H: 2, Kind: "branch"},  // branch L2
				{X: 44, Y: L3, W: 28, H: 2, Kind: "branch"}, // branch L3
				{X: 8, Y: L4, W: 28, H: 2, Kind: "branch"},  // branch L4
				{X: 44, Y: L5, W: 28, H: 2, Kind: "branch"}, // branch L5
				{X: 8, Y: L6, W: 28, H: 2, Kind: "branch"},  // branch L6
				{X: 12, Y: L7, W: 56, H: 3, Kind: "secret"}, // crown L7 (5 above L6)
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 20, Y: L2},
				{Type: "Rat", X: 52, Y: L3},
			},
		},

		{
			// forest_glade — open glade, chain L1/GND→L1→L2→L3→L4→L5→L6(secret).
			ID: "forest_glade", Width: 110, Height: H,
			Tags:        []string{"forest"},
			LeftSocket:  NodeSocket{SurfaceY: L1, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L1, W: 28, H: 3, Kind: "main"},    // left patch L1
				{X: 42, Y: GND, W: 25, H: 3, Kind: "main"},  // centre dip GND (5 below L1)
				{X: 80, Y: L1, W: 28, H: 3, Kind: "main"},   // right patch L1
				{X: 32, Y: L1, W: 8, H: 2, Kind: "step"},    // bridge step L1
				{X: 70, Y: L1, W: 8, H: 2, Kind: "step"},    // bridge step L1
				{X: 8, Y: L2, W: 28, H: 2, Kind: "branch"},  // mid-air L2
				{X: 50, Y: L2, W: 22, H: 2, Kind: "branch"}, // mid-air L2
				{X: 82, Y: L2, W: 22, H: 2, Kind: "branch"}, // mid-air L2
				{X: 15, Y: L3, W: 25, H: 2, Kind: "branch"}, // L3
				{X: 55, Y: L3, W: 25, H: 2, Kind: "branch"}, // L3
				{X: 85, Y: L3, W: 18, H: 2, Kind: "branch"}, // L3
				{X: 30, Y: L4, W: 22, H: 2, Kind: "step"},   // step L4 (bridges L3→L5)
				{X: 60, Y: L4, W: 22, H: 2, Kind: "step"},   // step L4
				{X: 25, Y: L5, W: 58, H: 2, Kind: "branch"}, // canopy top L5
				{X: 32, Y: L6, W: 48, H: 2, Kind: "secret"}, // sky secret L6 (5 above L5)
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 52, Y: GND},
				{Type: "Rat", X: 55, Y: L2},
			},
		},

		{
			// forest_aerial — floating islands, spiral L1→L2→L3→L4→L5→L6→L7(secret).
			ID: "forest_aerial", Width: 100, Height: H,
			Tags:        []string{"forest"},
			LeftSocket:  NodeSocket{SurfaceY: L1, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L2, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L1, W: 22, H: 2, Kind: "main"},    // entry L1
				{X: 76, Y: L2, W: 22, H: 2, Kind: "main"},   // exit L2
				{X: 38, Y: L2, W: 24, H: 2, Kind: "main"},   // central L2
				{X: 10, Y: L2, W: 18, H: 2, Kind: "step"},   // spiral L2
				{X: 62, Y: L3, W: 18, H: 2, Kind: "step"},   // L3
				{X: 22, Y: L4, W: 18, H: 2, Kind: "step"},   // L4
				{X: 58, Y: L5, W: 18, H: 2, Kind: "step"},   // L5
				{X: 14, Y: L6, W: 18, H: 2, Kind: "step"},   // L6
				{X: 62, Y: L6, W: 18, H: 2, Kind: "step"},   // L6
				{X: 28, Y: L7, W: 42, H: 2, Kind: "secret"}, // sky secret L7 (5 above L6)
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 46, Y: L2},
			},
		},

		// ══════════════════════════════════════════════════════════════════════
		// UNIVERSAL CONNECTORS
		// ══════════════════════════════════════════════════════════════════════

		{
			// connector_flat — level bridge, full chain L1→L2→L3→L4→L5→L6(secret).
			ID: "connector_flat", Width: 60, Height: H,
			Tags:        []string{"ruins", "crystal", "forest", "connector"},
			LeftSocket:  NodeSocket{SurfaceY: L1, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L1, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L1, W: 56, H: 3, Kind: "main"},   // bridge deck L1
				{X: 8, Y: L2, W: 18, H: 2, Kind: "step"},   // step L2
				{X: 34, Y: L2, W: 18, H: 2, Kind: "step"},  // step L2
				{X: 16, Y: L3, W: 26, H: 2, Kind: "branch"}, // shelf L3
				{X: 4, Y: L4, W: 20, H: 2, Kind: "step"},   // step L4
				{X: 36, Y: L4, W: 20, H: 2, Kind: "step"},  // step L4
				{X: 10, Y: L5, W: 40, H: 2, Kind: "branch"}, // high shelf L5
				{X: 14, Y: L6, W: 32, H: 2, Kind: "secret"}, // secret L6 (5 above L5)
			},
			Entities: []NodeEntity{},
		},

		{
			// connector_rise — GND→L1(step)→L2(exit), side chain up to L5(secret).
			ID: "connector_rise", Width: 65, Height: H,
			Tags:        []string{"ruins", "crystal", "forest", "connector"},
			LeftSocket:  NodeSocket{SurfaceY: GND, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: L2, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: GND, W: 20, H: 3, Kind: "main"},   // entry GND
				{X: 22, Y: L1, W: 18, H: 2, Kind: "step"},   // mid step L1
				{X: 42, Y: L2, W: 21, H: 3, Kind: "main"},   // exit L2
				{X: 5, Y: L3, W: 20, H: 2, Kind: "branch"},  // side L3
				{X: 38, Y: L3, W: 20, H: 2, Kind: "branch"}, // side L3
				{X: 12, Y: L4, W: 16, H: 2, Kind: "step"},   // step L4
				{X: 36, Y: L4, W: 16, H: 2, Kind: "step"},   // step L4
				{X: 10, Y: L5, W: 44, H: 2, Kind: "branch"}, // top shelf L5
				{X: 18, Y: L6, W: 28, H: 2, Kind: "secret"}, // secret L6 (5 above L5)
			},
			Entities: []NodeEntity{},
		},

		{
			// connector_fall — L2(entry)→L1(step)→GND(exit), side chain up to L5(secret).
			ID: "connector_fall", Width: 65, Height: H,
			Tags:        []string{"ruins", "crystal", "forest", "connector"},
			LeftSocket:  NodeSocket{SurfaceY: L2, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: GND, Type: "platform"},
			Islands: []NodeIsland{
				{X: 2, Y: L2, W: 20, H: 3, Kind: "main"},    // entry L2
				{X: 24, Y: L1, W: 16, H: 2, Kind: "step"},   // mid step L1
				{X: 43, Y: GND, W: 20, H: 3, Kind: "main"},  // exit GND
				{X: 5, Y: L3, W: 20, H: 2, Kind: "branch"},  // side L3
				{X: 38, Y: L3, W: 20, H: 2, Kind: "branch"}, // side L3
				{X: 12, Y: L4, W: 16, H: 2, Kind: "step"},   // step L4
				{X: 36, Y: L4, W: 16, H: 2, Kind: "step"},   // step L4
				{X: 12, Y: L5, W: 42, H: 2, Kind: "branch"}, // upper shelf L5
				{X: 18, Y: L6, W: 30, H: 2, Kind: "secret"}, // secret L6 (5 above L5)
			},
			Entities: []NodeEntity{},
		},
	}
}

// ─── Node file I/O ────────────────────────────────────────────────────────────

// EnsureNodes writes all built-in node definitions to dir as JSON files.
// Existing files are overwritten so they stay in sync with the code.
func EnsureNodes(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("nodelib mkdir %s: %w", dir, err)
	}
	for _, node := range AllNodes() {
		data, err := json.MarshalIndent(node, "", "  ")
		if err != nil {
			return fmt.Errorf("nodelib marshal %s: %w", node.ID, err)
		}
		path := filepath.Join(dir, node.ID+".json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("nodelib write %s: %w", path, err)
		}
	}
	return nil
}

// LoadNodes reads all *.json node files from dir and returns them.
func LoadNodes(dir string) ([]NodeDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("loadnodes readdir %s: %w", dir, err)
	}
	var nodes []NodeDef
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("loadnodes read %s: %w", e.Name(), err)
		}
		var n NodeDef
		if err := json.Unmarshal(data, &n); err != nil {
			return nil, fmt.Errorf("loadnodes parse %s: %w", e.Name(), err)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}
