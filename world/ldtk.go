package world

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"warpedrealms/shared"
)

// ─── LDtk raw JSON structures (only what we need) ────────────────────────────

type ldtkFile struct {
	Levels []ldtkLevel `json:"levels"`
	Defs   ldtkDefs    `json:"defs"`
}

type ldtkDefs struct {
	Tilesets []ldtkTilesetDef `json:"tilesets"`
}

type ldtkTilesetDef struct {
	UID          int    `json:"uid"`
	Identifier   string `json:"identifier"`
	RelPath      string `json:"relPath"`
	PxWid        int    `json:"pxWid"`
	PxHei        int    `json:"pxHei"`
	TileGridSize int    `json:"tileGridSize"`
}

type ldtkLevel struct {
	Identifier     string              `json:"identifier"`
	PxWid          int                 `json:"pxWid"`
	PxHei          int                 `json:"pxHei"`
	LayerInstances []ldtkLayerInstance `json:"layerInstances"`
}

type ldtkLayerInstance struct {
	Identifier  string           `json:"__identifier"`
	Type        string           `json:"__type"` // IntGrid | Tiles | AutoLayer | Entities
	GridSize    int              `json:"__gridSize"`
	CWid        int              `json:"__cWid"`
	CHei        int              `json:"__cHei"`
	TilesetPath string           `json:"__tilesetRelPath"`
	TilesetUID  int              `json:"__tilesetDefUid"`
	IntGridCSV  []int            `json:"intGridCsv"`
	GridTiles   []ldtkTileInst   `json:"gridTiles"`
	AutoTiles   []ldtkTileInst   `json:"autoLayerTiles"`
	Entities    []ldtkEntityInst `json:"entityInstances"`
}

type ldtkTileInst struct {
	Px  [2]int  `json:"px"`  // pixel position in level space
	Src [2]int  `json:"src"` // pixel position in tileset image
	F   int     `json:"f"`   // flip flags: bit0=flipH, bit1=flipV
	T   int     `json:"t"`   // tile ID (informational)
	A   float64 `json:"a"`   // alpha 0‥1 (0 means opaque in older LDtk)
}

type ldtkEntityInst struct {
	Identifier string      `json:"__identifier"`
	PX         [2]int      `json:"px"` // pixel position relative to level top-left
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	Fields     []ldtkField `json:"fieldInstances"`
}

type ldtkField struct {
	Identifier string          `json:"__identifier"`
	Value      json.RawMessage `json:"__value"`
}

// ─── Public LDtk tile types (used by renderer) ───────────────────────────────

// LDtkTile is a single rendered tile from an LDtk layer.
type LDtkTile struct {
	X, Y         int     // destination pixel position inside the level
	SrcX, SrcY   int     // source pixel position inside the tileset image
	W, H         int     // tile size in pixels
	FlipH, FlipV bool
	Alpha        float64 // 1 = fully opaque
}

// LDtkLayer is a visual tile layer extracted from LDtk.
type LDtkLayer struct {
	Name        string
	TilesetPath string // absolute path to the tileset PNG
	TileW, TileH int
	Tiles        []LDtkTile
}

// ─── Public loaders ───────────────────────────────────────────────────────────

// LoadLDtk parses a .ldtk file and returns all levels as separate MapData objects.
// Levels are returned in the order they appear in the project file.
func LoadLDtk(ldtkPath string) ([]*MapData, error) {
	raw, err := os.ReadFile(ldtkPath)
	if err != nil {
		return nil, fmt.Errorf("read ldtk %s: %w", ldtkPath, err)
	}
	var file ldtkFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("unmarshal ldtk %s: %w", ldtkPath, err)
	}

	baseDir := filepath.Dir(ldtkPath)

	// UID → tileset definition
	tsByUID := make(map[int]ldtkTilesetDef, len(file.Defs.Tilesets))
	for _, ts := range file.Defs.Tilesets {
		tsByUID[ts.UID] = ts
	}

	results := make([]*MapData, 0, len(file.Levels))
	for _, level := range file.Levels {
		m, err := parseLDtkLevel(level, tsByUID, baseDir, ldtkPath)
		if err != nil {
			return nil, fmt.Errorf("level %s: %w", level.Identifier, err)
		}
		results = append(results, m)
	}
	return results, nil
}

// ─── Internal parsing ─────────────────────────────────────────────────────────

func parseLDtkLevel(level ldtkLevel, tsByUID map[int]ldtkTilesetDef, baseDir, ldtkPath string) (*MapData, error) {
	m := &MapData{
		ID:          level.Identifier, // e.g. "Level_0"
		Path:        ldtkPath,
		PixelWidth:  level.PxWid,
		PixelHeight: level.PxHei,
	}

	// Process layers in reverse so the first layer in the list renders on top.
	// LDtk stores layers top-to-bottom (first = topmost visually).
	for i := len(level.LayerInstances) - 1; i >= 0; i-- {
		layer := level.LayerInstances[i]
		switch layer.Type {
		case "IntGrid":
			if m.TileWidth == 0 {
				m.TileWidth = layer.GridSize
				m.TileHeight = layer.GridSize
				m.Width = layer.CWid
				m.Height = layer.CHei
			}
			ldtkExtractSolids(m, layer)

		case "Tiles", "AutoLayer":
			ldtkLayer, err := ldtkExtractTileLayer(layer, tsByUID, baseDir)
			if err != nil {
				continue // non-fatal: skip layers with missing tilesets
			}
			m.LDtkLayers = append(m.LDtkLayers, ldtkLayer)
			if m.TileWidth == 0 && layer.GridSize > 0 {
				m.TileWidth = layer.GridSize
				m.TileHeight = layer.GridSize
			}

		case "Entities":
			ldtkExtractEntities(m, layer)
		}
	}

	if m.TileWidth == 0 {
		m.TileWidth = 16
		m.TileHeight = 16
	}

	// Hard boundary walls and floor so entities cannot leave the level.
	// Left/right walls prevent players from walking off edges (they must use
	// portals).  The floor prevents infinite falling when no platform is below.
	// These are per-level so they don't conflict when rooms share origin (0,0) —
	// per-room solid caching in serverapp/room.go ensures each entity only
	// collides against the walls of its own current room.
	m.SolidRects = append(m.SolidRects,
		shared.Rect{X: -256, Y: 0, W: 256, H: float64(m.PixelHeight) + 256},
		shared.Rect{X: float64(m.PixelWidth), Y: 0, W: 256, H: float64(m.PixelHeight) + 256},
		shared.Rect{X: -256, Y: float64(m.PixelHeight), W: float64(m.PixelWidth) + 512, H: 256},
	)

	return m, nil
}

func ldtkExtractSolids(m *MapData, layer ldtkLayerInstance) {
	gs := float64(layer.GridSize)
	for idx, val := range layer.IntGridCSV {
		col := idx % layer.CWid
		row := idx / layer.CWid
		rect := shared.Rect{
			X: float64(col) * gs,
			Y: float64(row) * gs,
			W: gs,
			H: gs,
		}
		switch val {
		case 1: // solid — full two-way collision
			m.SolidRects = append(m.SolidRects, rect)
		case 3: // one-way platform — passable from below, landable from above
			m.PlatformRects = append(m.PlatformRects, rect)
		}
	}
}

func ldtkExtractTileLayer(layer ldtkLayerInstance, tsByUID map[int]ldtkTilesetDef, baseDir string) (LDtkLayer, error) {
	// Resolve tileset definition.
	ts, ok := tsByUID[layer.TilesetUID]
	if !ok {
		return LDtkLayer{}, fmt.Errorf("tileset uid %d not found in defs", layer.TilesetUID)
	}

	relPath := layer.TilesetPath
	if relPath == "" {
		relPath = ts.RelPath
	}
	if relPath == "" {
		return LDtkLayer{}, fmt.Errorf("layer %s has no tileset path", layer.Identifier)
	}

	tileW := ts.TileGridSize
	if tileW <= 0 {
		tileW = layer.GridSize
	}
	tileH := tileW

	result := LDtkLayer{
		Name:        layer.Identifier,
		TilesetPath: filepath.Clean(filepath.Join(baseDir, relPath)),
		TileW:       tileW,
		TileH:       tileH,
	}

	// Combine gridTiles and autoLayerTiles.
	all := append(layer.GridTiles, layer.AutoTiles...)
	result.Tiles = make([]LDtkTile, 0, len(all))
	for _, t := range all {
		a := t.A
		if a <= 0 {
			a = 1 // older LDtk versions omit alpha field (defaults to opaque)
		}
		result.Tiles = append(result.Tiles, LDtkTile{
			X:     t.Px[0],
			Y:     t.Px[1],
			SrcX:  t.Src[0],
			SrcY:  t.Src[1],
			W:     tileW,
			H:     tileH,
			FlipH: t.F&1 != 0,
			FlipV: t.F&2 != 0,
			Alpha: a,
		})
	}
	return result, nil
}

func ldtkExtractEntities(m *MapData, layer ldtkLayerInstance) {
	for _, e := range layer.Entities {
		id := strings.ToLower(e.Identifier)
		px := float64(e.PX[0])
		py := float64(e.PX[1])
		props := ldtkParseFields(e.Fields)

		switch id {
		case "player", "playerspawn", "player_spawn":
			m.PlayerSpawns = append(m.PlayerSpawns, shared.Vec2{X: px, Y: py})

		case "rat", "ratspawn", "rat_spawn", "enemy_rat":
			m.RatSpawns = append(m.RatSpawns, shared.Vec2{X: px, Y: py})

		case "jumplink", "jump_link":
			link := MapJumpLink{
				ID:     fmt.Sprintf("link-%d-%d", e.PX[0], e.PX[1]),
				Area:   shared.Rect{X: px, Y: py, W: float64(e.Width), H: float64(e.Height)},
				Target: props["target"],
				Label:  props["label"],
			}
			if v, ok := props["arrival_x"]; ok {
				link.HasArrival = true
				fmt.Sscanf(v, "%f", &link.ArrivalX)
				fmt.Sscanf(props["arrival_y"], "%f", &link.ArrivalY)
			}
			m.JumpLinks = append(m.JumpLinks, link)

		case "revealzone", "reveal_zone":
			m.RevealZones = append(m.RevealZones, MapRevealZone{
				ID:     fmt.Sprintf("reveal-%d-%d", e.PX[0], e.PX[1]),
				Area:   shared.Rect{X: px, Y: py, W: float64(e.Width), H: float64(e.Height)},
				Target: props["target"],
			})

		case "rift":
			rift := MapRift{
				ID:   fmt.Sprintf("rift-%d-%d", e.PX[0], e.PX[1]),
				Area: shared.Rect{X: px, Y: py, W: float64(e.Width), H: float64(e.Height)},
				Target: props["target"],
				Kind:   props["kind"],
			}
			if rift.Kind == "" {
				rift.Kind = "green"
			}
			if v, ok := props["arrival_x"]; ok {
				rift.HasArrival = true
				fmt.Sscanf(v, "%f", &rift.ArrivalX)
				fmt.Sscanf(props["arrival_y"], "%f", &rift.ArrivalY)
			}
			m.Rifts = append(m.Rifts, rift)
		}
	}
}

// ldtkParseFields converts LDtk field instances to a simple string map.
// String, Int, Float, Bool fields are all converted to their string representations.
func ldtkParseFields(fields []ldtkField) map[string]string {
	result := make(map[string]string, len(fields))
	for _, f := range fields {
		key := strings.ToLower(f.Identifier)
		var s string
		// Try string first, then fall back to raw JSON.
		if err := json.Unmarshal(f.Value, &s); err == nil {
			result[key] = s
		} else {
			result[key] = strings.Trim(string(f.Value), `"`)
		}
	}
	return result
}
