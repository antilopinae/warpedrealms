package world

// tiled.go — runtime map data structures shared by the LDtk loader and the
// procedural level generator.  All TMX/XML-specific code has been removed;
// levels are authored in LDtk or generated via content.Bundle.GenerateRaidProcGen.

import "warpedrealms/shared"

// MapData is the unified in-memory representation of one level.
type MapData struct {
	ID          string // level identifier (e.g. LDtk level name or procgen "room_01")
	Path        string // source file path (empty for in-memory generated maps)

	Width      int // grid width  in blocks
	Height     int // grid height in blocks
	TileWidth  int // pixels per block (horizontal)
	TileHeight int // pixels per block (vertical)
	PixelWidth  int // Width  * TileWidth
	PixelHeight int // Height * TileHeight

	// LDtkLayers holds visual tile layers produced by the LDtk loader.
	// Nil / empty for maps that only carry collision geometry.
	LDtkLayers []LDtkLayer

	SolidRects    []shared.Rect  // collision rectangles (pixel coords)
	PlatformRects []shared.Rect  // one-way platform rects (pixel coords)
	PlayerSpawns []shared.Vec2  // player spawn points
	RatSpawns    []shared.Vec2  // rat / enemy spawn points
	JumpLinks    []MapJumpLink  // portal / door connections
	RevealZones  []MapRevealZone
	Rifts        []MapRift      // transient finite-use portals
}

// DefaultPlayerSpawn returns a safe player spawn point.
func (m *MapData) DefaultPlayerSpawn(index int) shared.Vec2 {
	if len(m.PlayerSpawns) == 0 {
		return shared.Vec2{X: 120, Y: 240}
	}
	return m.PlayerSpawns[index%len(m.PlayerSpawns)]
}

// MapJumpLink is a portal that teleports the player to another room.
// Target may be an explicit room ID (e.g. "room_02") or the relative
// strings "above" / "below" for backward compatibility.
type MapJumpLink struct {
	ID         string
	Area       shared.Rect // trigger zone in local level space
	Target     string      // destination room identifier
	Label      string      // UI label shown near the portal
	HasArrival bool
	ArrivalX   float64 // arrival X in target room local space
	ArrivalY   float64 // arrival Y in target room local space
	HasPreview bool
	PreviewX, PreviewY, PreviewW, PreviewH float64
}

// MapRevealZone reveals an adjacent room when the player enters the area.
type MapRevealZone struct {
	ID     string
	Area   shared.Rect
	Target string // destination room identifier or "above"/"below"
}

// MapRift is a transient inter-room portal with finite capacity.
// Kind is one of "red" (5 uses), "blue" (2 uses), "green" (1 use).
type MapRift struct {
	ID         string
	Area       shared.Rect
	Target     string // destination room ID
	Kind       string // "red" | "blue" | "green"
	HasArrival bool
	ArrivalX   float64
	ArrivalY   float64
}
