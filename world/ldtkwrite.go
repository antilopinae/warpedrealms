// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package world

// ldtkwrite.go — generates valid LDtk 1.5.3 JSON files from programmatic level data.
//
// The output includes all required metadata fields for the LDtk editor plus the
// IntGrid (collision) and Entities (spawns/portals) layers that world.LoadLDtk parses.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ─── LDtk editor-compatible JSON types ───────────────────────────────────────

type ldtkWriteFile struct {
	JSONVersion      string           `json:"jsonVersion"`
	AppBuildID       int              `json:"appBuildId"`
	Header           ldtkWriteHeader  `json:"__header__"`
	Iid              string           `json:"iid"`
	ExternalLevels   bool             `json:"externalLevels"`
	BackupOnSave     bool             `json:"backupOnSave"`
	BackupLimit      int              `json:"backupLimit"`
	BackupRelPath    interface{}      `json:"backupRelPath"`
	MinifyJson       bool             `json:"minifyJson"`
	ImageExportMode  string           `json:"imageExportMode"`
	ExportLevelBg    bool             `json:"exportLevelBg"`
	PngFilePattern   interface{}      `json:"pngFilePattern"`
	SimplifiedExport bool             `json:"simplifiedExport"`
	LevelNamePattern string           `json:"levelNamePattern"`
	BgColor          string           `json:"bgColor"`
	DefaultPivotX    float64          `json:"defaultPivotX"`
	DefaultPivotY    float64          `json:"defaultPivotY"`
	DefaultGridSize  int              `json:"defaultGridSize"`
	DefaultEntityW   int              `json:"defaultEntityWidth"`
	DefaultEntityH   int              `json:"defaultEntityHeight"`
	DefaultLevelW    int              `json:"defaultLevelWidth"`
	DefaultLevelH    int              `json:"defaultLevelHeight"`
	DefaultLevelBg   interface{}      `json:"defaultLevelBgColor"`
	Flags            []string         `json:"flags"`
	WorldLayout      string           `json:"worldLayout"`
	WorldGridW       int              `json:"worldGridWidth"`
	WorldGridH       int              `json:"worldGridHeight"`
	Worlds           []interface{}    `json:"worlds"`
	Toc              []interface{}    `json:"toc"`
	Defs             ldtkWriteDefs    `json:"defs"`
	Levels           []ldtkWriteLevel `json:"levels"`
}

type ldtkWriteHeader struct {
	FileType   string `json:"fileType"`
	App        string `json:"app"`
	Doc        string `json:"doc"`
	Schema     string `json:"schema"`
	AppAuthor  string `json:"appAuthor"`
	AppVersion string `json:"appVersion"`
	URL        string `json:"url"`
}

type ldtkWriteDefs struct {
	Layers        []ldtkWriteLayerDef  `json:"layers"`
	Entities      []ldtkWriteEntityDef `json:"entities"`
	Tilesets      []interface{}        `json:"tilesets"`
	Enums         []interface{}        `json:"enums"`
	ExternalEnums []interface{}        `json:"externalEnums"`
	LevelFields   []interface{}        `json:"levelFields"`
}

type ldtkWriteLayerDef struct {
	Identifier             string             `json:"identifier"`
	Type                   string             `json:"type"`
	UID                    int                `json:"uid"`
	GridSize               int                `json:"gridSize"`
	GuideGridWid           int                `json:"guideGridWid"`
	GuideGridHei           int                `json:"guideGridHei"`
	DisplayOpacity         float64            `json:"displayOpacity"`
	InactiveOpacity        float64            `json:"inactiveOpacity"`
	HideInList             bool               `json:"hideInList"`
	HideFieldsWhenInactive bool               `json:"hideFieldsWhenInactive"`
	CanSelectWhenInactive  bool               `json:"canSelectWhenInactive"`
	RenderInWorldView      bool               `json:"renderInWorldView"`
	PxOffsetX              int                `json:"pxOffsetX"`
	PxOffsetY              int                `json:"pxOffsetY"`
	Parallaxfactorx        float64            `json:"parallaxFactorX"`
	ParallaxfactorY        float64            `json:"parallaxFactorY"`
	ParallaxScaling        bool               `json:"parallaxScaling"`
	RequiredTags           []string           `json:"requiredTags"`
	ExcludedTags           []string           `json:"excludedTags"`
	IntGridValues          []ldtkIntGridValue `json:"intGridValues"`
	IntGridValuesGroups    []interface{}      `json:"intGridValuesGroups"`
	AutoRuleGroups         []interface{}      `json:"autoRuleGroups"`
	AutoSourceLayerDefUid  interface{}        `json:"autoSourceLayerDefUid"`
	TilesetDefUid          interface{}        `json:"tilesetDefUid"`
	TilePivotX             float64            `json:"tilePivotX"`
	TilePivotY             float64            `json:"tilePivotY"`
	Doc                    interface{}        `json:"doc"`
	UiColor                interface{}        `json:"uiColor"`
	UiFilterTags           []string           `json:"uiFilterTags"`
}

type ldtkIntGridValue struct {
	Value      int         `json:"value"`
	Identifier string      `json:"identifier"`
	Color      string      `json:"color"`
	Tile       interface{} `json:"tile"`
	GroupUid   int         `json:"groupUid"`
}

type ldtkWriteEntityDef struct {
	Identifier       string         `json:"identifier"`
	UID              int            `json:"uid"`
	Tags             []string       `json:"tags"`
	ExportToToc      bool           `json:"exportToToc"`
	AllowOutOfBounds bool           `json:"allowOutOfBounds"`
	Width            int            `json:"width"`
	Height           int            `json:"height"`
	ResizableX       bool           `json:"resizableX"`
	ResizableY       bool           `json:"resizableY"`
	KeepAspectRatio  bool           `json:"keepAspectRatio"`
	TileOpacity      float64        `json:"tileOpacity"`
	FillOpacity      float64        `json:"fillOpacity"`
	LineOpacity      float64        `json:"lineOpacity"`
	Hollow           bool           `json:"hollow"`
	Color            string         `json:"color"`
	RenderMode       string         `json:"renderMode"`
	ShowName         bool           `json:"showName"`
	TilesetId        interface{}    `json:"tilesetId"`
	TileRenderMode   string         `json:"tileRenderMode"`
	TileRect         interface{}    `json:"tileRect"`
	NineSliceBorders []int          `json:"nineSliceBorders"`
	MaxCount         int            `json:"maxCount"`
	LimitScope       string         `json:"limitScope"`
	LimitBehavior    string         `json:"limitBehavior"`
	PivotX           float64        `json:"pivotX"`
	PivotY           float64        `json:"pivotY"`
	FieldDefs        []ldtkFieldDef `json:"fieldDefs"`
	Doc              interface{}    `json:"doc"`
	UiTileRect       interface{}    `json:"uiTileRect"`
}

type ldtkFieldDef struct {
	Identifier           string      `json:"identifier"`
	UID                  int         `json:"uid"`
	InternalType         string      `json:"type"`   // Haxe enum: "F_String", "F_Float", "F_Int", "F_Bool"
	DisplayType          string      `json:"__type"` // Human-readable: "String", "Float", "Int", "Bool"
	IsArray              bool        `json:"isArray"`
	CanBeNull            bool        `json:"canBeNull"`
	ArrayMinLength       interface{} `json:"arrayMinLength"`
	ArrayMaxLength       interface{} `json:"arrayMaxLength"`
	EditorDisplayMode    string      `json:"editorDisplayMode"`
	EditorDisplayScale   float64     `json:"editorDisplayScale"`
	EditorDisplayPos     string      `json:"editorDisplayPos"`
	EditorLinkStyle      string      `json:"editorLinkStyle"`
	ShowInWorld          bool        `json:"showInWorld"`
	EditorDisplayColor   interface{} `json:"editorDisplayColor"`
	EditorAlwaysShow     bool        `json:"editorAlwaysShow"`
	EditorCutLongValues  bool        `json:"editorCutLongValues"`
	EditorTextSuffix     interface{} `json:"editorTextSuffix"`
	EditorTextPrefix     interface{} `json:"editorTextPrefix"`
	UseForSmartColor     bool        `json:"useForSmartColor"`
	Min                  interface{} `json:"min"`
	Max                  interface{} `json:"max"`
	Regex                interface{} `json:"regex"`
	AcceptFileTypes      interface{} `json:"acceptFileTypes"`
	DefaultOverride      interface{} `json:"defaultOverride"`
	TextLanguageMode     interface{} `json:"textLanguageMode"`
	SymmetricalRef       bool        `json:"symmetricalRef"`
	AutoChainRef         bool        `json:"autoChainRef"`
	AllowedRefs          string      `json:"allowedRefs"`
	AllowedRefsEntityUid interface{} `json:"allowedRefsEntityUid"`
	AllowedRefTags       []string    `json:"allowedRefTags"`
	TilesetUid           interface{} `json:"tilesetUid"`
	Doc                  interface{} `json:"doc"`
}

type ldtkWriteLevel struct {
	Identifier      string           `json:"identifier"`
	Iid             string           `json:"iid"`
	UID             int              `json:"uid"`
	WorldX          int              `json:"worldX"`
	WorldY          int              `json:"worldY"`
	WorldDepth      int              `json:"worldDepth"`
	PxWid           int              `json:"pxWid"`
	PxHei           int              `json:"pxHei"`
	BgColor         interface{}      `json:"bgColor"`
	BgRelPath       interface{}      `json:"bgRelPath"`
	ExternalRelPath interface{}      `json:"externalRelPath"`
	FieldInstances  []interface{}    `json:"fieldInstances"`
	BgColorFull     string           `json:"__bgColor"`
	BgPos           interface{}      `json:"__bgPos"`
	Neighbours      []interface{}    `json:"__neighbours"`
	SmartColor      string           `json:"__smartColor"`
	LayerInstances  []ldtkWriteLayer `json:"layerInstances"`
}

// ldtkWriteLayer field tags mirror ldtkLayerInstance in ldtk.go.
type ldtkWriteLayer struct {
	Identifier      string            `json:"__identifier"`
	Type            string            `json:"__type"`
	GridSize        int               `json:"__gridSize"`
	CWid            int               `json:"__cWid"`
	CHei            int               `json:"__cHei"`
	Opacity         float64           `json:"__opacity"`
	PxTotalOffsetX  int               `json:"__pxTotalOffsetX"`
	PxTotalOffsetY  int               `json:"__pxTotalOffsetY"`
	TilesetUID      interface{}       `json:"__tilesetDefUid"`
	TilesetPath     interface{}       `json:"__tilesetRelPath"`
	Iid             string            `json:"iid"`
	LevelId         int               `json:"levelId"`
	LayerDefUid     int               `json:"layerDefUid"`
	PxOffsetX       int               `json:"pxOffsetX"`
	PxOffsetY       int               `json:"pxOffsetY"`
	Visible         bool              `json:"visible"`
	OptionalRules   []interface{}     `json:"optionalRules"`
	IntGridCSV      []int             `json:"intGridCsv"`
	AutoTiles       []interface{}     `json:"autoLayerTiles"`
	Seed            int               `json:"seed"`
	OverrideTileset interface{}       `json:"overrideTilesetUid"`
	GridTiles       []interface{}     `json:"gridTiles"`
	Entities        []ldtkWriteEntity `json:"entityInstances"`
}

// ldtkWriteEntity field tags mirror ldtkEntityInst in ldtk.go.
type ldtkWriteEntity struct {
	Identifier string           `json:"__identifier"`
	Grid       [2]int           `json:"__grid"`
	Pivot      [2]float64       `json:"__pivot"`
	Tags       []string         `json:"__tags"`
	Tile       interface{}      `json:"__tile"`
	SmartColor string           `json:"__smartColor"`
	Iid        string           `json:"iid"`
	Width      int              `json:"width"`
	Height     int              `json:"height"`
	DefUid     int              `json:"defUid"`
	PX         [2]int           `json:"px"`
	Fields     []ldtkWriteField `json:"fieldInstances"`
}

// ldtkWriteField field tags mirror ldtkField in ldtk.go.
type ldtkWriteField struct {
	Identifier string        `json:"__identifier"`
	Type       string        `json:"__type"`
	Value      interface{}   `json:"__value"`
	Tile       interface{}   `json:"__tile"`
	DefUid     int           `json:"defUid"`
	RealEditor []interface{} `json:"realEditorValues"`
}

// ─── Public input types ───────────────────────────────────────────────────────

// LDtkWriteLevel is the caller-facing description of one level.
type LDtkWriteLevel struct {
	ID            string   // level identifier (e.g. "room_01")
	GridW         int      // width  in blocks
	GridH         int      // height in blocks
	GridSize      int      // pixels per block (default 16)
	SolidCells    [][2]int // [col, row] pairs that should be solid (value 1)
	PlatformCells [][2]int // [col, row] pairs that are one-way platforms (value 3)
	Entities      []LDtkWriteEntity
}

// LDtkWriteEntity is a game object placed in the Entities layer.
type LDtkWriteEntity struct {
	Identifier string // "Player", "Rat", "JumpLink", "RevealZone", …
	PX, PY     int    // pixel position (top-left within the level)
	W, H       int    // bounding box size in pixels
	Fields     []LDtkWriteField
}

// LDtkWriteField is a named field on an entity.
// Value may be string, float64, int, or bool — any JSON-encodable scalar.
type LDtkWriteField struct {
	Key   string
	Value interface{}
}

// ─── UID constants (must match between defs and instances) ────────────────────

const (
	uidLayerCollision = 2
	uidLayerEntities  = 3

	uidEntityPlayer          = 10
	uidEntityRat             = 11
	uidEntityJumpLink        = 12
	uidEntityReveal          = 13
	uidEntityRift            = 14
	uidEntityMiniBoss        = 15 // level-1 boss encounter (ground)
	uidEntityBoss            = 16 // level-2 boss encounter (ground)
	uidEntitySuperBoss       = 17 // level-3 boss encounter (ground)
	uidEntityFlyingMiniBoss  = 18 // level-1 boss encounter (flying)
	uidEntityFlyingBoss      = 19 // level-2 boss encounter (flying)
	uidEntityFlyingSuperBoss = 20 // level-3 boss encounter (flying)
	uidEntityRiftZone        = 21 // zone where rifts may spawn (dim purple)
	uidEntityPortalZone      = 22 // static portal location at ground level (gold)

	uidFieldTarget   = 30
	uidFieldLabel    = 31
	uidFieldArrivalX = 32
	uidFieldArrivalY = 33
	uidFieldKind     = 34
)

// ─── WriteLDtkFile ────────────────────────────────────────────────────────────

// WriteLDtkFile serialises the supplied levels to an LDtk 1.5.3 JSON file.
// The output is readable by world.LoadLDtk and can be opened in the LDtk editor.
func WriteLDtkFile(path string, levels []LDtkWriteLevel) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ldtkwrite mkdir %s: %w", filepath.Dir(path), err)
	}

	gs := 16 // default grid size

	// Build layer definitions (shared across all levels).
	collisionLayerDef := ldtkWriteLayerDef{
		Identifier:             "Collision",
		Type:                   "IntGrid",
		UID:                    uidLayerCollision,
		GridSize:               gs,
		GuideGridWid:           0,
		GuideGridHei:           0,
		DisplayOpacity:         1,
		InactiveOpacity:        0.6,
		HideInList:             false,
		HideFieldsWhenInactive: true,
		CanSelectWhenInactive:  true,
		RenderInWorldView:      true,
		RequiredTags:           []string{},
		ExcludedTags:           []string{},
		IntGridValues: []ldtkIntGridValue{
			{Value: 1, Identifier: "solid", Color: "#826B4E", Tile: nil, GroupUid: 0},
			{Value: 3, Identifier: "platform", Color: "#5BAA72", Tile: nil, GroupUid: 0},
		},
		IntGridValuesGroups: []interface{}{},
		AutoRuleGroups:      []interface{}{},
		UiFilterTags:        []string{},
	}

	entitiesLayerDef := ldtkWriteLayerDef{
		Identifier:             "Entities",
		Type:                   "Entities",
		UID:                    uidLayerEntities,
		GridSize:               gs,
		GuideGridWid:           0,
		GuideGridHei:           0,
		DisplayOpacity:         1,
		InactiveOpacity:        0.6,
		HideInList:             false,
		HideFieldsWhenInactive: true,
		CanSelectWhenInactive:  true,
		RenderInWorldView:      true,
		RequiredTags:           []string{},
		ExcludedTags:           []string{},
		IntGridValues:          []ldtkIntGridValue{},
		IntGridValuesGroups:    []interface{}{},
		AutoRuleGroups:         []interface{}{},
		UiFilterTags:           []string{},
	}

	// Entity definitions.
	entityDefs := []ldtkWriteEntityDef{
		makeEntityDef("Player", uidEntityPlayer, "#00FF00", 26, 76),
		makeEntityDef("Rat", uidEntityRat, "#FF6600", 20, 30),
		makeEntityDef("JumpLink", uidEntityJumpLink, "#00CCFF", 56, 56),
		makeEntityDef("RevealZone", uidEntityReveal, "#FFFF00", 64, 64),
		makeEntityDef("Rift", uidEntityRift, "#FF4444", 32, 32),
		// Boss encounter markers — colours match maprenderer.go bossSpawnColors.
		// Ground bosses: filled squares.
		makeEntityDef("MiniBoss", uidEntityMiniBoss, "#B482FF", 16, 16),   // soft lavender
		makeEntityDef("Boss", uidEntityBoss, "#FF5050", 16, 16),           // vivid crimson
		makeEntityDef("SuperBoss", uidEntitySuperBoss, "#FFC832", 16, 16), // molten gold
		// Flying bosses: same hue but lighter tint (diamond in the game renderer).
		makeEntityDef("FlyingMiniBoss", uidEntityFlyingMiniBoss, "#D8B8FF", 16, 16),   // pale lavender
		makeEntityDef("FlyingBoss", uidEntityFlyingBoss, "#FF9090", 16, 16),           // pale crimson
		makeEntityDef("FlyingSuperBoss", uidEntityFlyingSuperBoss, "#FFE882", 16, 16), // pale gold
		// Zone overlays — resizable rectangles drawn on the map.
		makeZoneEntityDef("RiftZone", uidEntityRiftZone, "#8844DD", 0.15, true),      // dim purple, hollow
		makeZoneEntityDef("PortalZone", uidEntityPortalZone, "#DDBB00", 0.20, false), // gold, semi-filled
	}
	// Add field defs to JumpLink (index 2).
	entityDefs[2].FieldDefs = []ldtkFieldDef{
		makeFieldDef("target", uidFieldTarget, "String"),
		makeFieldDef("label", uidFieldLabel, "String"),
		makeFieldDef("arrival_x", uidFieldArrivalX, "Float"),
		makeFieldDef("arrival_y", uidFieldArrivalY, "Float"),
	}
	// Add field defs to Rift (index 4).
	entityDefs[4].FieldDefs = []ldtkFieldDef{
		makeFieldDef("target", uidFieldTarget, "String"),
		makeFieldDef("kind", uidFieldKind, "String"),
		makeFieldDef("arrival_x", uidFieldArrivalX, "Float"),
		makeFieldDef("arrival_y", uidFieldArrivalY, "Float"),
	}

	wLevels := make([]ldtkWriteLevel, len(levels))
	for i, lvl := range levels {
		levelGs := lvl.GridSize
		if levelGs <= 0 {
			levelGs = gs
		}
		gw, gh := lvl.GridW, lvl.GridH
		levelUID := i + 1

		// IntGrid CSV: row-major flat array (0 = air, 1 = solid, 3 = platform).
		csv := make([]int, gw*gh)
		for _, cell := range lvl.SolidCells {
			col, row := cell[0], cell[1]
			if col >= 0 && col < gw && row >= 0 && row < gh {
				csv[row*gw+col] = 1
			}
		}
		for _, cell := range lvl.PlatformCells {
			col, row := cell[0], cell[1]
			if col >= 0 && col < gw && row >= 0 && row < gh {
				csv[row*gw+col] = 3
			}
		}

		// Build entity instances.
		entInsts := make([]ldtkWriteEntity, len(lvl.Entities))
		for j, ent := range lvl.Entities {
			defUID := uidEntityPlayer
			switch ent.Identifier {
			case "Rat":
				defUID = uidEntityRat
			case "JumpLink":
				defUID = uidEntityJumpLink
			case "RevealZone":
				defUID = uidEntityReveal
			case "Rift":
				defUID = uidEntityRift
			case "MiniBoss":
				defUID = uidEntityMiniBoss
			case "Boss":
				defUID = uidEntityBoss
			case "SuperBoss":
				defUID = uidEntitySuperBoss
			case "FlyingMiniBoss":
				defUID = uidEntityFlyingMiniBoss
			case "FlyingBoss":
				defUID = uidEntityFlyingBoss
			case "FlyingSuperBoss":
				defUID = uidEntityFlyingSuperBoss
			case "RiftZone":
				defUID = uidEntityRiftZone
			case "PortalZone":
				defUID = uidEntityPortalZone
			}

			fields := make([]ldtkWriteField, len(ent.Fields))
			for k, f := range ent.Fields {
				fUID := uidFieldTarget
				fType := "String"
				switch f.Key {
				case "label":
					fUID = uidFieldLabel
				case "kind":
					fUID = uidFieldKind
				case "arrival_x":
					fUID = uidFieldArrivalX
					fType = "Float"
				case "arrival_y":
					fUID = uidFieldArrivalY
					fType = "Float"
				}
				fields[k] = ldtkWriteField{
					Identifier: f.Key,
					Type:       fType,
					Value:      f.Value,
					Tile:       nil,
					DefUid:     fUID,
					RealEditor: []interface{}{},
				}
			}

			gridX, gridY := 0, 0
			if levelGs > 0 {
				gridX = ent.PX / levelGs
				gridY = ent.PY / levelGs
			}

			entInsts[j] = ldtkWriteEntity{
				Identifier: ent.Identifier,
				Grid:       [2]int{gridX, gridY},
				Pivot:      [2]float64{0.5, 1},
				Tags:       []string{},
				Tile:       nil,
				SmartColor: "#FFFFFF",
				Iid:        fakeIID(i*1000 + j + 100),
				Width:      ent.W,
				Height:     ent.H,
				DefUid:     defUID,
				PX:         [2]int{ent.PX, ent.PY},
				Fields:     fields,
			}
		}

		// LDtk stores layers top-to-bottom visually; our parser reverses them.
		// Put Entities first (top) and Collision second (bottom).
		wLevels[i] = ldtkWriteLevel{
			Identifier:      lvl.ID,
			Iid:             fakeIID(i + 1),
			UID:             levelUID,
			WorldX:          i * (gw*levelGs + 32), // side by side with gap
			WorldY:          0,
			WorldDepth:      0,
			PxWid:           gw * levelGs,
			PxHei:           gh * levelGs,
			BgColor:         nil,
			BgRelPath:       nil,
			ExternalRelPath: nil,
			FieldInstances:  []interface{}{},
			BgColorFull:     "#1D2B53",
			BgPos:           nil,
			Neighbours:      []interface{}{},
			SmartColor:      "#FFFFFF",
			LayerInstances: []ldtkWriteLayer{
				{
					Identifier:      "Entities",
					Type:            "Entities",
					GridSize:        levelGs,
					CWid:            gw,
					CHei:            gh,
					Opacity:         1,
					PxTotalOffsetX:  0,
					PxTotalOffsetY:  0,
					TilesetUID:      nil,
					TilesetPath:     nil,
					Iid:             fakeIID(i*10 + 1),
					LevelId:         levelUID,
					LayerDefUid:     uidLayerEntities,
					PxOffsetX:       0,
					PxOffsetY:       0,
					Visible:         true,
					OptionalRules:   []interface{}{},
					IntGridCSV:      []int{},
					AutoTiles:       []interface{}{},
					Seed:            0,
					OverrideTileset: nil,
					GridTiles:       []interface{}{},
					Entities:        entInsts,
				},
				{
					Identifier:      "Collision",
					Type:            "IntGrid",
					GridSize:        levelGs,
					CWid:            gw,
					CHei:            gh,
					Opacity:         1,
					PxTotalOffsetX:  0,
					PxTotalOffsetY:  0,
					TilesetUID:      nil,
					TilesetPath:     nil,
					Iid:             fakeIID(i*10 + 2),
					LevelId:         levelUID,
					LayerDefUid:     uidLayerCollision,
					PxOffsetX:       0,
					PxOffsetY:       0,
					Visible:         true,
					OptionalRules:   []interface{}{},
					IntGridCSV:      csv,
					AutoTiles:       []interface{}{},
					Seed:            0,
					OverrideTileset: nil,
					GridTiles:       []interface{}{},
					Entities:        []ldtkWriteEntity{},
				},
			},
		}
	}

	out := ldtkWriteFile{
		JSONVersion: "1.5.3",
		AppBuildID:  473703, // build number of LDtk 1.5.3
		Header: ldtkWriteHeader{
			FileType:   "LDtk Project JSON",
			App:        "LDtk",
			Doc:        "https://ldtk.io/docs/",
			Schema:     "https://ldtk.io/files/JSON_SCHEMA.json",
			AppAuthor:  "Deepnight Games",
			AppVersion: "1.5.3",
			URL:        "https://ldtk.io",
		},
		Iid:              fakeIID(0),
		ExternalLevels:   false,
		BackupOnSave:     false,
		BackupLimit:      10,
		BackupRelPath:    nil,
		MinifyJson:       false,
		ImageExportMode:  "None",
		ExportLevelBg:    true,
		PngFilePattern:   nil,
		SimplifiedExport: false,
		LevelNamePattern: "Level_%idx",
		BgColor:          "#1D2B53",
		DefaultPivotX:    0,
		DefaultPivotY:    1,
		DefaultGridSize:  gs,
		DefaultEntityW:   gs,
		DefaultEntityH:   gs,
		DefaultLevelW:    256,
		DefaultLevelH:    256,
		DefaultLevelBg:   nil,
		Flags:            []string{},
		WorldLayout:      "Free",
		WorldGridW:       512,
		WorldGridH:       512,
		Worlds:           []interface{}{},
		Toc:              []interface{}{},
		Defs: ldtkWriteDefs{
			Layers:        []ldtkWriteLayerDef{entitiesLayerDef, collisionLayerDef},
			Entities:      entityDefs,
			Tilesets:      []interface{}{},
			Enums:         []interface{}{},
			ExternalEnums: []interface{}{},
			LevelFields:   []interface{}{},
		},
		Levels: wLevels,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("ldtkwrite marshal: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// fakeIID generates a deterministic fake UUID-like string.
func fakeIID(n int) string {
	return fmt.Sprintf("wr%08x-0000-0000-0000-000000000000", n)
}

func makeEntityDef(id string, uid int, color string, w, h int) ldtkWriteEntityDef {
	return ldtkWriteEntityDef{
		Identifier:       id,
		UID:              uid,
		Tags:             []string{},
		ExportToToc:      false,
		AllowOutOfBounds: false,
		Width:            w,
		Height:           h,
		ResizableX:       true,
		ResizableY:       true,
		KeepAspectRatio:  false,
		TileOpacity:      1,
		FillOpacity:      0.08,
		LineOpacity:      1,
		Hollow:           false,
		Color:            color,
		RenderMode:       "Rectangle",
		ShowName:         true,
		TilesetId:        nil,
		TileRenderMode:   "FitInside",
		TileRect:         nil,
		NineSliceBorders: []int{},
		MaxCount:         0,
		LimitScope:       "PerLevel",
		LimitBehavior:    "MoveLastOne",
		PivotX:           0.5,
		PivotY:           1,
		FieldDefs:        []ldtkFieldDef{},
	}
}

// makeZoneEntityDef creates an entity definition for a resizable zone overlay.
// fillOpacity controls how filled the zone rectangle appears in LDtk.
// hollow=true draws only the border.
func makeZoneEntityDef(id string, uid int, color string, fillOpacity float64, hollow bool) ldtkWriteEntityDef {
	d := makeEntityDef(id, uid, color, 32, 32)
	d.FillOpacity = fillOpacity
	d.LineOpacity = 0.9
	d.Hollow = hollow
	d.ShowName = false
	d.PivotX = 0
	d.PivotY = 0
	return d
}

// makeFieldDef creates a field definition for an entity.
// displayType is the human-readable name ("String", "Float", "Int", "Bool").
// internalType ("F_String" etc.) is derived automatically.
func makeFieldDef(id string, uid int, displayType string) ldtkFieldDef {
	return ldtkFieldDef{
		Identifier:          id,
		UID:                 uid,
		InternalType:        "F_" + displayType, // LDtk Haxe enum constructor
		DisplayType:         displayType,
		IsArray:             false,
		CanBeNull:           true,
		ArrayMinLength:      nil,
		ArrayMaxLength:      nil,
		EditorDisplayMode:   "Hidden",
		EditorDisplayScale:  1,
		EditorDisplayPos:    "Above",
		EditorLinkStyle:     "CurvedArrow",
		ShowInWorld:         false,
		EditorAlwaysShow:    false,
		EditorCutLongValues: true,
		UseForSmartColor:    false,
		AllowedRefs:         "Any",
		AllowedRefTags:      []string{},
	}
}
