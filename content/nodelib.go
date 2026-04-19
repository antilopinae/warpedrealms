package content

// nodelib.go — pre-authored room node definitions.
//
// Each node is a rectangular chunk 200 blocks tall (full level height) and
// 60–150 blocks wide.  Nodes are assembled side-by-side to fill a 800-block-
// wide location.  Socket heights tell the assembler where the player stands
// when entering/leaving the node from the left or right.
//
// All coordinates are in blocks relative to the node's own top-left corner.
// Block size = 16 px.  Y increases downward; level top = 0, bottom = 200.
//
// "Aerial Dead Cells" style: no continuous floor, lots of open air, many
// floating islands at varied heights, all reachable via generous jumps.

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
	Kind        string // "main" | "branch" | "step" | "trunk" | "secret"
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
	Width       int          `json:"width"`  // blocks
	Height      int          `json:"height"` // always 200
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
	const H = 200 // level height in blocks

	return []NodeDef{

		// ══════════════════════════════════════════════════════════════════════
		// RUINS BIOME
		// Rule: NO thin tall walls.  All solid islands are horizontal surfaces
		// (platforms the player can land on).  Vertical traversal is achieved
		// through chains of offset platforms, not through blocking columns.
		// ══════════════════════════════════════════════════════════════════════

		{
			ID: "ruins_entry", Width: 120, Height: H,
			Tags:        []string{"ruins", "start"},
			LeftSocket:  NodeSocket{SurfaceY: 158, Type: "open"},
			RightSocket: NodeSocket{SurfaceY: 152, Type: "platform"},
			Islands: []NodeIsland{
				// Big landing pad
				{X: 4, Y: 158, W: 48, H: 4, Kind: "main"},
				// Mid step
				{X: 55, Y: 144, W: 22, H: 3, Kind: "branch"},
				// Upper arch ledge
				{X: 40, Y: 120, W: 30, H: 2, Kind: "branch"},
				// Right exit pad
				{X: 88, Y: 152, W: 28, H: 3, Kind: "main"},
				// High platform
				{X: 50, Y: 78, W: 35, H: 2, Kind: "branch"},
				// Ceiling secret
				{X: 20, Y: 12, W: 40, H: 2, Kind: "secret"},
				// Mid left step
				{X: 15, Y: 105, W: 18, H: 2, Kind: "step"},
				// Mid right step
				{X: 78, Y: 98, W: 18, H: 2, Kind: "step"},
				// Upper secret
				{X: 70, Y: 45, W: 25, H: 2, Kind: "secret"},
			},
			Entities: []NodeEntity{
				{Type: "Player", X: 22, Y: 155},
				{Type: "Rat", X: 60, Y: 141},
			},
		},

		{
			ID: "ruins_arch", Width: 100, Height: H,
			Tags:        []string{"ruins"},
			LeftSocket:  NodeSocket{SurfaceY: 152, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 148, Type: "platform"},
			Islands: []NodeIsland{
				// Left floor
				{X: 3, Y: 152, W: 32, H: 3, Kind: "main"},
				// Right floor
				{X: 65, Y: 148, W: 32, H: 3, Kind: "main"},
				// Arch keystone (floating, no legs)
				{X: 30, Y: 112, W: 40, H: 2, Kind: "branch"},
				// Upper left
				{X: 10, Y: 80, W: 28, H: 2, Kind: "branch"},
				// Upper right
				{X: 65, Y: 75, W: 28, H: 2, Kind: "branch"},
				// Top secret
				{X: 30, Y: 45, W: 40, H: 2, Kind: "secret"},
				// Steps up left
				{X: 5, Y: 125, W: 14, H: 2, Kind: "step"},
				{X: 5, Y: 105, W: 14, H: 2, Kind: "step"},
				// Steps up right
				{X: 80, Y: 120, W: 14, H: 2, Kind: "step"},
				{X: 80, Y: 100, W: 14, H: 2, Kind: "step"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 45, Y: 108},
				{Type: "Rat", X: 75, Y: 145},
			},
		},

		{
			ID: "ruins_tower", Width: 80, Height: H,
			Tags:        []string{"ruins"},
			LeftSocket:  NodeSocket{SurfaceY: 162, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 162, Type: "platform"},
			Islands: []NodeIsland{
				// Base left
				{X: 3, Y: 162, W: 28, H: 3, Kind: "main"},
				// Base right
				{X: 49, Y: 162, W: 28, H: 3, Kind: "main"},
				// Zigzag climbing path (no blocking shaft)
				{X: 18, Y: 145, W: 18, H: 2, Kind: "step"},
				{X: 44, Y: 128, W: 18, H: 2, Kind: "step"},
				{X: 18, Y: 110, W: 18, H: 2, Kind: "step"},
				{X: 44, Y: 92, W: 18, H: 2, Kind: "step"},
				{X: 18, Y: 74, W: 18, H: 2, Kind: "step"},
				{X: 44, Y: 56, W: 18, H: 2, Kind: "step"},
				// Tower top
				{X: 20, Y: 38, W: 38, H: 3, Kind: "secret"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 20, Y: 159},
				{Type: "Rat", X: 58, Y: 159},
			},
		},

		{
			ID: "ruins_hall", Width: 150, Height: H,
			Tags:        []string{"ruins"},
			LeftSocket:  NodeSocket{SurfaceY: 155, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 150, Type: "platform"},
			Islands: []NodeIsland{
				// Grand floor (partial)
				{X: 3, Y: 155, W: 55, H: 4, Kind: "main"},
				{X: 95, Y: 150, W: 52, H: 4, Kind: "main"},
				// Grand hall middle gap bridged by mid platform
				{X: 58, Y: 165, W: 35, H: 3, Kind: "main"},
				// Balcony left
				{X: 5, Y: 120, W: 40, H: 3, Kind: "branch"},
				// Balcony right
				{X: 108, Y: 115, W: 40, H: 3, Kind: "branch"},
				// Middle balcony
				{X: 60, Y: 132, W: 35, H: 2, Kind: "branch"},
				// Upper bridge
				{X: 35, Y: 88, W: 80, H: 2, Kind: "branch"},
				// High platforms
				{X: 10, Y: 62, W: 35, H: 2, Kind: "branch"},
				{X: 110, Y: 58, W: 35, H: 2, Kind: "branch"},
				// Ceiling secret
				{X: 50, Y: 15, W: 55, H: 2, Kind: "secret"},
				// Stepping stones
				{X: 25, Y: 145, W: 12, H: 2, Kind: "step"},
				{X: 120, Y: 135, W: 12, H: 2, Kind: "step"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 25, Y: 152},
				{Type: "Rat", X: 120, Y: 147},
				{Type: "Rat", X: 65, Y: 162},
			},
		},

		// ══════════════════════════════════════════════════════════════════════
		// CRYSTAL BIOME
		// ══════════════════════════════════════════════════════════════════════

		{
			ID: "crystal_entry", Width: 100, Height: H,
			Tags:        []string{"crystal", "start"},
			LeftSocket:  NodeSocket{SurfaceY: 162, Type: "open"},
			RightSocket: NodeSocket{SurfaceY: 155, Type: "platform"},
			Islands: []NodeIsland{
				// Anchor left
				{X: 3, Y: 162, W: 30, H: 4, Kind: "main"},
				// Crystal cluster centre (ascending steps, no vertical walls)
				{X: 40, Y: 150, W: 12, H: 2, Kind: "main"},
				{X: 54, Y: 140, W: 10, H: 2, Kind: "main"},
				{X: 66, Y: 130, W: 10, H: 2, Kind: "main"},
				// Right exit
				{X: 78, Y: 155, W: 20, H: 3, Kind: "main"},
				// Spike field mid-air (thin horizontal pads only)
				{X: 20, Y: 120, W: 8, H: 1, Kind: "step"},
				{X: 35, Y: 108, W: 6, H: 1, Kind: "step"},
				{X: 50, Y: 97, W: 8, H: 1, Kind: "step"},
				{X: 65, Y: 86, W: 6, H: 1, Kind: "step"},
				{X: 78, Y: 75, W: 8, H: 1, Kind: "step"},
				// High crystal shelf
				{X: 15, Y: 65, W: 28, H: 2, Kind: "branch"},
				{X: 60, Y: 55, W: 28, H: 2, Kind: "branch"},
				// Top crystal
				{X: 30, Y: 18, W: 40, H: 2, Kind: "secret"},
			},
			Entities: []NodeEntity{
				{Type: "Player", X: 15, Y: 159},
				{Type: "Rat", X: 48, Y: 147},
			},
		},

		{
			ID: "crystal_spires", Width: 120, Height: H,
			Tags:        []string{"crystal"},
			LeftSocket:  NodeSocket{SurfaceY: 155, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 158, Type: "platform"},
			Islands: []NodeIsland{
				// Left anchor
				{X: 3, Y: 155, W: 22, H: 3, Kind: "main"},
				// Right anchor
				{X: 95, Y: 158, W: 22, H: 3, Kind: "main"},
				// Central floating crystal shelf
				{X: 50, Y: 140, W: 20, H: 2, Kind: "main"},
				// Diagonal ascending chain left
				{X: 15, Y: 138, W: 8, H: 1, Kind: "step"},
				{X: 26, Y: 125, W: 8, H: 1, Kind: "step"},
				{X: 37, Y: 112, W: 8, H: 1, Kind: "step"},
				{X: 48, Y: 99, W: 8, H: 1, Kind: "step"},
				// Diagonal ascending chain right
				{X: 75, Y: 142, W: 8, H: 1, Kind: "step"},
				{X: 85, Y: 128, W: 8, H: 1, Kind: "step"},
				{X: 95, Y: 115, W: 8, H: 1, Kind: "step"},
				// Mid air large crystal
				{X: 42, Y: 85, W: 36, H: 2, Kind: "branch"},
				// High shelf
				{X: 20, Y: 55, W: 30, H: 2, Kind: "branch"},
				{X: 75, Y: 50, W: 30, H: 2, Kind: "branch"},
				// Secret top
				{X: 40, Y: 15, W: 40, H: 2, Kind: "secret"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 50, Y: 137},
				{Type: "Rat", X: 60, Y: 47},
			},
		},

		{
			ID: "crystal_cave", Width: 90, Height: H,
			Tags:        []string{"crystal"},
			LeftSocket:  NodeSocket{SurfaceY: 158, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 152, Type: "platform"},
			Islands: []NodeIsland{
				// Floor chunk left
				{X: 3, Y: 158, W: 25, H: 4, Kind: "main"},
				// Floor chunk right
				{X: 65, Y: 152, W: 22, H: 4, Kind: "main"},
				// Dense crystal mid cluster (horizontal pads only, no pillars)
				{X: 30, Y: 148, W: 8, H: 2, Kind: "main"},
				{X: 42, Y: 138, W: 8, H: 2, Kind: "main"},
				{X: 30, Y: 128, W: 8, H: 2, Kind: "main"},
				{X: 50, Y: 118, W: 8, H: 2, Kind: "main"},
				{X: 35, Y: 108, W: 8, H: 2, Kind: "main"},
				// Upper level
				{X: 8, Y: 95, W: 25, H: 2, Kind: "branch"},
				{X: 58, Y: 88, W: 25, H: 2, Kind: "branch"},
				// Crystal crown
				{X: 20, Y: 58, W: 50, H: 2, Kind: "branch"},
				// Steps up side
				{X: 5, Y: 138, W: 10, H: 2, Kind: "step"},
				{X: 5, Y: 118, W: 10, H: 2, Kind: "step"},
				{X: 72, Y: 133, W: 10, H: 2, Kind: "step"},
				{X: 72, Y: 113, W: 10, H: 2, Kind: "step"},
				// Secret
				{X: 25, Y: 20, W: 40, H: 2, Kind: "secret"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 40, Y: 145},
				{Type: "Rat", X: 40, Y: 105},
			},
		},

		{
			ID: "crystal_peak", Width: 100, Height: H,
			Tags:        []string{"crystal"},
			LeftSocket:  NodeSocket{SurfaceY: 152, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 152, Type: "platform"},
			Islands: []NodeIsland{
				// Two base ledges
				{X: 3, Y: 152, W: 25, H: 3, Kind: "main"},
				{X: 72, Y: 152, W: 25, H: 3, Kind: "main"},
				// Rising crystal mountain centre
				{X: 35, Y: 168, W: 30, H: 5, Kind: "main"}, // near bottom
				{X: 40, Y: 150, W: 20, H: 3, Kind: "main"}, // lower peak
				{X: 43, Y: 125, W: 14, H: 2, Kind: "branch"},
				{X: 45, Y: 100, W: 10, H: 2, Kind: "branch"},
				{X: 46, Y: 75, W: 8, H: 2, Kind: "branch"},
				{X: 47, Y: 50, W: 6, H: 2, Kind: "secret"}, // peak
				// Side access steps
				{X: 20, Y: 138, W: 12, H: 2, Kind: "step"},
				{X: 25, Y: 118, W: 12, H: 2, Kind: "step"},
				{X: 22, Y: 95, W: 12, H: 2, Kind: "step"},
				{X: 68, Y: 133, W: 12, H: 2, Kind: "step"},
				{X: 65, Y: 112, W: 12, H: 2, Kind: "step"},
				{X: 68, Y: 88, W: 12, H: 2, Kind: "step"},
				// Hidden side platforms
				{X: 5, Y: 70, W: 22, H: 2, Kind: "branch"},
				{X: 73, Y: 65, W: 22, H: 2, Kind: "branch"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 50, Y: 122},
				{Type: "Rat", X: 50, Y: 97},
			},
		},

		// ══════════════════════════════════════════════════════════════════════
		// FOREST BIOME
		// ══════════════════════════════════════════════════════════════════════

		{
			ID: "forest_canopy", Width: 120, Height: H,
			Tags:        []string{"forest", "start"},
			LeftSocket:  NodeSocket{SurfaceY: 160, Type: "open"},
			RightSocket: NodeSocket{SurfaceY: 155, Type: "platform"},
			Islands: []NodeIsland{
				// Ground patch left
				{X: 3, Y: 160, W: 35, H: 6, Kind: "main"},
				// Ground patch right
				{X: 85, Y: 155, W: 32, H: 6, Kind: "main"},
				// Canopy level 1
				{X: 5, Y: 138, W: 22, H: 2, Kind: "main"},
				{X: 40, Y: 133, W: 25, H: 2, Kind: "main"},
				{X: 80, Y: 130, W: 22, H: 2, Kind: "main"},
				// Canopy level 2
				{X: 15, Y: 108, W: 20, H: 2, Kind: "main"},
				{X: 55, Y: 102, W: 22, H: 2, Kind: "main"},
				{X: 95, Y: 105, W: 20, H: 2, Kind: "main"},
				// Canopy level 3
				{X: 8, Y: 80, W: 18, H: 2, Kind: "branch"},
				{X: 45, Y: 75, W: 20, H: 2, Kind: "branch"},
				{X: 90, Y: 78, W: 18, H: 2, Kind: "branch"},
				// High canopy
				{X: 25, Y: 52, W: 70, H: 2, Kind: "branch"},
				// Sky secret
				{X: 40, Y: 18, W: 45, H: 2, Kind: "secret"},
				// Scatter steps (no tall trunks — open forest feel)
				{X: 30, Y: 118, W: 10, H: 2, Kind: "step"},
				{X: 72, Y: 116, W: 10, H: 2, Kind: "step"},
			},
			Entities: []NodeEntity{
				{Type: "Player", X: 18, Y: 157},
				{Type: "Rat", X: 50, Y: 130},
				{Type: "Rat", X: 95, Y: 99},
			},
		},

		{
			// Climbing-tree feel achieved via dense offset platforms; no blocking shaft.
			ID: "forest_trunk", Width: 80, Height: H,
			Tags:        []string{"forest"},
			LeftSocket:  NodeSocket{SurfaceY: 155, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 150, Type: "platform"},
			Islands: []NodeIsland{
				// Left base
				{X: 3, Y: 155, W: 22, H: 4, Kind: "main"},
				// Right base
				{X: 55, Y: 150, W: 22, H: 4, Kind: "main"},
				// Alternating branches simulate climbing a tree
				{X: 10, Y: 140, W: 25, H: 2, Kind: "branch"},
				{X: 41, Y: 130, W: 25, H: 2, Kind: "branch"},
				{X: 8, Y: 118, W: 22, H: 2, Kind: "branch"},
				{X: 43, Y: 108, W: 22, H: 2, Kind: "branch"},
				{X: 10, Y: 96, W: 22, H: 2, Kind: "branch"},
				{X: 41, Y: 86, W: 22, H: 2, Kind: "branch"},
				{X: 8, Y: 74, W: 22, H: 2, Kind: "branch"},
				{X: 43, Y: 64, W: 22, H: 2, Kind: "branch"},
				{X: 10, Y: 52, W: 22, H: 2, Kind: "branch"},
				// Tree crown
				{X: 12, Y: 32, W: 56, H: 3, Kind: "secret"},
				// Moss steps near base
				{X: 22, Y: 168, W: 10, H: 2, Kind: "step"},
				{X: 48, Y: 165, W: 10, H: 2, Kind: "step"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 20, Y: 137},
				{Type: "Rat", X: 55, Y: 127},
			},
		},

		{
			ID: "forest_glade", Width: 110, Height: H,
			Tags:        []string{"forest"},
			LeftSocket:  NodeSocket{SurfaceY: 150, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 148, Type: "platform"},
			Islands: []NodeIsland{
				// Glade floor — three patches with gaps
				{X: 3, Y: 150, W: 28, H: 4, Kind: "main"},
				{X: 42, Y: 157, W: 25, H: 4, Kind: "main"},
				{X: 80, Y: 148, W: 27, H: 4, Kind: "main"},
				// Stepping mushrooms
				{X: 32, Y: 142, W: 8, H: 2, Kind: "step"},
				{X: 70, Y: 138, W: 8, H: 2, Kind: "step"},
				// Mid-air platforms
				{X: 8, Y: 118, W: 28, H: 2, Kind: "main"},
				{X: 50, Y: 112, W: 25, H: 2, Kind: "main"},
				{X: 82, Y: 118, W: 25, H: 2, Kind: "main"},
				// Upper branches
				{X: 15, Y: 82, W: 25, H: 2, Kind: "branch"},
				{X: 55, Y: 75, W: 25, H: 2, Kind: "branch"},
				{X: 85, Y: 80, W: 20, H: 2, Kind: "branch"},
				// Canopy top
				{X: 25, Y: 48, W: 60, H: 2, Kind: "branch"},
				// Sky secret
				{X: 30, Y: 15, W: 50, H: 2, Kind: "secret"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 45, Y: 154},
				{Type: "Rat", X: 55, Y: 109},
			},
		},

		{
			ID: "forest_aerial", Width: 100, Height: H,
			Tags:        []string{"forest"},
			LeftSocket:  NodeSocket{SurfaceY: 148, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 152, Type: "platform"},
			Islands: []NodeIsland{
				// Pure aerial islands — no ground, no vertical walls
				{X: 3, Y: 148, W: 22, H: 2, Kind: "main"},
				{X: 75, Y: 152, W: 22, H: 2, Kind: "main"},
				// Central floating island cluster
				{X: 38, Y: 140, W: 24, H: 2, Kind: "main"},
				// Spiral upward path
				{X: 10, Y: 128, W: 18, H: 2, Kind: "step"},
				{X: 32, Y: 115, W: 18, H: 2, Kind: "step"},
				{X: 55, Y: 102, W: 18, H: 2, Kind: "step"},
				{X: 72, Y: 90, W: 18, H: 2, Kind: "step"},
				{X: 50, Y: 78, W: 18, H: 2, Kind: "step"},
				{X: 25, Y: 67, W: 18, H: 2, Kind: "step"},
				{X: 10, Y: 55, W: 18, H: 2, Kind: "step"},
				{X: 35, Y: 44, W: 18, H: 2, Kind: "step"},
				// Sky platform
				{X: 30, Y: 20, W: 40, H: 2, Kind: "secret"},
			},
			Entities: []NodeEntity{
				{Type: "Rat", X: 45, Y: 137},
			},
		},

		// ══════════════════════════════════════════════════════════════════════
		// UNIVERSAL CONNECTORS (work with any biome)
		// ══════════════════════════════════════════════════════════════════════

		{
			ID: "connector_flat", Width: 60, Height: H,
			Tags:        []string{"ruins", "crystal", "forest", "connector"},
			LeftSocket:  NodeSocket{SurfaceY: 155, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 155, Type: "platform"},
			Islands: []NodeIsland{
				// Bridge deck
				{X: 3, Y: 155, W: 54, H: 3, Kind: "main"},
				// Mid air platform
				{X: 18, Y: 120, W: 24, H: 2, Kind: "branch"},
				// Upper alcove
				{X: 5, Y: 85, W: 20, H: 2, Kind: "step"},
				{X: 35, Y: 78, W: 20, H: 2, Kind: "step"},
				// Secret ledge
				{X: 15, Y: 30, W: 30, H: 2, Kind: "secret"},
			},
			Entities: []NodeEntity{},
		},

		{
			ID: "connector_rise", Width: 65, Height: H,
			Tags:        []string{"ruins", "crystal", "forest", "connector"},
			LeftSocket:  NodeSocket{SurfaceY: 165, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 140, Type: "platform"},
			Islands: []NodeIsland{
				// Rising staircase of platforms
				{X: 3, Y: 165, W: 18, H: 3, Kind: "main"},
				{X: 22, Y: 155, W: 14, H: 2, Kind: "step"},
				{X: 36, Y: 148, W: 14, H: 2, Kind: "step"},
				{X: 50, Y: 140, W: 14, H: 3, Kind: "main"},
				// High path
				{X: 5, Y: 120, W: 20, H: 2, Kind: "branch"},
				{X: 35, Y: 108, W: 20, H: 2, Kind: "branch"},
				// Top shelf
				{X: 8, Y: 72, W: 45, H: 2, Kind: "branch"},
				// Secret
				{X: 15, Y: 20, W: 35, H: 2, Kind: "secret"},
			},
			Entities: []NodeEntity{},
		},

		{
			ID: "connector_fall", Width: 65, Height: H,
			Tags:        []string{"ruins", "crystal", "forest", "connector"},
			LeftSocket:  NodeSocket{SurfaceY: 140, Type: "platform"},
			RightSocket: NodeSocket{SurfaceY: 165, Type: "platform"},
			Islands: []NodeIsland{
				// Descending staircase
				{X: 3, Y: 140, W: 14, H: 3, Kind: "main"},
				{X: 18, Y: 148, W: 14, H: 2, Kind: "step"},
				{X: 32, Y: 155, W: 14, H: 2, Kind: "step"},
				{X: 46, Y: 165, W: 18, H: 3, Kind: "main"},
				// Side path high
				{X: 5, Y: 110, W: 20, H: 2, Kind: "branch"},
				{X: 38, Y: 102, W: 20, H: 2, Kind: "branch"},
				// Upper shelf
				{X: 12, Y: 68, W: 42, H: 2, Kind: "branch"},
				// Secret
				{X: 18, Y: 22, W: 30, H: 2, Kind: "secret"},
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
