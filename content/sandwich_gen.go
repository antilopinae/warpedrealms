// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package content

import (
	"math"
	"math/rand"
	"sort"

	"warpedrealms/shared"
	"warpedrealms/world"
)

const (
	sgBaseGridW = 400
	sgBaseGridH = 300

	// Player body size in blocks — used throughout BFS reachability, tunnel sizing,
	// step widths, etc. Changing these two constants adapts the whole generator.
	sgPlayerW = 3 // width (horizontal blocks the player occupies)
	sgPlayerH = 5 // height (vertical blocks from feet to head)

	sgCellAir      = 0
	sgCellSolid    = 1
	sgCellBackwall = 2
	// sgCellPlatform is a one-way platform: the player can jump through it from
	// below and land on top. Rendered with a distinct colour. Not a full solid —
	// does not block horizontal movement or upward passage.
	sgCellPlatform = 3

	sgBlockSizePx = 16
)

// sgPreflightFailReason captures the last preflight failure for debugging.
// Set to empty string when validation passes.
var sgPreflightFailReason string

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

type sgPoint struct {
	X int
	Y int
}

type sgInlet struct {
	CenterX int
	Y       int
	Width   int
}

type sgShaft struct {
	CenterX int
	Left    int
	Right   int
	TopY    int
	BottomY int
}

type sgHub struct {
	X int
	Y int
	W int
	H int
}

func (h sgHub) Center() sgPoint {
	// Y - 2, чтобы туннель заходил ровно на уровне ног игрока, стоящего на полу
	return sgPoint{X: h.X + h.W/2, Y: h.Y + h.H - 2}
}

type sgRowSegment struct {
	Left  int
	Right int
}

type sgSkyHub struct {
	X        int
	Y        int
	W        int
	H        int
	ObjectID int
}

type sgStairSide string

const (
	sgStairSideLeft  sgStairSide = "left"
	sgStairSideRight sgStairSide = "right"
)

type sgStairChain struct {
	HubIndex   int
	Side       sgStairSide
	TrendDir   int
	ObjectID   int
	Steps      []sgPoint
	BaseSteps  int
	ExtraSteps int
	Connected  bool
}

type sgSkyDebugData struct {
	SkyBandH       int
	SkyArea        int
	HubCount       int
	ExtraStepCount int
	ExtraPlaced    int
	GapPlatforms   int
	Hubs           []sgSkyHub
	Chains         []sgStairChain
	Tags           *sgSkyTagGrid
}

type sgSkyObjectKind string

const (
	sgSkyObjectHub          sgSkyObjectKind = "hub"
	sgSkyObjectSkyStepLeft  sgSkyObjectKind = "sky_step_left"
	sgSkyObjectSkyStepRight sgSkyObjectKind = "sky_step_right"
	sgSkyObjectShaftStep    sgSkyObjectKind = "shaft_step"
)

type sgSkyTag struct {
	ObjectID int
	Kind     sgSkyObjectKind
}

type sgSkyTagGrid struct {
	Cells   [][]sgSkyTag
	nextObj int
}

func sgNewSkyTagGrid(w, h int) *sgSkyTagGrid {
	if w <= 0 || h <= 0 {
		return nil
	}
	cells := make([][]sgSkyTag, h)
	for y := range cells {
		cells[y] = make([]sgSkyTag, w)
	}
	return &sgSkyTagGrid{Cells: cells}
}

func (tg *sgSkyTagGrid) NewObject(kind sgSkyObjectKind) int {
	if tg == nil {
		return 0
	}
	tg.nextObj++
	return tg.nextObj
}

func (tg *sgSkyTagGrid) MarkRect(x0, y0, w, h, objectID int, kind sgSkyObjectKind) {
	if tg == nil || objectID <= 0 {
		return
	}
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			tg.MarkCell(x, y, objectID, kind)
		}
	}
}

func (tg *sgSkyTagGrid) MarkCell(x, y, objectID int, kind sgSkyObjectKind) {
	if tg == nil || objectID <= 0 {
		return
	}
	if y < 0 || y >= len(tg.Cells) || x < 0 || x >= len(tg.Cells[y]) {
		return
	}
	tg.Cells[y][x] = sgSkyTag{ObjectID: objectID, Kind: kind}
}

func (tg *sgSkyTagGrid) ClearCell(x, y int) {
	if tg == nil {
		return
	}
	if y < 0 || y >= len(tg.Cells) || x < 0 || x >= len(tg.Cells[y]) {
		return
	}
	tg.Cells[y][x] = sgSkyTag{}
}

func (tg *sgSkyTagGrid) At(x, y int) sgSkyTag {
	if tg == nil {
		return sgSkyTag{}
	}
	if y < 0 || y >= len(tg.Cells) || x < 0 || x >= len(tg.Cells[y]) {
		return sgSkyTag{}
	}
	return tg.Cells[y][x]
}

type SandwichLocation struct {
	Grid          [][]int
	Inlets        []sgInlet
	Hubs          []sgHub
	Level         world.LDtkWriteLevel
	SolidRects    []shared.Rect
	BackwallRects []shared.Rect
	// PlatformRects are one-way platforms (sgCellPlatform=3): passable from
	// below, landable from above. Rendered with a distinct colour.
	PlatformRects []shared.Rect
	PrimarySpawn  shared.Vec2
	SpawnPoints   []shared.Vec2
	// BossSpawns lists planned boss encounter positions in pixel space.
	// Level 1 = mini boss, 2 = boss, 3 = super boss.
	BossSpawns []shared.BossSpawn
	// RiftZones lists pixel-space rectangles where a rift is allowed to spawn.
	// Each zone is a candidate location; the server picks among them each time a
	// rift needs to materialise. Zones avoid boss rooms and favour sky platforms
	// and underground tunnel mouths at ground level.
	RiftZones []shared.Rect
	// SplitY is the block-row that separates sky (y < SplitY) from underground.
	SplitY int
	// IsValid is false when pre-flight reachability checks fail.
	// Callers (e.g. the server) should discard invalid maps and retry with a new seed.
	IsValid bool
}

// GenerateSandwichLocation builds one room using HK-style cave hubs.
// Geometry is produced in block-space and exported as LDtkWriteLevel solid cells.
func GenerateSandwichLocation(rng *rand.Rand, id string, cfg ProcGenConfig) SandwichLocation {
	cfg = cfg.fill()

	gridW := max(48, cfg.GridW)
	gridH := max(36, cfg.GridH)
	splitY := gridH / 2

	grid := make([][]int, gridH)
	for y := 0; y < gridH; y++ {
		grid[y] = make([]int, gridW)
		base := sgCellAir
		if y >= splitY {
			base = sgCellSolid
		}
		for x := 0; x < gridW; x++ {
			grid[y][x] = base
		}
	}
	skyTags := sgNewSkyTagGrid(gridW, gridH)

	inlets := sgBuildInlets(gridW, splitY, cfg)
	hubs := sgPlaceHubs(rng, grid, splitY, cfg)
	if len(hubs) == 0 {
		fallback := sgHub{
			X: max(2, gridW/2-sgScaleX(12, cfg)/2),
			Y: min(gridH-4, splitY+sgScaleY(10, cfg)),
			W: sgScaleX(12, cfg),
			H: sgScaleY(10, cfg),
		}
		if fallback.W < 8 {
			fallback.W = 8
		}
		if fallback.H < 6 {
			fallback.H = 6
		}
		fallback.X = sgClamp(fallback.X, 1, max(1, gridW-fallback.W-1))
		fallback.Y = sgClamp(fallback.Y, splitY+1, max(splitY+1, gridH-fallback.H-1))
		hubs = append(hubs, fallback)
		sgCarveRect(grid, fallback.X, fallback.Y, fallback.W, fallback.H)
	}

	carveRadius := 3
	edges := make(map[[2]int]bool)

	// Carve inlet shafts first — we need their positions for boss room placement.
	shafts := make([]sgShaft, 0, len(inlets))
	for _, inlet := range inlets {
		nearest := sgNearestHub(inlet, hubs)
		hubCenter := hubs[nearest].Center()
		shaftBottom := sgClamp(hubCenter.Y, splitY+max(4, sgScaleY(6, cfg)), gridH-2)
		shaft := sgCarveInletShaft(grid, inlet, shaftBottom)
		sgAddInletZigZagPlatforms(grid, shaft, cfg, skyTags)
		bottom := sgPoint{X: shaft.CenterX, Y: shaft.BottomY}
		sgCarveBeziez(rng, grid, bottom, hubCenter, carveRadius)
		shafts = append(shafts, shaft)
	}

	// Place boss rooms in the densest solid earth between/near inlet shafts.
	// Must run after shafts are carved so solid-ratio checks see actual density.
	bossRooms := sgPlaceBossRoomsNearShafts(rng, grid, shafts, hubs, splitY, cfg, carveRadius)
	// Place mini-boss side rooms adjacent to boss rooms (60% chance each).
	miniBossRooms := sgPlaceMiniBossRooms(rng, grid, bossRooms, splitY)

	sgConnectHubMST(rng, grid, hubs, carveRadius, edges)
	// Extra return loop: 1 additional cross-connection.
	extraLoops := 1
	for i := 0; i < extraLoops; i++ {
		a, b := sgPickLoopHubPair(rng, len(hubs), edges)
		if a >= 0 && b >= 0 {
			sgCarveBeziez(rng, grid, hubs[a].Center(), hubs[b].Center(), carveRadius)
			edges[[2]int{min(a, b), max(a, b)}] = true
		}
	}

	// Apply organic noise texture to underground walls before MST tunneling.
	sgApplySimplexTexture(rng, grid, splitY)

	returnLoopUsed := sgAddReturnLoops(rng, grid, splitY, inlets, hubs, carveRadius, cfg, skyTags)
	sgAddExitTunnels(rng, grid, splitY, cfg, inlets, skyTags, returnLoopUsed)
	sgFillAllShaftPits(grid, splitY, bossRooms, miniBossRooms)
	sgAddInternalLedges(rng, grid, splitY, cfg)
	// Close single-cell horizontal gaps between solid platforms.
	sgFillThinGaps(grid)

	// Sky layer: place hubs + build stair chains to ground immediately (no deferred step).
	skyDbg := sgPopulateSkyAndObjectsWithTags(rng, grid, cfg, splitY, skyTags)
	sgApplySkyStrictAntiStacking(grid, skyTags, splitY, cfg)
	sgFinalizeHubAccessibility(grid, hubs, skyTags, cfg)
	sgApplyGroundAirCorridor(grid, splitY, cfg, inlets, skyDbg.Chains, skyTags)
	sgApplyTunnelHeadroom(grid, splitY, cfg, skyTags)
	sgEnsureJumpConnectivity(grid, splitY, cfg)
	// Extend stair chains all the way to the ground and add any missing chains.
	// sgCleanUpAndOptimizeGeometry is gone, so these steps will no longer be deleted.
	sgEnsureGroundToSkyAccessibility(rng, grid, skyTags, skyDbg.Hubs, skyDbg.Chains, inlets, splitY, cfg)
	sgEnsureGlobalPassableConnectivity(rng, grid, splitY, cfg, skyTags)
	penthouses := sgAddSkyPenthouses(rng, grid, skyTags, skyDbg.Hubs, cfg)
	skyDbg.Hubs = append(skyDbg.Hubs, penthouses...)
	// Remove any sky-zone platform the player can never reach.
	// Must run after all sky objects are placed, before preflight validation.
	sgPruneUnreachableSkyPlatforms(grid, skyTags, splitY)

	maxBossLevel := sgMaxBossLevelForZone(cfg.Zone)
	bossSpawns := sgBuildBossSpawnList(grid, bossRooms, miniBossRooms, skyDbg.Hubs, splitY, gridH, maxBossLevel, sgBlockSizePx)

	solidCells := sgCellsForValue(grid, sgCellSolid)
	platformCells := sgCellsForValue(grid, sgCellPlatform)
	spawnPoints := sgSpawnPointsFromInlets(inlets, cfg)
	primarySpawn := shared.Vec2{}
	if len(spawnPoints) > 0 {
		primarySpawn = spawnPoints[0]
	}

	entities := make([]world.LDtkWriteEntity, 0, 1+len(bossSpawns))
	if len(spawnPoints) > 0 {
		entities = append(entities, world.LDtkWriteEntity{
			Identifier: "Player",
			PX:         int(primarySpawn.X),
			PY:         int(primarySpawn.Y),
			W:          26,
			H:          76,
		})
	}

	bossEntityName := func(level int, flying bool) string {
		if flying {
			switch level {
			case 1:
				return "FlyingMiniBoss"
			case 3:
				return "FlyingSuperBoss"
			default:
				return "FlyingBoss"
			}
		}
		switch level {
		case 1:
			return "MiniBoss"
		case 3:
			return "SuperBoss"
		default:
			return "Boss"
		}
	}

	for _, bs := range bossSpawns {
		entities = append(entities, world.LDtkWriteEntity{
			Identifier: bossEntityName(bs.Level, bs.Flying),
			PX:         int(bs.X),
			PY:         int(bs.Y),
			W:          sgBlockSizePx,
			H:          sgBlockSizePx,
		})
	}

	level := world.LDtkWriteLevel{
		ID:            id,
		GridW:         gridW,
		GridH:         gridH,
		GridSize:      sgBlockSizePx,
		SolidCells:    solidCells,
		PlatformCells: platformCells,
		Entities:      entities,
	}

	isValid := sgRunPreflightValidation(grid, skyTags, skyDbg.Hubs, inlets, splitY)

	return SandwichLocation{
		Grid:          grid,
		Inlets:        inlets,
		Hubs:          hubs,
		Level:         level,
		SolidRects:    GenerateRects(grid, sgCellSolid),
		BackwallRects: GenerateRects(grid, sgCellBackwall),
		PlatformRects: GenerateRects(grid, sgCellPlatform),
		BossSpawns:    bossSpawns,
		RiftZones:     sgBuildRiftZones(grid, skyTags, bossRooms, miniBossRooms, splitY, sgBlockSizePx),
		SplitY:        splitY,
		PrimarySpawn:  primarySpawn,
		SpawnPoints:   spawnPoints,
		IsValid:       isValid,
	}
}

func sgEnsureJumpConnectivity(grid [][]int, splitY int, cfg ProcGenConfig) {
	width := len(grid[0])
	height := len(grid)

	for iteration := 0; iteration < 20; iteration++ {
		canEscape := make([][]bool, height)
		for i := range canEscape {
			canEscape[i] = make([]bool, width)
		}

		sgMarkReachable(grid, canEscape, splitY)

		modified := false
		// Сканируем снизу вверх, ищем изолированный пол
		for y := height - 2; y > splitY; y-- {
			for x := 1; x < width-1; x++ {
				// Пол есть, а выхода нет
				if grid[y][x] != sgCellSolid && grid[y+1][x] == sgCellSolid && !canEscape[y][x] {
					sgDigToSafety(grid, canEscape, splitY, x, y)
					modified = true
					break
				}
			}
			if modified {
				break
			}
		}

		for y := height - 2; y > splitY; y-- {
			for x := 1; x < width-1; x++ {
				// Если это пол, на котором стоит игрок, но он не может выпрыгнуть
				if grid[y][x] == sgCellAir && grid[y+1][x] == sgCellSolid && !canEscape[y][x] {
					// Ищем ближайшую проходимую точку СЛЕВА или СПРАВА на этом же уровне (y)
					for dist := 1; dist < 20; dist++ {
						// Проверяем вправо
						if x+dist < width && canEscape[y][x+dist] {
							sgCarvePlayerCorridor(grid, x, x+dist, y)
							modified = true
							break
						}
						// Проверяем влево
						if x-dist > 0 && canEscape[y][x-dist] {
							sgCarvePlayerCorridor(grid, x-dist, x, y)
							modified = true
							break
						}
					}
				}
			}
		}

		if !modified {
			break
		}
	}
}

// sgCarvePlayerCorridor carves a horizontal tunnel from x1 to x2 at standing row y.
// The tunnel is exactly sgPlayerW wide (extending the path by pW-1) and sgPlayerH tall
// (from y-sgPlayerH+1 to y), so a player standing at y can walk through without
// hitting a ceiling. The floor at y+1 is preserved.
func sgCarvePlayerCorridor(grid [][]int, x1, x2, y int) {
	gridH, gridW := len(grid), len(grid[0])
	lx := min(x1, x2)
	rx := max(x1, x2) + sgPlayerW - 1 // extend by player width so last cell is also accessible
	for cx := lx; cx <= rx; cx++ {
		for cy := y - sgPlayerH + 1; cy <= y; cy++ {
			if cx >= 0 && cx < gridW && cy >= 0 && cy < gridH {
				sgCarveCell(grid, cx, cy)
			}
		}
	}
}

func sgDigToSafety(grid [][]int, canEscape [][]bool, splitY, startX, startY int) {
	width := len(grid[0])
	height := len(grid)

	// Ищем ближайшую безопасную точку (по воздуху и полу)
	type point struct{ x, y int }
	queue := []point{{startX, startY}}
	visited := make([][]bool, height)
	for i := range visited {
		visited[i] = make([]bool, width)
	}
	visited[startY][startX] = true

	var target *point
	head := 0
	for head < len(queue) {
		curr := queue[head]
		head++

		if canEscape[curr.y][curr.x] {
			target = &curr
			break
		}

		dirs := []point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
		for _, d := range dirs {
			nx, ny := curr.x+d.x, curr.y+d.y
			if nx >= 1 && nx < width-1 && ny >= 1 && ny < height-1 && !visited[ny][nx] {
				visited[ny][nx] = true
				queue = append(queue, point{nx, ny})
			}
		}
	}

	if target == nil {
		target = &point{startX, splitY + 2} // Резервный выход на поверхность
	}

	// Копаем ВЕРТИКАЛЬНУЮ ШАХТУ ВВЕРХ
	// Начинаем чуть выше пола ямы, чтобы гарантированно не разрушить пол
	digStartY := startY - 2
	if digStartY < 1 {
		digStartY = 1
	}
	targetY := target.y - 1 // Целимся на уровень головы безопасной зоны
	if targetY < 1 {
		targetY = 1
	}

	stepY := -1
	if targetY > digStartY {
		stepY = 1 // Редкий случай: безопасная зона ниже ямы (падаем вниз)
	}

	// Ширина шахты = 8 блоков (достаточно для zig-zag платформ шириной sgPlayerW)
	for y := digStartY; y != targetY+stepY; y += stepY {
		for dx := -3; dx <= 4; dx++ {
			nx := startX + dx
			if nx >= 1 && nx < width-1 && y >= 1 && y < height-1 {
				if grid[y][nx] == sgCellSolid {
					grid[y][nx] = sgCellBackwall
				}
			}
		}
	}

	// Копаем ГОРИЗОНТАЛЬНЫЙ ТУННЕЛЬ ВБОК
	stepX := 1
	if target.x < startX {
		stepX = -1
	}
	// Туннель строго прямоугольный, высота 4 блока
	for x := startX; x != target.x+stepX; x += stepX {
		for dy := 0; dy < 4; dy++ {
			ty := targetY - dy
			if ty >= 1 && ty < height-1 && x >= 1 && x < width-1 {
				if grid[ty][x] == sgCellSolid {
					grid[ty][x] = sgCellBackwall
				}
			}
		}
	}

	// Если прорубили наверх — ставим зигзагообразные платформы шириной sgPlayerW
	if targetY < startY-6 {
		platW := sgPlayerW // Минимум 3 блока — ровно под ширину игрока
		shaftLeft := startX - 3
		shaftRight := startX + 4 - platW // right platform starts here
		sideLeft := true

		for y := startY - 6; y >= targetY; y -= 6 {
			platX := shaftLeft
			if !sideLeft {
				platX = shaftRight
			}

			if platX < 1 {
				platX = 1
			}
			if platX+platW > width-1 {
				platX = width - platW - 1
			}

			// Skip this row if an existing platform is already within sgPlayerH+1
			// rows in any column of the shaft — this prevents sgDigToSafety from
			// doubling up on zigzag steps that were already placed by
			// sgAddInletZigZagPlatforms, sgAddReturnLoops, or sgAddExitTunnels.
			tooClose := false
			checkX0 := max(1, shaftLeft)
			checkX1 := min(width-2, shaftLeft+7) // shaft is ~7 wide
			for scanY := max(0, y-sgPlayerH-1); scanY <= min(height-1, y+sgPlayerH+1); scanY++ {
				if scanY == y {
					continue
				}
				for scanX := checkX0; scanX <= checkX1; scanX++ {
					if grid[scanY][scanX] == sgCellPlatform {
						tooClose = true
						break
					}
				}
				if tooClose {
					break
				}
			}
			if tooClose {
				sideLeft = !sideLeft
				continue
			}

			for i := 0; i < platW; i++ {
				px := platX + i
				if px >= 1 && px < width-1 {
					grid[y][px] = sgCellPlatform
					// Прорубаем sgPlayerH блоков воздуха над платформой
					for pDy := 1; pDy < sgPlayerH; pDy++ {
						if y-pDy >= 1 {
							grid[y-pDy][px] = sgCellBackwall
						}
					}
				}
			}
			sideLeft = !sideLeft
		}
	}
}

func sgMarkReachable(grid [][]int, canEscape [][]bool, splitY int) {
	sgMarkReachableWithTags(grid, canEscape, splitY, nil)
}

func sgMarkReachableWithTags(grid [][]int, canEscape [][]bool, splitY int, tags *sgSkyTagGrid) {
	width, height := len(grid[0]), len(grid)
	pW, pH := sgPlayerW, sgPlayerH

	// isSolidBlocking returns true for cells that physically block the player.
	// sgCellPlatform cells (one-way platforms) and tagged stairchain steps are
	// treated as passable from below so jump arcs can pass through them.
	isSolidBlocking := func(tx, ty int) bool {
		if ty < 0 || ty >= height || tx < 0 || tx >= width {
			return true // out of bounds = blocked
		}
		c := grid[ty][tx]
		if c != sgCellSolid {
			return false // air, backwall, or platform — all passable
		}
		// sgCellSolid: check if it's a tagged one-way step.
		if tags != nil {
			tag := tags.At(tx, ty)
			if tag.Kind == sgSkyObjectSkyStepLeft ||
				tag.Kind == sgSkyObjectSkyStepRight ||
				tag.Kind == sgSkyObjectShaftStep {
				return false // one-way platform, arc passes through
			}
		}
		return true
	}

	// isFloor returns true if the cell below the feet is solid enough to stand on.
	// Both regular solid cells and one-way platforms count as floor.
	isFloor := func(tx, ty int) bool {
		if ty < 0 || ty >= height || tx < 0 || tx >= width {
			return false
		}
		c := grid[ty][tx]
		return c == sgCellSolid || c == sgCellPlatform
	}

	// Проверка: влезет ли "тело" игрока в точку x,y
	canFit := func(x, y int) bool {
		for dy := 0; dy < pH; dy++ {
			for dx := 0; dx < pW; dx++ {
				tx, ty := x+dx, y-dy
				if isSolidBlocking(tx, ty) {
					return false
				}
			}
		}
		return true
	}

	type point struct{ x, y int }
	queue := []point{}

	// Стартуем со всех проходимых точек на поверхности (splitY)
	for x := 0; x < width-pW; x++ {
		if canFit(x, splitY) {
			canEscape[splitY][x] = true
			queue = append(queue, point{x, splitY})
		}
	}

	head := 0
	for head < len(queue) {
		curr := queue[head]
		head++

		// ПЕРЕМЕЩЕНИЕ ПО ПОЛУ (влево-вправо)
		for _, dx := range []int{-1, 1} {
			nx := curr.x + dx
			if nx >= 0 && nx < width-pW && canFit(nx, curr.y) && !canEscape[curr.y][nx] {
				// Проверяем наличие пола под ногами игрока (pW блоков ширины).
				// Платформы (sgCellPlatform) тоже считаются полом — стоять можно.
				hasFloor := false
				for fdx := 0; fdx < pW && nx+fdx < width; fdx++ {
					if isFloor(nx+fdx, curr.y+1) {
						hasFloor = true
						break
					}
				}
				if hasFloor {
					canEscape[curr.y][nx] = true
					queue = append(queue, point{nx, curr.y})
				}
			}
		}

		// ПРЫЖКИ — диапазон откалиброван по физике игрока:
		//    JumpSpeed=735 px/s, Gravity=2050 px/s², BlockSize=16 px →
		//    max высота ≈8.2 блока, max горизонталь ≈10 блоков при беге.
		//    При высоком прыжке (dy=-8) горизонталь ≈9 блоков одновременно.
		//    Фильтр: отсекает недостижимые комбинации (слишком далеко/высоко).
		for dy := -9; dy <= 3; dy++ {
			for dx := -10; dx <= 10; dx++ {
				// Физически недостижимые комбинации: слишком высоко + слишком далеко.
				// Формула учитывает, что при max высоте (8 блоков) можно пройти ≈9 блоков.
				// Нормировка: (dy/9)² + (dx/10)² > 1 → dy²*100 + dx²*81 > 8100
				if dy*dy*100+dx*dx*81 > 8100 {
					continue
				}
				nx, ny := curr.x+dx, curr.y+dy
				if nx < 0 || nx >= width-pW || ny < pH || ny >= height-1 {
					continue
				}

				if !canEscape[ny][nx] && canFit(nx, ny) {
					// Приземление на твёрдый пол или платформу (пW позиций).
					hasFloor := false
					for fdx := 0; fdx < pW && nx+fdx < width; fdx++ {
						if isFloor(nx+fdx, ny+1) {
							hasFloor = true
							break
						}
					}
					if hasFloor {
						// Проверка траектории: не летим ли сквозь стену
						if sgIsJumpPathClearWithTags(grid, tags, curr.x, curr.y, nx, ny, pW, pH) {
							canEscape[ny][nx] = true
							queue = append(queue, point{nx, ny})
						}
					}
				}
			}
		}
	}
}

// sgFindUndergroundRiftSurfaces ищет подходящие площадки для рифтов в подземелье.
// Он проверяет только физическую возможность размещения (пол + свободное место),
// не учитывая, может ли игрок дойти туда пешком от спавна.
func sgFindUndergroundRiftSurfaces(grid [][]int, splitY int, bossRooms, miniBossRooms [][4]int, blockSize int) []shared.Rect {
	gridH := len(grid)
	gridW := len(grid[0])
	var zones []shared.Rect

	// Параметры рифта в блоках (примерно 2x3 блока)
	const riftWidthBlocks = 2
	const riftHeightBlocks = 5

	// Проверка на вхождение в комнаты боссов (чтобы не спавнить рифты прямо в бою)
	inBossArea := func(x, y int) bool {
		for _, r := range append(bossRooms, miniBossRooms...) {
			cx, cy, rx, ry := r[0], r[1], r[2], r[3]
			if x >= cx-rx-1 && x <= cx+rx+1 && y >= cy-ry-1 && y <= cy+1 {
				return true
			}
		}
		return false
	}

	// Сканируем всё, что ниже поверхности
	for y := splitY + 1; y < gridH-riftHeightBlocks; y++ {
		runStart := -1
		for x := 0; x < gridW; x++ {
			// Условие для потенциального пола рифта:
			// 1. Клетка (x, y) - воздух или бэкволл
			// 2. Клетка (x, y+1) - Solid ИЛИ Platform (ступеньки в шахтах)
			// 3. Над клеткой (x, y) есть еще 2 свободных блока (высота рифта)
			// 4. Мы не в комнате босса

			hasFloor := grid[y+1][x] == sgCellSolid || grid[y+1][x] == sgCellPlatform
			hasHeadroom := sgIsPassable(grid[y][x]) && sgIsPassable(grid[y-1][x]) && sgIsPassable(grid[y-2][x])

			isValidSpot := hasFloor && hasHeadroom && !inBossArea(x, y)

			if isValidSpot {
				if runStart < 0 {
					runStart = x
				}
			} else {
				if runStart >= 0 {
					runLen := x - runStart
					// Если нашли хотя бы 2 блока ровного пола подряд
					if runLen >= riftWidthBlocks {
						zones = append(zones, shared.Rect{
							X: float64(runStart * blockSize),
							Y: float64((y - 1) * blockSize), // Ставим зону так, чтобы низ рифта был на уровне пола
							W: float64(runLen * blockSize),
							H: float64(2 * blockSize),
						})
					}
					runStart = -1
				}
			}
		}
	}
	return zones
}

// sgIsJumpPathClear checks if a player can jump from (x1,y1) to (x2,y2) by
// simulating a parabolic arc. Stairchain step cells are treated as one-way
// platforms (passable from below) so they don't block jump arcs.
func sgIsJumpPathClear(grid [][]int, x1, y1, x2, y2, pW, pH int) bool {
	return sgIsJumpPathClearWithTags(grid, nil, x1, y1, x2, y2, pW, pH)
}

func sgIsJumpPathClearWithTags(grid [][]int, tags *sgSkyTagGrid, x1, y1, x2, y2, pW, pH int) bool {
	gridH, gridW := len(grid), len(grid[0])
	dx := x2 - x1
	dy := y2 - y1
	peakH := 0
	if dy < 0 {
		if -dy < 8 {
			peakH = -dy
		} else {
			peakH = 8
		}
	}
	steps := (abs(dx) + abs(dy)) * 4
	if steps < 8 {
		steps = 8
	}

	isBlocking := func(tx, ty int) bool {
		if tx < 0 || tx >= gridW || ty < 0 || ty >= gridH {
			return true
		}
		if grid[ty][tx] != sgCellSolid {
			return false
		}
		// Stairchain steps are one-way platforms: passable for arc checks.
		if tags != nil {
			tag := tags.At(tx, ty)
			if tag.Kind == sgSkyObjectSkyStepLeft ||
				tag.Kind == sgSkyObjectSkyStepRight ||
				tag.Kind == sgSkyObjectShaftStep {
				return false
			}
		}
		return true
	}

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		cx := x1 + int(math.Round(float64(dx)*t))
		arcOffset := int(math.Round(float64(peakH) * 4.0 * t * (1.0 - t)))
		cy := y1 + int(math.Round(float64(dy)*t)) - arcOffset
		for ddx := 0; ddx < pW; ddx++ {
			for ddy := 0; ddy < pH; ddy++ {
				if isBlocking(cx+ddx, cy-ddy) {
					return false
				}
			}
		}
	}
	return true
}

func sgEnsureGroundToSkyAccessibility(rng *rand.Rand, grid [][]int, tags *sgSkyTagGrid, hubs []sgSkyHub, chains []sgStairChain, inlets []sgInlet, splitY int, cfg ProcGenConfig) {
	if len(grid) == 0 {
		return
	}
	stepW := max(sgPlayerW, sgScaleX(3, cfg))

	// Прогоняем существующие цепочки
	for i := range chains {
		sgExtendChainToGround(rng, &chains[i], grid, tags, inlets, splitY, stepW, 0, 0)
	}

	// Гарантируем спуск для хабов, если у них нет цепочек
	for hubIdx, hub := range hubs {
		hasLeft, hasRight := false, false
		for _, ch := range chains {
			if ch.HubIndex == hubIdx {
				if ch.Side == sgStairSideLeft {
					hasLeft = true
				}
				if ch.Side == sgStairSideRight {
					hasRight = true
				}
			}
		}

		if !hasLeft {
			newChain := sgStairChain{HubIndex: hubIdx, Side: sgStairSideLeft, TrendDir: -1,
				ObjectID: tags.NewObject(sgSkyObjectSkyStepLeft),
				Steps:    []sgPoint{{X: hub.X, Y: hub.Y + hub.H - 1}}}
			sgExtendChainToGround(rng, &newChain, grid, tags, inlets, splitY, stepW, 0, 0)
		}
		if !hasRight {
			newChain := sgStairChain{HubIndex: hubIdx, Side: sgStairSideRight, TrendDir: 1,
				ObjectID: tags.NewObject(sgSkyObjectSkyStepRight),
				Steps:    []sgPoint{{X: hub.X + hub.W - stepW, Y: hub.Y + hub.H - 1}}}
			sgExtendChainToGround(rng, &newChain, grid, tags, inlets, splitY, stepW, 0, 0)
		}
	}
}

func sgFindDynamicGroundY(grid [][]int, x int, splitY int) int {
	gridH := len(grid)
	if x < 0 || x >= len(grid[0]) {
		return gridH - 1
	}
	// Начинаем искать от splitY (уровень поверхности) вниз
	for y := splitY; y < gridH; y++ {
		if grid[y][x] == sgCellSolid {
			return y
		}
	}
	return gridH - 1
}

func sgSmoothConnectToGround(grid [][]int, x, y, stepW int) {
	gridH := len(grid)
	gridW := len(grid[0])

	for dx := 0; dx < stepW; dx++ {
		nx := x + dx
		if nx < 0 || nx >= gridW {
			continue
		}

		// Ищем пол строго под этим пикселем (startY = y + 1)
		// Передаем y как splitY, чтобы искать только вниз от платформы
		gy := sgFindDynamicGroundY(grid, nx, y+1)

		dist := gy - y
		// Если до земли 1, 2 или 3 блока — заливаем монолитом
		if dist > 1 && dist <= 4 {
			for fillY := y + 1; fillY < gy; fillY++ {
				if fillY < gridH {
					grid[fillY][nx] = sgCellSolid
				}
			}
		}
	}
}

func sgExtendChainToGround(rng *rand.Rand, ch *sgStairChain, grid [][]int, tags *sgSkyTagGrid, inlets []sgInlet, splitY, stepW, _, _ int) {
	if len(ch.Steps) == 0 {
		return
	}

	gridW := len(grid[0])
	curr := ch.Steps[len(ch.Steps)-1]

	vJump := 6         // Прыжок вниз на 6 блоков
	hMin, hMax := 2, 4 // Сдвиг вбок минимален (2-4 блока)
	stopDist := 6      // Останавливаемся в 6 блоках от пола (чтобы игрок прошел под ней)

	// Find the nearest inlet in the same horizontal direction as the chain trend.
	// Left wings (TrendDir=-1) aim for an inlet to the left of the hub;
	// right wings (TrendDir=+1) aim for an inlet to the right. This prevents
	// both wings from converging on the same inlet and blocking each other's
	// jump arcs with interleaved stair steps.
	nearestInletX := -1
	if len(inlets) > 0 {
		bestDist := 1<<30 - 1
		for _, inlet := range inlets {
			// Prefer inlets in the chain's travel direction.
			inSameDir := (ch.TrendDir > 0 && inlet.CenterX >= curr.X) ||
				(ch.TrendDir < 0 && inlet.CenterX <= curr.X)
			d := abs(inlet.CenterX - curr.X)
			if inSameDir && d < bestDist {
				bestDist = d
				nearestInletX = inlet.CenterX
			}
		}
		// Fallback: use any nearest inlet if none on the preferred side.
		if nearestInletX < 0 {
			for _, inlet := range inlets {
				d := abs(inlet.CenterX - curr.X)
				if d < bestDist {
					bestDist = d
					nearestInletX = inlet.CenterX
				}
			}
		}
	}

	for {
		// Ищем землю под следующим шагом (учитывая туннели)
		// Steer toward the designated inlet so the chain terminus lands within
		// jump reach of an inlet entrance (~6 blocks horizontal).
		trendDir := ch.TrendDir
		if nearestInletX >= 0 && nearestInletX != curr.X {
			if nearestInletX > curr.X {
				trendDir = 1
			} else {
				trendDir = -1
			}
		}
		nx := curr.X + trendDir*(hMin+rng.Intn(hMax-hMin+1))
		nx = sgClamp(nx, 2, gridW-stepW-2)

		// Cap at splitY: inlet shafts make dynamic ground appear deep underground,
		// which would cause the chain to place steps inside the shaft and block
		// the player from fitting at the inlet entrance (player body is 4 tall).
		realGroundY := min(sgFindDynamicGroundY(grid, nx, splitY), splitY)

		// Рассчитываем Y следующей ступеньки
		ny := curr.Y + vJump

		// Если мы слишком близко к полу - ограничиваем высоту до предела прыжка
		if ny > realGroundY-stopDist {
			ny = realGroundY - stopDist
		}

		// Если ny выше или на уровне текущей точки (уперлись в холм) — стоп
		if ny <= curr.Y || ny >= realGroundY {
			sgSmoothConnectToGround(grid, curr.X, curr.Y, stepW) // Приклеиваем последнюю точку
			break
		}

		// Ставим платформу (one-way)
		sgWritePlatformRect(grid, nx, ny, stepW, 1)

		// Сразу проверяем плавное соединение с землей (1-2 блока)
		sgSmoothConnectToGround(grid, nx, ny, stepW)

		if tags != nil && ch.ObjectID > 0 {
			kind := sgSkyObjectSkyStepLeft
			if ch.Side == sgStairSideRight {
				kind = sgSkyObjectSkyStepRight
			}
			tags.MarkRect(nx, ny, stepW, 1, ch.ObjectID, kind)
		}

		curr = sgPoint{X: nx, Y: ny}
		ch.Steps = append(ch.Steps, curr)

		// Если мы уже на высоте прыжка от реальной земли — выходим
		if curr.Y >= realGroundY-stopDist {
			break
		}
		if len(ch.Steps) > 40 {
			break
		}
	}
}

// sgFillAllShaftPits теперь учитывает комнаты боссов и не засыпает их.
func sgFillAllShaftPits(grid [][]int, splitY int, bossRooms, miniBossRooms [][4]int) {
	gridH := len(grid)
	gridW := len(grid[0])

	// Вспомогательная функция для проверки: находится ли точка внутри комнаты босса
	isInsideBossRoom := func(x, y int) bool {
		for _, r := range append(bossRooms, miniBossRooms...) {
			cx, cy, rx, ry := r[0], r[1], r[2], r[3]
			// Границы комнаты (с небольшим запасом в 1 блок)
			if x >= cx-rx && x <= cx+rx && y >= cy-ry && y <= cy {
				return true
			}
		}
		return false
	}

	for x := 0; x < gridW; x++ {
		for y := gridH - 1; y > splitY; y-- {
			// Если мы уперлись в комнату босса — СТОП для этой колонки
			if isInsideBossRoom(x, y) {
				break
			}

			if grid[y][x] == sgCellSolid {
				continue
			}

			l, r := x, x
			for l > 0 && grid[y][l] != sgCellSolid {
				l--
			}
			for r < gridW-1 && grid[y][r] != sgCellSolid {
				r++
			}
			width := r - l

			// Если это узкая шахта — засыпаем
			if width < 16 {
				for ix := l + 1; ix < r; ix++ {
					// Даже при засыпке проверяем, не замуровываем ли мы край комнаты
					if !isInsideBossRoom(ix, y) {
						grid[y][ix] = sgCellSolid
					}
				}
			} else {
				// Вышли в широкий коридор
				break
			}
		}
	}
}

func sgAddSkyPenthouses(rng *rand.Rand, grid [][]int, tags *sgSkyTagGrid, hubs []sgSkyHub, cfg ProcGenConfig) []sgSkyHub {
	var extraHubs []sgSkyHub

	minTopMargin := 10
	stackChance := 0.6
	floorH := 2

	for _, hub := range hubs {
		currY, currX, currW := hub.Y, hub.X, hub.W

		for rng.Float64() < stackChance {
			vDist := 15 + rng.Intn(6)
			newY := currY - vDist - floorH
			if newY < minTopMargin {
				break
			}

			newW := currW - rng.Intn(4)
			if newW < 12 {
				newW = 12
			}
			newX := currX + (currW-newW)/2

			// Рисуем основной этаж (2 блока толщиной)
			objID := tags.NewObject(sgSkyObjectHub)
			sgWriteSolidRect(grid, newX, newY, newW, floorH)
			tags.MarkRect(newX, newY, newW, floorH, objID, sgSkyObjectHub)

			extraHubs = append(extraHubs, sgSkyHub{
				X: newX, Y: newY, W: newW, H: floorH, ObjectID: objID,
			})

			// ГЕНЕРИРУЕМ ЛЕСТНИЦЫ С ДВУХ СТОРОН
			landingW := 8
			gap := 3
			stepID := tags.NewObject(sgSkyObjectSkyStepLeft)

			for _, side := range []int{-1, 1} {
				var lx int
				if side == 1 {
					// Справа от самого широкого края
					lx = max(currX+currW, newX+newW) + gap
				} else {
					// Слева от самого широкого края
					lx = min(currX, newX) - landingW - gap
				}

				// Проверяем границы карты для этой стороны
				if lx > 2 && lx < len(grid[0])-landingW-2 {
					// Расставляем по 2 платформы на каждой стороне
					h1 := vDist / 3
					h2 := (vDist * 2) / 3

					for idx, dy := range []int{h1, h2} {
						py := currY - dy

						// Небольшая асимметрия для красоты:
						// Левая и правая стороны будут чуть-чуть смещены по высоте относительно друг друга
						if side == -1 {
							py -= 1
						}

						sgWritePlatformRect(grid, lx, py, landingW, 1)
						tags.MarkRect(lx, py, landingW, 1, stepID, sgSkyObjectSkyStepLeft)

						// Ступеньки на одной стороне идут лесенкой вглубь или наружу
						if idx == 0 {
							lx += side * 2
						} else {
							lx -= side * 1
						}
					}
				}
			}

			// Теперь этот этаж база для следующего
			currY, currX, currW = newY, newX, newW
		}
	}
	return extraHubs
}

func sgBuildInlets(gridW int, splitY int, cfg ProcGenConfig) []sgInlet {
	// Рассчитываем количество входов: примерно 1 вход на каждые 65 блоков ширины
	count := max(3, gridW/65)
	step := 1.0 / float64(count+1)

	// Inlet shaft must be wide enough for the player (sgPlayerW) with room to navigate.
	// Minimum 2×sgPlayerW so zig-zag platforms still leave passage room.
	width := max(sgPlayerW*2, sgScaleX(8, cfg))
	if width%2 != 0 {
		width++
	}

	inlets := make([]sgInlet, 0, count)
	for i := 1; i <= count; i++ {
		a := step * float64(i)
		cx := int(math.Round(float64(gridW) * a))
		cx = sgClamp(cx, width/2+1, gridW-width/2-2)
		inlets = append(inlets, sgInlet{CenterX: cx, Y: splitY, Width: width})
	}
	return inlets
}

// PopulateSkyAndObjects adaptively fills the upper layer with sky hubs and
// connected stair routes so traversal remains readable across room sizes.
func PopulateSkyAndObjects(rng *rand.Rand, grid [][]int, cfg ProcGenConfig, splitY int) {
	_ = sgPopulateSkyAndObjects(rng, grid, cfg, splitY)
}

type sgSkyWingRefs struct {
	LeftIdx  int
	RightIdx int
}

func sgPopulateSkyAndObjects(rng *rand.Rand, grid [][]int, cfg ProcGenConfig, splitY int) sgSkyDebugData {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return sgSkyDebugData{}
	}
	tags := sgNewSkyTagGrid(len(grid[0]), len(grid))
	return sgPopulateSkyAndObjectsWithTags(rng, grid, cfg, splitY, tags)
}

func sgPopulateSkyAndObjectsWithTags(rng *rand.Rand, grid [][]int, cfg ProcGenConfig, splitY int, tags *sgSkyTagGrid) sgSkyDebugData {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	cfg = cfg.fill()
	if len(grid) == 0 || len(grid[0]) == 0 {
		return sgSkyDebugData{}
	}

	gridW, gridH := len(grid[0]), len(grid)

	// Получаем параметры. 4-й аргумент (extra steps) игнорируем, он нам больше не нужен.
	skyBandH, skyArea, hubCount, extraStepCount := sgSkyDensity(cfg, gridW, gridH, splitY)

	dbg := sgSkyDebugData{
		SkyBandH: skyBandH,
		SkyArea:  skyArea,
		HubCount: hubCount,
		Tags:     tags,
	}

	if hubCount <= 0 || splitY <= 2 {
		return dbg
	}

	// Зона размещения основных островов
	skyTopMin := sgClamp(sgScaleY(25, cfg), 2, max(2, splitY-8))
	skyTopMax := sgClamp(sgScaleY(55, cfg), skyTopMin, max(skyTopMin, splitY-4))

	// Размещаем основные хабы (летающие острова)
	hubs := sgPlaceSkyHubs(rng, grid, tags, hubCount, skyTopMin, skyTopMax, splitY, cfg)
	dbg.Hubs = append(dbg.Hubs, hubs...)

	// Строим начальные крылья (3 ступени каждое). Полное продление до земли
	// с учётом позиций входов делается в sgEnsureGroundToSkyAccessibility, который
	// получает список inlets и направляет цепочку к ближайшему входу.
	stepW := max(sgPlayerW, sgScaleX(3, cfg))
	chains := make([]sgStairChain, 0, len(hubs)*2)
	wingRefs := make([]sgSkyWingRefs, len(hubs))
	for i := range wingRefs {
		wingRefs[i] = sgSkyWingRefs{LeftIdx: -1, RightIdx: -1}
	}
	for i, hub := range hubs {
		left := sgBuildSkyStaircaseWing(rng, grid, tags, hub, i, sgStairSideLeft, splitY, cfg)
		wingRefs[i].LeftIdx = len(chains)
		chains = append(chains, left)

		right := sgBuildSkyStaircaseWing(rng, grid, tags, hub, i, sgStairSideRight, splitY, cfg)
		wingRefs[i].RightIdx = len(chains)
		chains = append(chains, right)
	}
	_ = stepW

	extraPlaced := sgDistributeExtraSkySteps(rng, grid, tags, chains, extraStepCount, splitY, cfg)
	// gapPlatforms := sgAddSkyGapPlatformsWithTags(grid, tags, hubs, chains, wingRefs, splitY, cfg)
	dbg.ExtraPlaced = extraPlaced
	// dbg.GapPlatforms = gapPlatforms
	dbg.Chains = chains
	return dbg
}

func sgSkyDensity(cfg ProcGenConfig, gridW, gridH, splitY int) (skyBandH, skyArea, hubCount, extraStepCount int) {
	if gridW <= 0 || gridH <= 0 {
		return 0, 0, 0, 0
	}
	skyBandH = sgScaleY(70, cfg)
	if skyBandH < 8 {
		skyBandH = 8
	}
	if splitY > 2 && skyBandH >= splitY {
		skyBandH = splitY - 1
	}
	if skyBandH > gridH {
		skyBandH = gridH
	}
	skyArea = gridW * skyBandH
	hubCount = max(3, gridW/70)
	extraStepCount = gridW / 20
	return skyBandH, skyArea, hubCount, extraStepCount
}

func sgPlaceSkyHubs(rng *rand.Rand, grid [][]int, tags *sgSkyTagGrid, hubCount int, topMinY int, topMaxY int, splitY int, cfg ProcGenConfig) []sgSkyHub {
	gridW := len(grid[0])
	minW := max(8, sgScaleX(30, cfg))
	maxW := max(minW, sgScaleX(50, cfg))
	minH := max(2, sgScaleY(2, cfg))
	maxH := max(minH, sgScaleY(3, cfg))

	hubs := make([]sgSkyHub, 0, hubCount)
	for i := 0; i < hubCount; i++ {
		sectorStart := i * gridW / hubCount
		sectorEnd := (i + 1) * gridW / hubCount
		if i == hubCount-1 {
			sectorEnd = gridW
		}
		sectorSpan := max(1, sectorEnd-sectorStart)

		w := minW
		if maxW > minW {
			w += rng.Intn(maxW - minW + 1)
		}
		if w > sectorSpan-2 {
			w = max(4, sectorSpan-2)
		}
		if w >= gridW-2 {
			w = max(4, gridW-3)
		}
		if w <= 0 {
			continue
		}

		h := minH
		if maxH > minH {
			h += rng.Intn(maxH - minH + 1)
		}
		y := topMinY
		if topMaxY > topMinY {
			y += rng.Intn(topMaxY - topMinY + 1)
		}
		if y+h >= splitY-1 {
			y = max(1, splitY-h-2)
		}

		center := sectorStart + sectorSpan/2
		jitter := max(1, sectorSpan/4)
		center += rng.Intn(jitter*2+1) - jitter
		x := center - w/2
		minX := sectorStart + 1
		maxX := sectorEnd - w - 1
		if maxX < minX {
			minX = max(1, sectorStart)
			maxX = min(gridW-w-1, sectorEnd-w)
		}
		if maxX < minX {
			minX = sgClamp(minX, 1, max(1, gridW-w-1))
			maxX = minX
		}
		x = sgClamp(x, minX, maxX)

		objectID := 0
		if tags != nil {
			objectID = tags.NewObject(sgSkyObjectHub)
		}
		hub := sgSkyHub{X: x, Y: y, W: w, H: h, ObjectID: objectID}
		sgWriteSolidRect(grid, hub.X, hub.Y, hub.W, hub.H)
		if tags != nil {
			tags.MarkRect(hub.X, hub.Y, hub.W, hub.H, objectID, sgSkyObjectHub)
		}
		hubs = append(hubs, hub)
	}
	return hubs
}

func sgBuildSkyStaircaseWing(rng *rand.Rand, grid [][]int, tags *sgSkyTagGrid, hub sgSkyHub, hubIndex int, side sgStairSide, splitY int, cfg ProcGenConfig) sgStairChain {
	trend := -1
	kind := sgSkyObjectSkyStepLeft
	if side == sgStairSideRight {
		trend = 1
		kind = sgSkyObjectSkyStepRight
	}

	objectID := tags.NewObject(kind)
	chain := sgStairChain{
		HubIndex: hubIndex,
		Side:     side,
		TrendDir: trend,
		ObjectID: objectID,
	}

	gridW := len(grid[0])
	// Случайная ширина ступени: (в пределах [sgPlayerW, sgPlayerW+1])
	stepW := sgPlayerW + rng.Intn(2)

	// ПАРАМЕТРЫ КРУТИЗНЫ:
	dy := 6               // Прыжок вниз почти на максимум (7)
	dx := 2 + rng.Intn(2) // Сдвиг вбок всего на 2-3 блока

	anchorX := hub.X
	if side == sgStairSideRight {
		anchorX = hub.X + hub.W - 1
	}

	// Стартуем от нижнего края хаба (учитывая, что он теперь 2 блока толщиной)
	x, y := anchorX, hub.Y+hub.H-1

	// Генерируем только 3 начальные ступеньки.
	// Этого достаточно, чтобы "зацепить" цепочку, а остальное достроит
	// алгоритм доступности, который видит реальный пол (пещеры и т.д.)
	for i := 0; i < 3; i++ {
		x += trend * dx
		y += dy

		if y >= splitY-5 || x < 2 || x >= gridW-stepW-2 {
			break
		}

		sgWritePlatformRect(grid, x, y, stepW, 1)
		tags.MarkRect(x, y, stepW, 1, objectID, kind)
		chain.Steps = append(chain.Steps, sgPoint{X: x, Y: y})
	}

	return chain
}

func sgDistributeExtraSkySteps(rng *rand.Rand, grid [][]int, tags *sgSkyTagGrid, chains []sgStairChain, extraStepCount int, splitY int, cfg ProcGenConfig) int {
	if extraStepCount <= 0 || len(chains) == 0 {
		return 0
	}

	// Устанавливаем "небесную границу".
	// Не генерируем случайные шаги ниже, чем 10 блоков до земли,
	// чтобы оставить пространство для бега и не плодить горизонтальные цепочки.
	skyBoundaryY := splitY - 10

	placed := 0
	cursor := 0
	if len(chains) > 1 {
		cursor = rng.Intn(len(chains))
	}

	for placed < extraStepCount {
		ok := false
		for attempt := 0; attempt < len(chains)*10; attempt++ {
			idx := (cursor + attempt) % len(chains)
			chain := &chains[idx]

			// Если цепочка уже внизу, не добавляем ей мусора
			if len(chain.Steps) > 0 {
				lastY := chain.Steps[len(chain.Steps)-1].Y
				if lastY >= skyBoundaryY {
					continue
				}
			}

			// Пытаемся добавить шаг
			if sgAppendExtraSkyStep(rng, grid, tags, chain, splitY, cfg) {
				// Дополнительная проверка безопасности: если новый шаг все же
				// оказался слишком низко, мы его удаляем.
				lastIdx := len(chain.Steps) - 1
				newStep := chain.Steps[lastIdx]

				if newStep.Y >= skyBoundaryY {
					// Удаляем блок из сетки и из тегов
					stepW := max(sgPlayerW, sgScaleX(3, cfg))
					for dx := 0; dx < stepW; dx++ {
						nx := newStep.X + dx
						if nx >= 0 && nx < len(grid[0]) {
							grid[newStep.Y][nx] = sgCellAir
							if tags != nil {
								tags.ClearCell(nx, newStep.Y)
							}
						}
					}
					// Убираем из массива цепочки
					chain.Steps = chain.Steps[:lastIdx]
					continue
				}

				placed++
				cursor = (idx + 1) % len(chains)
				ok = true
				break
			}
		}
		if !ok {
			break
		}
	}
	return placed
}

func sgAddSkyGapPlatforms(grid [][]int, hubs []sgSkyHub, chains []sgStairChain, refs []sgSkyWingRefs, splitY int, cfg ProcGenConfig) int {
	return sgAddSkyGapPlatformsWithTags(grid, nil, hubs, chains, refs, splitY, cfg)
}

func sgAddSkyGapPlatformsWithTags(grid [][]int, tags *sgSkyTagGrid, hubs []sgSkyHub, chains []sgStairChain, refs []sgSkyWingRefs, splitY int, cfg ProcGenConfig) int {
	if len(hubs) < 2 || len(chains) == 0 || len(refs) != len(hubs) {
		return 0
	}

	indices := make([]int, len(hubs))
	for i := range hubs {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		ai := hubs[indices[i]].X + hubs[indices[i]].W/2
		aj := hubs[indices[j]].X + hubs[indices[j]].W/2
		if ai == aj {
			return indices[i] < indices[j]
		}
		return ai < aj
	})

	stepW := max(sgPlayerW, sgScaleX(3, cfg))
	restW := max(3, sgScaleX(6, cfg))
	gapThreshold := max(8, sgScaleX(40, cfg))
	placed := 0

	for i := 0; i < len(indices)-1; i++ {
		leftHub := indices[i]
		rightHub := indices[i+1]
		leftWingIdx := refs[leftHub].RightIdx
		rightWingIdx := refs[rightHub].LeftIdx
		if leftWingIdx < 0 || leftWingIdx >= len(chains) || rightWingIdx < 0 || rightWingIdx >= len(chains) {
			continue
		}

		leftWing := chains[leftWingIdx]
		rightWing := chains[rightWingIdx]
		if len(leftWing.Steps) == 0 || len(rightWing.Steps) == 0 {
			continue
		}

		leftEnd := leftWing.Steps[len(leftWing.Steps)-1]
		rightEnd := rightWing.Steps[len(rightWing.Steps)-1]
		leftEdge := leftEnd.X + stepW - 1
		rightEdge := rightEnd.X
		gap := rightEdge - leftEdge - 1
		if gap <= gapThreshold {
			continue
		}

		px := leftEdge + gap/2 - restW/2
		minX := leftEdge + 1
		maxX := rightEdge - restW
		if maxX < minX {
			continue
		}
		px = sgClamp(px, minX, maxX)
		py := sgClamp((leftEnd.Y+rightEnd.Y)/2, 2, max(2, splitY-2))

		ok := false
		for _, dy := range []int{0, -2, 2, -4, 4} {
			ty := py + dy
			if !sgCanPlaceSkyStep(grid, px, ty, restW) {
				continue
			}
			sgWritePlatformRect(grid, px, ty, restW, 1)
			if tags != nil {
				objID := tags.NewObject(sgSkyObjectSkyStepRight)
				tags.MarkRect(px, ty, restW, 1, objID, sgSkyObjectSkyStepRight)
			}
			ok = true
			break
		}
		if !ok {
			// Last resort: force a small bridge platform in the midpoint.
			forceY := sgClamp(py, 1, max(1, splitY-1))
			sgWritePlatformRect(grid, px, forceY, restW, 1)
			if tags != nil {
				objID := tags.NewObject(sgSkyObjectSkyStepRight)
				tags.MarkRect(px, forceY, restW, 1, objID, sgSkyObjectSkyStepRight)
			}
		}
		placed++
	}
	return placed
}

func sgAppendExtraSkyStep(rng *rand.Rand, grid [][]int, tags *sgSkyTagGrid, chain *sgStairChain, splitY int, cfg ProcGenConfig) bool {
	if chain == nil || len(chain.Steps) == 0 {
		return false
	}

	// Случайная ширина ступени
	stepW := sgPlayerW + rng.Intn(2)
	dxMin := max(stepW+1, sgScaleX(7, cfg))
	dxMax := max(dxMin, sgScaleX(10, cfg))
	dy := max(2, sgScaleY(6, cfg))
	minGap := max(1, sgScaleX(4, cfg))
	projectionH := max(2, sgScaleY(10, cfg))
	kind := sgSkyObjectSkyStepLeft
	if chain.Side == sgStairSideRight {
		kind = sgSkyObjectSkyStepRight
	}

	base := chain.Steps[len(chain.Steps)-1]
	for try := 0; try < 18; try++ {
		dxAbs := dxMin
		if dxMax > dxMin {
			dxAbs += rng.Intn(dxMax - dxMin + 1)
		}

		dir := chain.TrendDir
		if try%6 == 5 {
			dir = -dir
		}
		nx := base.X + dir*dxAbs
		ny := base.Y + dy
		if ny >= splitY {
			ny = base.Y
		}
		placedX, ok := sgTryPlaceSkyStepConstrained(grid, tags, nx, ny, stepW, chain.TrendDir, chain.ObjectID, kind, projectionH, minGap, 0, dxMax, &base, splitY)
		if !ok {
			continue
		}
		chain.Steps = append(chain.Steps, sgPoint{X: placedX, Y: ny})
		chain.ExtraSteps++
		return true
	}

	// Last-resort: contiguous side ledge attached to the chain endpoint.
	for _, dir := range []int{chain.TrendDir, -chain.TrendDir} {
		nx := base.X + dir*(stepW+minGap)
		ny := base.Y
		placedX, ok := sgTryPlaceSkyStepConstrained(grid, tags, nx, ny, stepW, chain.TrendDir, chain.ObjectID, kind, projectionH, minGap, 0, dxMax, &base, splitY)
		if !ok {
			continue
		}
		chain.Steps = append(chain.Steps, sgPoint{X: placedX, Y: ny})
		chain.ExtraSteps++
		return true
	}
	return false
}

func sgTryPlaceSkyStepConstrained(grid [][]int, tags *sgSkyTagGrid, x, y, stepW int, biasDir int, objectID int, kind sgSkyObjectKind, projectionH int, minGap int, minDX int, maxDX int, prev *sgPoint, splitY int) (int, bool) {
	if y < 1 || y >= splitY {
		return 0, false
	}
	offsets := []int{0, -2, 2, -4, 4}
	if biasDir > 0 {
		offsets = []int{0, 2, 4, -2, -4}
	}
	for _, off := range offsets {
		nx := x + off
		if !sgCanPlaceSkyStep(grid, nx, y, stepW) {
			continue
		}
		if prev != nil {
			if biasDir < 0 && nx >= prev.X {
				continue
			}
			if biasDir > 0 && nx <= prev.X {
				continue
			}
			if sgHorizontalGap(prev.X, nx, stepW) < minGap {
				continue
			}
			dx := sgAbsInt(nx - prev.X)
			if minDX > 0 && dx < minDX {
				continue
			}
			if maxDX > 0 && dx > maxDX {
				continue
			}
		}
		if !sgSkyProjectionClear(grid, tags, nx, y, stepW, projectionH, objectID, splitY) {
			continue
		}
		// One-way sky step: write sgCellPlatform so the player can jump through
		// from below and land on top. Tags remain for anti-stacking checks.
		sgWritePlatformRect(grid, nx, y, stepW, 1)
		if tags != nil && objectID > 0 {
			tags.MarkRect(nx, y, stepW, 1, objectID, kind)
		}
		return nx, true
	}
	return 0, false
}

func sgHorizontalGap(aX, bX, width int) int {
	if aX <= bX {
		return bX - (aX + width)
	}
	return aX - (bX + width)
}

func sgSkyProjectionClear(grid [][]int, tags *sgSkyTagGrid, x, y, stepW, projectionH, objectID, splitY int) bool {
	if y <= 0 {
		return true
	}
	gridW := len(grid[0])
	x0 := sgClamp(x-1, 0, gridW-1)
	x1 := sgClamp(x+stepW, 0, gridW-1)
	y0 := max(0, y-projectionH)
	for yy := y0; yy < y; yy++ {
		if yy >= splitY {
			continue
		}
		for xx := x0; xx <= x1; xx++ {
			if grid[yy][xx] != sgCellSolid {
				continue
			}
			tag := sgSkyTag{}
			if tags != nil {
				tag = tags.At(xx, yy)
			}
			if objectID > 0 && tag.ObjectID == objectID {
				continue
			}
			// Unknown or foreign solid counts as blocked jump line.
			return false
		}
	}
	return true
}

func sgCanPlaceSkyStep(grid [][]int, x, y, stepW int) bool {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return false
	}
	gridW := len(grid[0])
	gridH := len(grid)
	if y < 1 || y >= gridH-1 {
		return false
	}
	if x < 1 || x+stepW >= gridW {
		return false
	}

	// At least one cell must be air or backwall (not solid and not another platform).
	hasPlaceable := false
	for xx := x; xx < x+stepW; xx++ {
		c := grid[y][xx]
		if c != sgCellSolid && c != sgCellPlatform {
			hasPlaceable = true
			break
		}
	}
	return hasPlaceable
}

func sgWriteSolidRect(grid [][]int, x0, y0, w, h int) {
	for y := y0; y < y0+h; y++ {
		if y < 0 || y >= len(grid) {
			continue
		}
		for x := x0; x < x0+w; x++ {
			if x < 0 || x >= len(grid[y]) {
				continue
			}
			grid[y][x] = sgCellSolid
		}
	}
}

// sgWritePlatformRect fills a rect with sgCellPlatform (one-way passable from below).
// Before writing each row it checks whether placing that row would seal an air
// pocket above the platform that the player cannot escape from (no horizontal or
// upward exit reachable via BFS). Trapped rows are silently skipped.
func sgWritePlatformRect(grid [][]int, x0, y0, w, h int) {
	gridH := len(grid)
	if gridH == 0 {
		return
	}
	gridW := len(grid[0])
	for y := y0; y < y0+h; y++ {
		if y < 0 || y >= gridH {
			continue
		}
		// Air-pocket check: skip this platform row if it would trap the player.
		if sgPlatformTrapsAirPocket(grid, x0, y, w) {
			continue
		}
		for x := x0; x < x0+w; x++ {
			if x < 0 || x >= gridW {
				continue
			}
			grid[y][x] = sgCellPlatform
		}
	}
}

// sgPlatformTrapsAirPocket returns true when placing a one-way platform at
// (x0, y, w, 1) would create an inescapable pocket directly above it.
//
// The pocket is "sealed" when a BFS starting from the cells just above the
// platform (row y-1) cannot exit the platform's column range horizontally,
// cannot reach open sky above (y < y-sgPlayerH*2), and explores fewer than
// maxPocketNodes cells total (large spaces are always considered safe).
//
// The BFS treats row y as an impassable floor (the new platform) and all
// sgCellSolid cells as walls.  Existing one-way platforms and air cells are
// passable so the player can jump between stacked platforms.
// sgPlatformTrapsAirPocket проверяет, не создаст ли платформа ловушку.
// Теперь учитывается ширина игрока (sgPlayerW) и высота (sgPlayerH).
func sgPlatformTrapsAirPocket(grid [][]int, x0, y, w int) bool {
	if y <= sgPlayerH {
		return false
	}
	gridH, gridW := len(grid), len(grid[0])

	// Проверяем: может ли игрок вообще стоять на этой платформе хоть в одной позиции?
	// Для этого нужно найти X, где под игроком (3 блока) есть эта платформа,
	// а в пространстве 3x5 над ней нет Solid блоков.
	startPosFound := false
	queue := []sgPoint{}
	visited := make(map[sgPoint]bool)

	for tx := x0 - sgPlayerW + 1; tx <= x0+w-1; tx++ {
		// Игрок стоит в tx..tx+sgPlayerW-1. Проверяем границы.
		if tx < 1 || tx+sgPlayerW > gridW-1 {
			continue
		}
		// Проверяем "голову" и "тело" (5 блоков вверх)
		canFit := true
		for py := 0; py < sgPlayerH; py++ {
			for px := 0; px < sgPlayerW; px++ {
				if grid[y-1-py][tx+px] == sgCellSolid {
					canFit = false
					break
				}
			}
			if !canFit {
				break
			}
		}
		if canFit {
			p := sgPoint{X: tx, Y: y - 1}
			queue = append(queue, p)
			visited[p] = true
			startPosFound = true
		}
	}

	// Если игроку негде даже встать на этой платформе (голова в потолке) - это ловушка
	if !startPosFound {
		return true
	}

	// BFS: Пытаемся вывести "толстого" игрока (3x5) из этой зоны
	head := 0
	for head < len(queue) {
		curr := queue[head]
		head++

		// Условие успеха: если мы ушли достаточно далеко от платформы по горизонтали
		// или поднялись достаточно высоко
		if curr.X < x0-sgPlayerW || curr.X > x0+w || curr.Y < y-sgPlayerH-2 {
			return false
		}

		// Проверяем 4 направления для "тела" игрока
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := curr.X+d[0], curr.Y+d[1]
			np := sgPoint{X: nx, Y: ny}

			if ny < sgPlayerH || ny >= gridH-1 || nx < 1 || nx+sgPlayerW > gridW-1 || visited[np] {
				continue
			}

			// Проверка коллизии тела 3x5 в новой точке
			collision := false
			for py := 0; py < sgPlayerH; py++ {
				for px := 0; px < sgPlayerW; px++ {
					if grid[ny-py][nx+px] == sgCellSolid {
						collision = true
						break
					}
				}
				if collision {
					break
				}
			}

			// Новая платформа для нас - тоже пол (нельзя сквозь неё падать в рамках проверки кармана)
			if ny == y {
				collision = true
			}

			if !collision {
				visited[np] = true
				queue = append(queue, np)
			}
		}
		if len(queue) > 200 {
			return false
		} // Слишком большое пространство - не ловушка
	}

	return true // Выхода для тела 3x5 не найдено
}

func sgPlaceHubs(rng *rand.Rand, grid [][]int, splitY int, cfg ProcGenConfig) []sgHub {
	gridW := len(grid[0])
	gridH := len(grid)

	// Hub sizes: moderately larger than original for comfortable cave feel.
	// Boss rooms (sgPlaceBossRooms) provide the truly large boss-scale spaces.
	hubMinW := max(14, sgScaleX(22, cfg))
	hubMaxW := max(hubMinW+4, sgScaleX(38, cfg))
	hubMinH := max(7, sgScaleY(10, cfg)) // -15% again from max(8,12)
	hubMaxH := max(hubMinH+3, sgScaleY(17, cfg))
	minGap := sgMinHubGap(cfg)

	// Вычисляем масштаб площади, чтобы понять, сколько комнат нужно сгенерировать
	area := gridW * (gridH - splitY)
	baseArea := sgBaseGridW * (sgBaseGridH / 2)
	areaScale := float64(area) / float64(baseArea)

	// 6..8 хабов для базовой площади * множитель
	baseTarget := 6 + rng.Intn(3)
	target := int(math.Round(float64(baseTarget) * areaScale))

	yMin := splitY + max(2, sgScaleY(10, cfg))
	yMax := max(yMin, gridH-hubMaxH-2)

	hubs := make([]sgHub, 0, target)
	maxAttempts := int(1600 * areaScale) // Даем больше попыток на размещение
	for attempt := 0; attempt < maxAttempts && len(hubs) < target; attempt++ {
		w := hubMinW + rng.Intn(hubMaxW-hubMinW+1)
		h := hubMinH + rng.Intn(hubMaxH-hubMinH+1)
		xMax := gridW - w - 2
		if xMax <= 2 || yMax <= yMin {
			break
		}
		x := 2 + rng.Intn(xMax-1)
		y := yMin + rng.Intn(yMax-yMin+1)
		cand := sgHub{X: x, Y: y, W: w, H: h}
		if !sgHubFits(hubs, cand, minGap) {
			continue
		}
		hubs = append(hubs, cand)
		sgCarveRect(grid, cand.X, cand.Y, cand.W, cand.H)
	}
	return hubs
}

func sgHubFits(hubs []sgHub, candidate sgHub, gap int) bool {
	for _, hub := range hubs {
		if sgRectIntersectsInflated(candidate, hub, gap) {
			return false
		}
	}
	return true
}

func sgRectIntersectsInflated(a sgHub, b sgHub, pad int) bool {
	ax0 := a.X - pad
	ay0 := a.Y - pad
	ax1 := a.X + a.W + pad
	ay1 := a.Y + a.H + pad
	bx0 := b.X
	by0 := b.Y
	bx1 := b.X + b.W
	by1 := b.Y + b.H
	return ax0 < bx1 && ax1 > bx0 && ay0 < by1 && ay1 > by0
}

func sgNearestHub(inlet sgInlet, hubs []sgHub) int {
	best := 0
	bestDist := math.MaxFloat64
	start := sgPoint{X: inlet.CenterX, Y: inlet.Y}
	for i, hub := range hubs {
		c := hub.Center()
		d := sgDistanceSq(start, c)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

func sgConnectHubMST(rng *rand.Rand, grid [][]int, hubs []sgHub, radius int, edges map[[2]int]bool) {
	if len(hubs) <= 1 {
		return
	}
	inTree := map[int]bool{0: true}
	for len(inTree) < len(hubs) {
		bestA := -1
		bestB := -1
		bestDist := math.MaxFloat64
		for a := range inTree {
			for b := 0; b < len(hubs); b++ {
				if inTree[b] {
					continue
				}
				d := sgDistanceSq(hubs[a].Center(), hubs[b].Center())
				if d < bestDist {
					bestDist = d
					bestA = a
					bestB = b
				}
			}
		}
		if bestA < 0 || bestB < 0 {
			break
		}
		sgCarveBeziez(rng, grid, hubs[bestA].Center(), hubs[bestB].Center(), radius)
		edges[sgEdgeKey(bestA, bestB)] = true
		inTree[bestB] = true
	}
}

func sgPickLoopHubPair(rng *rand.Rand, hubCount int, existing map[[2]int]bool) (int, int) {
	if hubCount < 2 {
		return -1, -1
	}
	for try := 0; try < 64; try++ {
		a := rng.Intn(hubCount)
		b := rng.Intn(hubCount)
		if a == b {
			continue
		}
		key := sgEdgeKey(a, b)
		if existing[key] {
			continue
		}
		existing[key] = true
		return a, b
	}
	return -1, -1
}

func sgCarveInletShaft(grid [][]int, inlet sgInlet, bottomY int) sgShaft {
	gridW := len(grid[0])
	gridH := len(grid)

	width := inlet.Width
	if width < 4 {
		width = 4
	}
	if width%2 != 0 {
		width++
	}

	left := inlet.CenterX - width/2
	right := left + width - 1
	if left < 1 {
		left = 1
		right = left + width - 1
	}
	if right > gridW-2 {
		right = gridW - 2
		left = right - width + 1
	}

	topY := max(0, inlet.Y-1)
	bottomY = sgClamp(bottomY, inlet.Y+3, gridH-2)
	for y := topY; y <= bottomY; y++ {
		for x := left; x <= right; x++ {
			sgCarveCell(grid, x, y)
			if y < inlet.Y {
				grid[y][x] = sgCellAir
			}
		}
	}

	sgApplyEntranceFlare(grid, inlet.CenterX, inlet.Y, inlet.Width)

	return sgShaft{
		CenterX: inlet.CenterX,
		Left:    left,
		Right:   right,
		TopY:    topY,
		BottomY: bottomY,
	}
}

func sgAddInletZigZagPlatforms(grid [][]int, shaft sgShaft, cfg ProcGenConfig, tags *sgSkyTagGrid) {
	// step must be > sgPlayerH so there are exactly sgPlayerH rows of air between
	// consecutive platforms — enough for the player to stand.
	step := sgPlayerH + 1
	ledgeW := max(sgPlayerW, sgScaleX(3, cfg))
	sideGap := max(1, sgScaleX(3, cfg))
	shaftW := shaft.Right - shaft.Left + 1
	if ledgeW >= shaftW {
		ledgeW = max(1, shaftW-1)
	}

	sideLeft := true
	for y := shaft.BottomY - step + 1; y >= shaft.TopY+3; y -= step {
		leftX := shaft.Left
		rightX := shaft.Right - ledgeW + 1
		if rightX-leftX < sideGap {
			mid := (leftX + rightX) / 2
			leftX = sgClamp(mid-sideGap/2, shaft.Left, rightX)
			rightX = sgClamp(leftX+sideGap, leftX, shaft.Right-ledgeW+1)
		}
		x0 := leftX
		if !sideLeft {
			x0 = rightX
		}
		x0 = sgClamp(x0, shaft.Left, shaft.Right-ledgeW+1)
		objectID := 0
		if tags != nil {
			objectID = tags.NewObject(sgSkyObjectShaftStep)
		}
		for dx := 0; dx < ledgeW; dx++ {
			x := x0 + dx
			if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
				continue
			}
			if sgIsPassable(grid[y][x]) {
				grid[y][x] = sgCellPlatform // one-way: passable from below
				if tags != nil && objectID > 0 {
					tags.MarkCell(x, y, objectID, sgSkyObjectShaftStep)
				}
			}
			// Guarantee sgPlayerH rows of headroom above the platform so the
			// player can stand here. Clear any solid that was
			// placed by other passes (e.g. sgAddInternalLedges).
			for pDy := 1; pDy <= sgPlayerH; pDy++ {
				hy := y - pDy
				if hy >= 0 && hy < len(grid) && grid[hy][x] == sgCellSolid {
					grid[hy][x] = sgCellBackwall
				}
			}
		}
		sideLeft = !sideLeft
	}
}

// sgAddReturnLoops returns the set of surface X positions it used so the caller
// can pass them to sgAddExitTunnels, preventing shaft overlap.
func sgAddReturnLoops(rng *rand.Rand, grid [][]int, splitY int, inlets []sgInlet, hubs []sgHub, carveRadius int, cfg ProcGenConfig, tags *sgSkyTagGrid) map[int]bool {
	count := max(2, len(hubs)/4)
	deep := sgDeepestHubIndices(hubs, count)
	exitW := max(4, sgScaleX(6, cfg))
	if exitW%2 != 0 {
		exitW++
	}
	usedTargets := map[int]bool{}

	for _, hubIdx := range deep {
		targetX, ok := sgPickGroundExitX(rng, len(grid[0]), exitW, inlets, usedTargets)
		if !ok {
			targetX = hubs[hubIdx].Center().X
		}
		usedTargets[targetX] = true

		start := hubs[hubIdx].Center()
		if start.Y < splitY+1 {
			start.Y = splitY + 1
		}
		end := sgRunUpwardWalker(rng, grid, start, targetX, splitY, carveRadius)
		sgCarveLine(grid, sgPoint{X: end.X, Y: splitY}, sgPoint{X: targetX, Y: splitY}, carveRadius)
		sgOpenSurfaceExit(grid, targetX, splitY, exitW)

		exitShaft := sgShaft{
			CenterX: targetX,
			Left:    targetX - 2,
			Right:   targetX + 2,
			TopY:    splitY,
			BottomY: start.Y,
		}
		sgAddInletZigZagPlatforms(grid, exitShaft, cfg, tags)
	}
	return usedTargets
}

// sgAddExitTunnels creates additional surface-to-underground shafts with zigzag
// platforms. Each shaft bottom is connected via a Bezier tunnel to the nearest
// hub so the shaft is never an isolated dead-end. preUsed is the set of X
// positions already occupied by return loop shafts — prevents overlap.
func sgAddExitTunnels(rng *rand.Rand, grid [][]int, splitY int, cfg ProcGenConfig, inlets []sgInlet, tags *sgSkyTagGrid, preUsed map[int]bool) {
	gridW := len(grid[0])
	gridH := len(grid)

	shaftCount := max(3, len(grid[0])/80)
	shaftW := max(6, sgScaleX(8, cfg))
	if shaftW%2 != 0 {
		shaftW++
	}
	stepY := sgPlayerH + 1 // gap of sgPlayerH air rows between platforms
	stepW := max(sgPlayerW, sgScaleX(4, cfg))
	if stepW >= shaftW {
		stepW = max(2, shaftW-2)
	}

	// Seed used map with all return-loop shaft positions so we never overlap them.
	used := map[int]bool{}
	for x := range preUsed {
		used[x] = true
	}
	for i := 0; i < shaftCount; i++ {
		centerX, ok := sgPickGroundExitX(rng, gridW, shaftW, inlets, used)
		if !ok {
			break
		}
		used[centerX] = true
		left := sgClamp(centerX-shaftW/2, 1, max(1, gridW-shaftW-1))
		right := left + shaftW - 1
		bottomY := gridH - 2

		for y := max(0, splitY-1); y <= bottomY; y++ {
			for x := left; x <= right; x++ {
				// Also clear any existing platforms left by an earlier pass (e.g.
				// a return-loop shaft that happened to be carved in the same column
				// before the used-map exclusion was introduced). Without this, the
				// column could end up with two independent sets of zigzag steps.
				if grid[y][x] == sgCellPlatform {
					grid[y][x] = sgCellAir
				}
				sgCarveCell(grid, x, y)
				if y < splitY {
					grid[y][x] = sgCellAir
				}
			}
		}
		sgOpenSurfaceExit(grid, centerX, splitY, shaftW)

		objectID := tags.NewObject(sgSkyObjectShaftStep)

		sideLeft := true
		firstY := bottomY - stepY + 1
		if firstY > splitY {
			sgPlaceShaftStep(grid, left, right, firstY, stepW, sideLeft, tags, objectID)
			sideLeft = !sideLeft
		}

		for y := firstY - stepY; y >= splitY+3; y -= stepY {
			sgPlaceShaftStep(grid, left, right, y, stepW, sideLeft, tags, objectID)
			sideLeft = !sideLeft
		}

		// Connect the shaft to the nearest existing air pocket by scanning
		// horizontally outward at several depths. We carve a short straight
		// horizontal corridor rather than a diagonal tunnel — keeps the geometry
		// clean and avoids cutting through solid walls at odd angles.
		sgConnectShaftToNearestVoid(grid, left, right, splitY, bottomY)
	}
}

// sgConnectShaftToNearestVoid scans horizontally outward from the shaft at
// several underground depths and carves a straight horizontal corridor to the
// nearest passable cell that is already reachable from the surface. Checking
// reachability prevents connecting to isolated pockets that themselves have no
// exit — only cells that a player could already reach before the tunnel is dug
// are eligible targets.
func sgConnectShaftToNearestVoid(grid [][]int, shaftLeft, shaftRight, splitY, shaftBottom int) {
	gridW := len(grid[0])
	gridH := len(grid)
	maxReach := gridW / 3 // don't search the whole map

	// Build a reachability map from the surface so we can filter candidates.
	canReach := make([][]bool, gridH)
	for i := range canReach {
		canReach[i] = make([]bool, gridW)
	}
	sgMarkReachable(grid, canReach, splitY)

	// Try a few Y levels spread across the shaft depth (skip the very top and bottom).
	totalDepth := shaftBottom - splitY
	scanYs := []int{
		splitY + totalDepth*2/5,
		splitY + totalDepth*3/5,
		splitY + totalDepth*4/5,
	}

	connected := false
	for _, scanY := range scanYs {
		if scanY < splitY+2 || scanY >= gridH-1 {
			continue
		}
		// Scan left — accept first passable cell that is surface-reachable.
		for dx := 1; dx <= maxReach; dx++ {
			nx := shaftLeft - dx
			if nx < 1 {
				break
			}
			if sgIsPassable(grid[scanY][nx]) && canReach[scanY][nx] {
				// Found a reachable void — carve corridor from shaft edge to here.
				for cx := nx; cx < shaftLeft; cx++ {
					if grid[scanY][cx] == sgCellSolid {
						grid[scanY][cx] = sgCellBackwall
					}
				}
				connected = true
				break
			}
		}
		// Scan right.
		for dx := 1; dx <= maxReach; dx++ {
			nx := shaftRight + dx
			if nx >= gridW-1 {
				break
			}
			if sgIsPassable(grid[scanY][nx]) && canReach[scanY][nx] {
				for cx := shaftRight + 1; cx <= nx; cx++ {
					if grid[scanY][cx] == sgCellSolid {
						grid[scanY][cx] = sgCellBackwall
					}
				}
				connected = true
				break
			}
		}
		if connected {
			break
		}
	}
}

func sgPlaceShaftStep(grid [][]int, left, right, y, stepW int, sideLeft bool, tags *sgSkyTagGrid, objID int) {
	x0 := left
	if !sideLeft {
		x0 = right - stepW + 1
	}
	gridH := len(grid)
	gridW := len(grid[0])
	for dx := 0; dx < stepW; dx++ {
		nx := x0 + dx
		if nx < 0 || nx >= gridW || y < 0 || y >= gridH {
			continue
		}
		grid[y][nx] = sgCellPlatform // one-way: passable from below
		if tags != nil && objID > 0 {
			tags.MarkCell(nx, y, objID, sgSkyObjectShaftStep)
		}
		// Clear sgPlayerH rows above so the player can stand.
		for pDy := 1; pDy <= sgPlayerH; pDy++ {
			hy := y - pDy
			if hy >= 0 && grid[hy][nx] == sgCellSolid {
				grid[hy][nx] = sgCellBackwall
			}
		}
	}
}

func sgDeepestHubIndices(hubs []sgHub, count int) []int {
	idx := make([]int, len(hubs))
	for i := range hubs {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool {
		ci := hubs[idx[i]].Center()
		cj := hubs[idx[j]].Center()
		if ci.Y == cj.Y {
			return ci.X < cj.X
		}
		return ci.Y > cj.Y
	})
	if count > len(idx) {
		count = len(idx)
	}
	return idx[:count]
}

func sgPickGroundExitX(rng *rand.Rand, gridW int, width int, inlets []sgInlet, used map[int]bool) (int, bool) {
	lo := width/2 + 2
	hi := gridW - width/2 - 3
	if hi < lo {
		return 0, false
	}

	for try := 0; try < 128; try++ {
		cx := lo + rng.Intn(hi-lo+1)
		if sgGroundExitAllowed(cx, width, inlets, used) {
			return cx, true
		}
	}
	for cx := lo; cx <= hi; cx++ {
		if sgGroundExitAllowed(cx, width, inlets, used) {
			return cx, true
		}
	}
	return 0, false
}

func sgGroundExitAllowed(centerX, width int, inlets []sgInlet, used map[int]bool) bool {
	for x := range used {
		if sgAbsInt(centerX-x) < width*2 {
			return false
		}
	}
	left := centerX - width/2
	right := left + width - 1
	for _, inlet := range inlets {
		iLeft := inlet.CenterX - inlet.Width/2
		iRight := iLeft + inlet.Width - 1
		if left <= iRight+1 && right >= iLeft-1 {
			return false
		}
	}
	return true
}

func sgAbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sgRunUpwardWalker(rng *rand.Rand, grid [][]int, start sgPoint, targetX, splitY, radius int) sgPoint {
	p := start
	gridW := len(grid[0])
	gridH := len(grid)
	maxSteps := gridW * gridH
	for step := 0; step < maxSteps && p.Y > splitY; step++ {
		sgCarveDisk(grid, p.X, p.Y, radius)

		if rng.Float64() < 0.60 {
			p.Y--
		} else {
			dir := 1
			if p.X > targetX {
				dir = -1
			} else if p.X == targetX && rng.Intn(2) == 0 {
				dir = -1
			}
			if rng.Float64() < 0.15 {
				dir = -dir
			}
			p.X += dir
		}

		p.X = sgClamp(p.X, 2, gridW-3)
		p.Y = sgClamp(p.Y, splitY, gridH-2)
	}
	sgCarveDisk(grid, p.X, p.Y, radius)
	return p
}

func sgOpenSurfaceExit(grid [][]int, centerX int, splitY int, width int) {
	gridH := len(grid)
	left := centerX - width/2
	right := left + width - 1

	// Прорезаем стандартный прямоугольник
	for y := splitY - 1; y <= splitY+2; y++ {
		if y < 0 || y >= gridH {
			continue
		}
		for x := left; x <= right; x++ {
			sgCarveCell(grid, x, y)
			if y < splitY {
				grid[y][x] = sgCellAir
			}
		}
	}

	// Применяем закругление
	sgApplyEntranceFlare(grid, centerX, splitY, width)
}

// sgApplyEntranceFlare делает воронкообразное расширение у выхода на поверхность.
func sgApplyEntranceFlare(grid [][]int, centerX int, splitY int, width int) {
	gridW := len(grid[0])
	left := centerX - width/2
	right := left + width - 1

	// Радиус закругления (на сколько блоков расширяем в стороны)
	flare := 2

	// 1. Уровень splitY (поверхность): самое широкое место
	for x := left - flare; x <= right+flare; x++ {
		if x >= 1 && x < gridW-1 {
			sgCarveCell(grid, x, splitY)
			// Над входом гарантированно должен быть воздух
			if splitY > 0 {
				grid[splitY-1][x] = sgCellAir
			}
		}
	}

	// 2. Уровень splitY + 1 (чуть глубже): среднее расширение
	for x := left - 1; x <= right+1; x++ {
		if x >= 1 && x < gridW-1 {
			sgCarveCell(grid, x, splitY+1)
		}
	}
}

func sgCarveRect(grid [][]int, x0, y0, w, h int) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			sgCarveCell(grid, x, y)
		}
	}
}

func sgCarveL(rng *rand.Rand, grid [][]int, a, b sgPoint, radius int) {
	if rng.Intn(2) == 0 {
		sgCarveLine(grid, sgPoint{X: a.X, Y: a.Y}, sgPoint{X: b.X, Y: a.Y}, radius)
		sgCarveLine(grid, sgPoint{X: b.X, Y: a.Y}, sgPoint{X: b.X, Y: b.Y}, radius)
		return
	}
	sgCarveLine(grid, sgPoint{X: a.X, Y: a.Y}, sgPoint{X: a.X, Y: b.Y}, radius)
	sgCarveLine(grid, sgPoint{X: a.X, Y: b.Y}, sgPoint{X: b.X, Y: b.Y}, radius)
}

// sgCarveBeziez carves an organic cubic Bézier tunnel between points a and b.
// Produces natural-looking curved passages instead of straight L-shaped corridors.
func sgCarveBeziez(rng *rand.Rand, grid [][]int, a, b sgPoint, radius int) {
	dx := b.X - a.X
	dy := b.Y - a.Y
	dist := int(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist == 0 {
		sgCarveDisk(grid, a.X, a.Y, radius)
		return
	}
	drift := max(4, dist/4)

	// Two random control points for cubic Bézier — offset from the 1/3 and 2/3 positions
	randInDrift := func(d int) int {
		if d <= 0 {
			return 0
		}
		return rng.Intn(d*2+1) - d
	}
	cp1 := sgPoint{
		X: a.X + dx/3 + randInDrift(drift),
		Y: a.Y + dy/3 + randInDrift(drift/2),
	}
	cp2 := sgPoint{
		X: a.X + 2*dx/3 + randInDrift(drift),
		Y: b.Y - dy/3 + randInDrift(drift/2),
	}

	steps := max(dist*3, 16)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		u := 1.0 - t
		x := int(math.Round(u*u*u*float64(a.X) + 3*u*u*t*float64(cp1.X) +
			3*u*t*t*float64(cp2.X) + t*t*t*float64(b.X)))
		y := int(math.Round(u*u*u*float64(a.Y) + 3*u*u*t*float64(cp1.Y) +
			3*u*t*t*float64(cp2.Y) + t*t*t*float64(b.Y)))
		sgCarveDisk(grid, x, y, radius)
	}
}

func sgCarveLine(grid [][]int, a, b sgPoint, radius int) {
	if a.X == b.X {
		step := 1
		if b.Y < a.Y {
			step = -1
		}
		for y := a.Y; y != b.Y+step; y += step {
			sgCarveDisk(grid, a.X, y, radius)
		}
		return
	}
	if a.Y == b.Y {
		step := 1
		if b.X < a.X {
			step = -1
		}
		for x := a.X; x != b.X+step; x += step {
			sgCarveDisk(grid, x, a.Y, radius)
		}
		return
	}
	// Fallback for non-orthogonal input.
	sgCarveL(rand.New(rand.NewSource(1)), grid, a, b, radius)
}

func sgCarveDisk(grid [][]int, cx, cy, radius int) {
	gridW := len(grid[0])
	gridH := len(grid)
	r2 := radius * radius
	for y := cy - radius; y <= cy+radius; y++ {
		if y < 0 || y >= gridH {
			continue
		}
		for x := cx - radius; x <= cx+radius; x++ {
			if x < 0 || x >= gridW {
				continue
			}
			dx := x - cx
			dy := y - cy
			if dx*dx+dy*dy <= r2 {
				sgCarveCell(grid, x, y)
			}
		}
	}
}

func sgCarveCell(grid [][]int, x, y int) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	if grid[y][x] == sgCellSolid {
		grid[y][x] = sgCellBackwall
	}
}

// sgCarveRectRoom carves a rectangular underground room centered at (cx, cy).
// cy is the floor row (stays solid). The room extends ry rows upward from cy.
// rx is the half-width. The function guarantees room integrity:
//
//	Step 1: stamps a solid border (2-cell side walls, ceiling, floor rows) even
//	        over previously-carved cells — so adjacent carved regions never
//	        dissolve the room walls.
//	Step 2: carves the interior, preserving the stamped border.
func sgCarveRectRoom(grid [][]int, cx, cy, rx, ry int) {
	const bottomGuard = 8
	gridH, gridW := len(grid), len(grid[0])

	x0, x1 := cx-rx, cx+rx
	y0, y1 := cy-ry, cy // y1 = floor row (solid, not carved)

	// Step 1: Stamp solid border so walls always exist.
	for y := y0 - 1; y <= y1; y++ {
		if y < 1 || y >= gridH-bottomGuard {
			continue
		}
		for x := x0 - 1; x <= x1+1; x++ {
			if x < 1 || x >= gridW-1 {
				continue
			}
			isSideWall := x < x0+2 || x > x1-2
			isCeilingOrFloor := y < y0 || y >= y1
			if isSideWall || isCeilingOrFloor {
				grid[y][x] = sgCellSolid
			}
		}
	}

	// Step 2: Carve interior only (skip 2-cell side walls, ceiling, floor).
	for y := y0; y < y1; y++ {
		if y < 1 || y >= gridH-bottomGuard {
			continue
		}
		for x := x0 + 2; x <= x1-2; x++ {
			if x >= 1 && x < gridW-1 {
				sgCarveCell(grid, x, y)
			}
		}
	}
}

// sgBossRoomSolidRatio returns the fraction of proposed rectangular room cells
// that are still solid (not yet carved). A high ratio means "mostly untouched
// earth" and signals a good placement site.
func sgBossRoomSolidRatio(grid [][]int, cx, cy, rx, ry int) float64 {
	gridH, gridW := len(grid), len(grid[0])
	total, solid := 0, 0
	for y := cy - ry; y < cy; y++ {
		if y < 0 || y >= gridH {
			return 0
		}
		for x := cx - rx; x <= cx+rx; x++ {
			if x < 0 || x >= gridW {
				return 0 // touches boundary → reject
			}
			total++
			if grid[y][x] == sgCellSolid {
				solid++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(solid) / float64(total)
}

// sgMaxBossLevelForZone returns the maximum boss power level for a given ring zone.
// Level 1 = mini boss, 2 = boss, 3 = super boss.
// Power levels 4 and 5 represent paired encounters (see sgRoomLevelToBossSpawns).
func sgMaxBossLevelForZone(zone shared.RingZone) int {
	switch zone {
	case shared.RingZoneGreen:
		return 2
	case shared.RingZoneRed:
		return 4
	case shared.RingZoneBlack, shared.RingZoneThrone:
		return 5
	default:
		return 2
	}
}

// sgRoomBossLevel returns the power level for a room at depth cy.
// Deep rooms (bottom 40%) get maxLevel; shallower rooms get maxLevel-1 (min 1).
func sgRoomBossLevel(cy, splitY, gridH, maxLevel int) int {
	deepLo := splitY + (gridH-splitY)*6/10
	if cy >= deepLo {
		return maxLevel
	}
	return max(1, maxLevel-1)
}

// sgRoomFloorOK returns true if at least 60% of the floor row (cy) within the
// room's interior is solid. Used to decide whether to place a ground boss.
func sgRoomFloorOK(grid [][]int, cx, cy, rx int) bool {
	gridW := len(grid[0])
	gridH := len(grid)
	if cy < 0 || cy >= gridH {
		return false
	}
	interior := 0
	solid := 0
	for x := cx - rx + 2; x <= cx+rx-2; x++ {
		if x < 0 || x >= gridW {
			continue
		}
		interior++
		if grid[cy][x] == sgCellSolid {
			solid++
		}
	}
	if interior == 0 {
		return false
	}
	return float64(solid)/float64(interior) >= 0.6
}

// sgRoomLevelToBossSpawns converts a room power level to a list of boss spawns
// placed symmetrically at block position (cx, floorRow) in pixel coordinates.
//
//	level 1 → 1 mini boss (level 1)
//	level 2 → 1 boss      (level 2)
//	level 3 → 1 super boss (level 3)
//	level 4 → super boss + mini boss
//	level 5 → super boss + boss
func sgRoomLevelToBossSpawns(powerLevel, cx, floorRow, blockSize int) []shared.BossSpawn {
	px := float64(cx * blockSize)
	py := float64(floorRow * blockSize)
	side := float64(blockSize) // one block offset for paired spawns
	switch powerLevel {
	case 1:
		return []shared.BossSpawn{{X: px, Y: py, Level: 1}}
	case 2:
		return []shared.BossSpawn{{X: px, Y: py, Level: 2}}
	case 3:
		return []shared.BossSpawn{{X: px, Y: py, Level: 3}}
	case 4:
		return []shared.BossSpawn{
			{X: px - side, Y: py, Level: 3},
			{X: px + side, Y: py, Level: 1},
		}
	default: // 5+
		return []shared.BossSpawn{
			{X: px - side, Y: py, Level: 3},
			{X: px + side, Y: py, Level: 2},
		}
	}
}

// sgAddFloorBridgePlatforms adds stepping-stone platforms across a floor gap so
// the player can hop across. Count and width scale with room half-width rx:
//
//	rx < 8  → 2 platforms, 2 cells wide each
//	rx < 14 → 3 platforms, 2-3 cells wide
//	rx >= 14 → 4 platforms, 3 cells wide
//
// Platforms are staggered vertically (alternating 2 and 3 rows above floor) to
// avoid a flat row of platforms at a single height.
func sgAddFloorBridgePlatforms(grid [][]int, cx, cy, rx int) {
	gridW := len(grid[0])
	gridH := len(grid)
	if cy < 4 {
		return
	}

	// Determine count and individual width from room size.
	count := 2
	platW := 2
	if rx >= 14 {
		count = 4
		platW = 3
	} else if rx >= 8 {
		count = 3
		platW = 2
	}

	// Distribute platforms evenly across [-rx*2/3 .. rx*2/3] relative to cx.
	span := rx * 4 / 3
	if count == 1 {
		span = 0
	}
	for i := 0; i < count; i++ {
		var offsetX int
		if count > 1 {
			offsetX = -span/2 + i*span/(count-1)
		}
		// Alternate height: even index → 3 above floor, odd → 2 above floor.
		aboveFloor := 3
		if i%2 == 1 {
			aboveFloor = 2
		}
		py := cy - aboveFloor
		px := cx + offsetX - platW/2

		for dx := 0; dx < platW; dx++ {
			x := px + dx
			if x < 1 || x >= gridW-1 || py < 1 || py >= gridH-1 {
				continue
			}
			if grid[py][x] == sgCellAir || grid[py][x] == sgCellBackwall {
				grid[py][x] = sgCellPlatform
			}
		}
	}
}

// sgBuildBossSpawnList assembles all boss spawn markers for the room:
//   - Underground boss rooms (bossRooms) get maxLevel or maxLevel-1 based on depth.
//   - Underground mini boss rooms (miniBossRooms) get one power level below their
//     adjacent boss room.
//   - Sky hubs (skyHubs, sorted bottom→top) scale from level 1 to maxLevel.
//
// All positions are converted to pixels using blockSize.
// sgBuildRiftZones collects pixel-space rectangles where a rift is allowed to
// materialise. Two categories of zone are produced:
//
//  1. Sky platforms (sgCellPlatform, y < splitY): each contiguous horizontal run
//     of platform cells becomes one zone. The rift sits on top of the platform.
//
//  2. Underground tunnel mouths at near-ground depth (splitY < y < splitY+8):
//     horizontal runs of passable floor cells that are NOT inside boss or
//     mini-boss rooms. These represent the widest flat sections at the entrance
//     to underground tunnels — exactly where players are most exposed.
func sgBuildRiftZones(grid [][]int, tags *sgSkyTagGrid, bossRooms, miniBossRooms [][4]int, splitY, blockSize int) []shared.Rect {
	gridH := len(grid)
	var allZones []shared.Rect

	// 1. Собираем площадки в небе (выше поверхности)
	// Начинаем с y=5, чтобы не спавнить у самого потолка карты
	skyZones := sgFindRiftSurfaces(grid, 5, splitY-1, bossRooms, miniBossRooms, blockSize)
	allZones = append(allZones, skyZones...)

	// 2. Собираем площадки под землей (включая туннели и шахты)
	// gridH-4 — чтобы не спавнить в самом низу, где пол карты
	undergroundZones := sgFindRiftSurfaces(grid, splitY+1, gridH-4, bossRooms, miniBossRooms, blockSize)
	allZones = append(allZones, undergroundZones...)

	return allZones
}

// sgFindRiftSurfaces — это чистая функция, которая ищет горизонтальные площадки
// для рифтов, учитывая наличие пола (Solid/Platform) и свободного места сверху.
func sgFindRiftSurfaces(grid [][]int, yMin, yMax int, bossRooms, miniBossRooms [][4]int, blockSize int) []shared.Rect {
	gridW := len(grid[0])
	gridH := len(grid)
	var zones []shared.Rect

	const riftWidthBlocks = 4
	const riftHeightBlocks = 5

	// Локальная проверка: не находимся ли мы внутри или слишком близко к комнате босса
	inBossArea := func(x, y int) bool {
		for _, r := range append(bossRooms, miniBossRooms...) {
			// [0]=cx, [1]=cy, [2]=rx, [3]=ry
			cx, cy, rx, ry := r[0], r[1], r[2], r[3]
			if x >= cx-rx-1 && x <= cx+rx+1 && y >= cy-ry-1 && y <= cy+1 {
				return true
			}
		}
		return false
	}

	for y := yMin; y <= yMax && y < gridH-1; y++ {
		runStart := -1
		for x := 0; x < gridW; x++ {
			// 1. Проверяем пол (теперь учитываем и обычные блоки, и платформы-ступеньки)
			hasFloor := grid[y+1][x] == sgCellSolid || grid[y+1][x] == sgCellPlatform

			// 2. Проверяем место для "тушки" рифта (3 блока воздуха вверх)
			hasHeadroom := true
			for h := 0; h < riftHeightBlocks; h++ {
				checkY := y - h
				if checkY < 0 || grid[checkY][x] == sgCellSolid {
					hasHeadroom = false
					break
				}
			}

			if hasFloor && hasHeadroom && !inBossArea(x, y) {
				if runStart < 0 {
					runStart = x
				}
			} else {
				if runStart >= 0 {
					runLen := x - runStart
					if runLen >= riftWidthBlocks {
						zones = append(zones, shared.Rect{
							X: float64(runStart * blockSize),
							Y: float64((y - 1) * blockSize),
							W: float64(runLen * blockSize),
							H: float64(2 * blockSize),
						})
					}
					runStart = -1
				}
			}
		}
	}
	return zones
}

func sgBuildBossSpawnList(
	grid [][]int,
	bossRooms, miniBossRooms [][4]int,
	skyHubs []sgSkyHub,
	splitY, gridH, maxLevel, blockSize int,
) []shared.BossSpawn {
	var spawns []shared.BossSpawn

	// Underground boss rooms.
	for _, r := range bossRooms {
		cx, cy, rx, ry := r[0], r[1], r[2], r[3]
		_ = ry
		level := sgRoomBossLevel(cy, splitY, gridH, maxLevel)
		if sgRoomFloorOK(grid, cx, cy, rx) {
			spawns = append(spawns, sgRoomLevelToBossSpawns(level, cx, cy-1, blockSize)...)
		} else {
			// Floor has gaps — flying boss in room center, add bridge platforms.
			sgAddFloorBridgePlatforms(grid, cx, cy, rx)
			midY := cy - ry/2
			spawns = append(spawns, shared.BossSpawn{
				X:      float64(cx * blockSize),
				Y:      float64(midY * blockSize),
				Level:  min(level, 3),
				Flying: true,
			})
		}
	}

	// Underground mini boss rooms: one power level below the associated boss room.
	// Mini rooms are adjacent to boss rooms and share the same depth zone, so
	// we compute their power level the same way but subtract 1.
	// Always place a mini-boss here — mini-boss rooms are specially carved for
	// this purpose. If the floor is damaged (e.g. by the connecting tunnel),
	// fall back to a flying spawn in the room centre.
	for _, r := range miniBossRooms {
		cx, cy, rx, ry := r[0], r[1], r[2], r[3]
		bossLevel := sgRoomBossLevel(cy, splitY, gridH, maxLevel)
		level := max(1, bossLevel-1)
		if sgRoomFloorOK(grid, cx, cy, rx) {
			spawns = append(spawns, sgRoomLevelToBossSpawns(level, cx, cy-1, blockSize)...)
		} else {
			// Floor damaged — place flying mini-boss at room centre.
			midY := cy - max(1, ry/2)
			spawns = append(spawns, shared.BossSpawn{
				X:      float64(cx * blockSize),
				Y:      float64(midY * blockSize),
				Level:  min(level, 1), // mini-boss only in side room
				Flying: true,
			})
		}
	}

	// Sky hubs: scale from level 1 (lowest hub, closest to ground) to maxLevel
	// (highest hub, furthest from ground). Hubs are already stored top→bottom in
	// Y-ascending order in sgSkyHub (lower Y = higher in the air).
	if len(skyHubs) > 0 {
		// Sort hubs by Y ascending (highest in sky = lowest Y = hardest).
		sorted := make([]sgSkyHub, len(skyHubs))
		copy(sorted, skyHubs)
		for i := 1; i < len(sorted); i++ {
			for j := i; j > 0 && sorted[j].Y < sorted[j-1].Y; j-- {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			}
		}
		n := len(sorted)
		for k, hub := range sorted {
			// k=0 is the highest hub (lowest Y) → maxLevel.
			// k=n-1 is the lowest hub (highest Y) → level 1.
			level := 1
			if n > 1 {
				level = 1 + (maxLevel-1)*(n-1-k)/(n-1)
			} else {
				level = maxLevel
			}
			level = max(1, min(level, maxLevel))
			bx := hub.X + hub.W/2
			by := hub.Y - 1 // one block above hub top face
			if by < 0 {
				by = 0
			}
			spawns = append(spawns, sgRoomLevelToBossSpawns(level, bx, by, blockSize)...)
		}
	}

	return spawns
}

// sgPlaceMiniBossRooms carves small rectangular side rooms adjacent to placed
// boss rooms. Each boss room has a random chance of spawning a mini room on its
// left or right side. Mini rooms connect via a narrow tunnel through the boss
// room wall and require a solid floor (no holes). Only solid earth is carved —
// existing carved regions are never modified.
// Returns [cx,cy,rx,ry] for each placed mini room so callers can place bosses.
func sgPlaceMiniBossRooms(rng *rand.Rand, grid [][]int, bossRooms [][4]int, splitY int) [][4]int {
	if len(bossRooms) == 0 {
		return nil
	}
	gridH := len(grid)
	gridW := len(grid[0])
	const bottomGuard = 8
	const spawnChance = 60 // percent chance per boss room

	var placed [][4]int

	for _, r := range bossRooms {
		cx, cy, rx, ry := r[0], r[1], r[2], r[3]
		_ = ry
		if rng.Intn(100) >= spawnChance {
			continue
		}

		// Mini room dimensions: 30-50% of the boss room size.
		mRx := max(4, rx*2/5+rng.Intn(max(1, rx/5)))
		mRy := max(3, r[3]*2/5+rng.Intn(max(1, r[3]/5)))

		// Try left side, then right side.
		for _, side := range []int{-1, 1} {
			if rng.Intn(2) == 0 {
				side = -side // randomise which side is tried first
			}
			// Mini room center X: just outside the boss room wall.
			var mCx int
			var tunnelX1, tunnelX2 int
			if side < 0 {
				// Left side
				mCx = cx - rx - mRx - 2
				tunnelX1, tunnelX2 = mCx+mRx, cx-rx
			} else {
				// Right side
				mCx = cx + rx + mRx + 2
				tunnelX1, tunnelX2 = cx+rx, mCx-mRx
			}
			// Mini room floor row same as boss room floor row.
			mCy := cy

			// Bounds check.
			if mCx-mRx < 2 || mCx+mRx >= gridW-2 {
				continue
			}
			if mCy-mRy < splitY+2 || mCy >= gridH-bottomGuard {
				continue
			}

			// Require ≥80% solid in the proposed mini room area.
			if sgBossRoomSolidRatio(grid, mCx, mCy, mRx, mRy) < 0.80 {
				continue
			}

			// Verify that the mini room has a solid floor (no holes in cy row).
			floorOk := true
			for x := mCx - mRx; x <= mCx+mRx; x++ {
				if x < 0 || x >= gridW {
					floorOk = false
					break
				}
				// cy row itself must be solid (floor).
				if grid[mCy][x] != sgCellSolid {
					floorOk = false
					break
				}
			}
			if !floorOk {
				continue
			}

			// Carve the mini room rectangle (walls stamped first, then interior).
			sgCarveRectRoom(grid, mCx, mCy, mRx, mRy)

			// Carve a narrow horizontal connecting tunnel at mid-height of mini room.
			tunnelY := mCy - mRy/2
			sgCarveLine(grid,
				sgPoint{X: tunnelX1, Y: tunnelY},
				sgPoint{X: tunnelX2, Y: tunnelY},
				1) // radius 1 = 3-cell-wide tunnel (just enough for player width)

			placed = append(placed, [4]int{mCx, mCy, mRx, mRy})
			break // one mini room per boss room
		}
	}
	return placed
}

// sgPlaceBossRoomsNearShafts places large elliptical boss rooms in the densest
// solid earth between and adjacent to inlet shafts.
//
// Priority rules:
//   - Deep first: scan the bottom 40% of underground before shallower depths.
//   - Max size first: try the largest rx/ry, reduce only if solid ratio fails.
//   - Multiple rooms per gap: try to place both a deep and a mid-depth room in
//     each inter-shaft gap so more solid earth is consumed.
//   - Multiple rooms per shaft: each shaft connects to all rooms within reach —
//     there is no "already covered" skip. A shaft may serve several boss rooms.
//
// Only solid cells are carved — existing carved regions (backwall, air) are never
// modified, so adjacent hubs and corridors are never damaged.
// sgPlaceBossRoomsNearShafts returns [cx,cy,rx,ry] for each placed room so
// the caller can feed them to sgPlaceMiniBossRooms.
func sgPlaceBossRoomsNearShafts(rng *rand.Rand, grid [][]int, shafts []sgShaft, hubs []sgHub, splitY int, cfg ProcGenConfig, carveRadius int) [][4]int {
	if len(shafts) == 0 {
		return nil
	}

	gridH := len(grid)
	gridW := len(grid[0])
	const bottomGuard = 8
	const solidThreshold = 0.80 // 80% solid required — avoids eating carved regions
	const tunnelRadius = 1      // half-width of horizontal entry tunnels (radius=1 → 3-cell wide)

	// Room height range (scaled to map size).
	ryMax := max(12, sgScaleY(16, cfg))
	ryMin := max(8, sgScaleY(10, cfg))

	// Sort shafts left→right for gap iteration.
	for i := 1; i < len(shafts); i++ {
		for j := i; j > 0 && shafts[j].CenterX < shafts[j-1].CenterX; j-- {
			shafts[j], shafts[j-1] = shafts[j-1], shafts[j]
		}
	}

	type roomRec struct{ cx, cy, rx, ry int }
	placed := make([]roomRec, 0, len(shafts)*2)

	// shaftConnected[i] becomes true when shaft i gets a horizontal tunnel to a
	// boss room or another shaft. Isolated shafts (never set) get a forced
	// connection in Pass 4.
	shaftConnected := make([]bool, len(shafts))

	// shaftIdx returns the index of the shaft pointed to, or -1.
	shaftIdx := func(s *sgShaft) int {
		for i := range shafts {
			if &shafts[i] == s {
				return i
			}
		}
		return -1
	}

	tooClose := func(cx, cy, rx int) bool {
		for _, r := range placed {
			dx, dy := cx-r.cx, cy-r.cy
			if dx*dx+dy*dy < (r.rx+rx+14)*(r.rx+rx+14) {
				return true
			}
		}
		return false
	}

	// scanZone finds the Y with the highest solid ratio in [yLo, yHi] for the
	// given (cx, rx, ry). Prefers deeper positions via a small depth bonus.
	scanZone := func(cx, rx, ry, yLo, yHi int) (int, float64) {
		if yHi <= yLo {
			return -1, 0
		}
		step := max(2, (yHi-yLo)/25)
		bestCy, bestScore := -1, 0.0
		for testY := yLo; testY <= yHi; testY += step {
			ratio := sgBossRoomSolidRatio(grid, cx, testY, rx, ry)
			// Depth bonus: 0 at yLo → +0.08 at yHi
			depthBonus := 0.08 * float64(testY-yLo) / float64(max(1, yHi-yLo))
			score := ratio + depthBonus
			if score > bestScore {
				bestScore = score
				bestCy = testY
			}
		}
		// Refine ±step around best candidate.
		if bestCy >= 0 {
			lo := max(yLo, bestCy-step)
			hi := min(yHi, bestCy+step)
			for testY := lo; testY <= hi; testY++ {
				ratio := sgBossRoomSolidRatio(grid, cx, testY, rx, ry)
				depthBonus := 0.08 * float64(testY-yLo) / float64(max(1, yHi-yLo))
				score := ratio + depthBonus
				if score > bestScore {
					bestScore = score
					bestCy = testY
				}
			}
		}
		// Return raw ratio (not the score) for the threshold check.
		if bestCy >= 0 {
			return bestCy, sgBossRoomSolidRatio(grid, cx, bestCy, rx, ry)
		}
		return -1, 0
	}

	// connectShaftToRoom carves a horizontal tunnel from shaft.CenterX to
	// roomEdgeX at tunnelY. If the shaft doesn't reach tunnelY it is extended
	// straight down first so the connection is physically traversable.
	// Marks the shaft as connected so Pass 4 won't treat it as isolated.
	connectShaftToRoom := func(shaft *sgShaft, tunnelY, roomEdgeX int) {
		sx := shaft.CenterX
		if shaft.BottomY < tunnelY {
			sgCarveLine(grid,
				sgPoint{X: sx, Y: shaft.BottomY},
				sgPoint{X: sx, Y: tunnelY},
				tunnelRadius)
		}
		sgCarveLine(grid,
			sgPoint{X: sx, Y: tunnelY},
			sgPoint{X: roomEdgeX, Y: tunnelY},
			tunnelRadius)
		if idx := shaftIdx(shaft); idx >= 0 {
			shaftConnected[idx] = true
		}
	}

	// connectToHubs Bezier-connects a room to its 2 nearest hubs.
	connectToHubs := func(cx, cy int) {
		if len(hubs) == 0 {
			return
		}
		type hd struct{ idx, d2 int }
		ds := make([]hd, len(hubs))
		for i, h := range hubs {
			c := h.Center()
			ddx, ddy := c.X-cx, c.Y-cy
			ds[i] = hd{i, ddx*ddx + ddy*ddy}
		}
		for i := 1; i < len(ds); i++ {
			for j := i; j > 0 && ds[j].d2 < ds[j-1].d2; j-- {
				ds[j], ds[j-1] = ds[j-1], ds[j]
			}
		}
		roomPt := sgPoint{X: cx, Y: cy - 1}
		for i := 0; i < min(2, len(ds)); i++ {
			sgCarveBeziez(rng, grid, roomPt, hubs[ds[i].idx].Center(), carveRadius)
		}
	}

	// placeRoomAt tries to carve a boss room at (cx, *) scanning the given Y
	// zones in order (deep zone first). Connects to leftShaft/rightShaft and hubs.
	// Returns true if a room was successfully placed.
	placeRoomAt := func(cx, rxCap int, leftShaft, rightShaft *sgShaft, deepFirst bool) bool {
		rxMaxLocal := min(rxCap, max(14, sgScaleX(22, cfg)))
		rxMinLocal := max(6, sgScaleX(8, cfg))
		if rxMaxLocal < rxMinLocal {
			rxMaxLocal = rxMinLocal
		}

		undergroundH := gridH - splitY
		// Deep zone: bottom 40% of underground.
		deepLo := splitY + undergroundH*6/10
		deepHi := gridH - bottomGuard - 1
		// Shallow zone: rest of underground.
		shallowLo := splitY + 4
		shallowHi := deepLo - 1

		for rx := rxMaxLocal; rx >= rxMinLocal; rx -= 2 {
			if cx-rx < 2 || cx+rx >= gridW-2 {
				continue
			}
			for ry := ryMax; ry >= ryMin; ry-- {
				yLo1, yHi1 := deepLo, deepHi
				yLo2, yHi2 := shallowLo, shallowHi
				if !deepFirst {
					yLo1, yHi1, yLo2, yHi2 = yLo2, yHi2, yLo1, yHi1
				}
				bestCy, bestRatio := scanZone(cx, rx, ry, max(yLo1, splitY+ry+4), yHi1)
				if bestRatio < solidThreshold || bestCy < 0 {
					bestCy, bestRatio = scanZone(cx, rx, ry, max(yLo2, splitY+ry+4), yHi2)
				}
				if bestRatio < solidThreshold || bestCy < 0 {
					continue
				}
				if tooClose(cx, bestCy, rx) {
					continue
				}

				sgCarveRectRoom(grid, cx, bestCy, rx, ry)
				placed = append(placed, roomRec{cx, bestCy, rx, ry})

				tunnelY := bestCy - ry/2
				if leftShaft != nil {
					connectShaftToRoom(leftShaft, tunnelY, cx-rx)
				}
				if rightShaft != nil {
					connectShaftToRoom(rightShaft, tunnelY, cx+rx)
				}
				connectToHubs(cx, bestCy)
				return true
			}
		}
		return false
	}

	// ── Pass 1: Between each pair of adjacent shafts ─────────────────────────
	// For each gap we try up to two rooms: first deep, then at a shallower
	// depth (if the gap is wide enough for a second room without overlap).
	for i := 0; i < len(shafts)-1; i++ {
		left := &shafts[i]
		right := &shafts[i+1]
		spacing := right.CenterX - left.CenterX
		if spacing < 20 {
			continue
		}
		midX := (left.CenterX + right.CenterX) / 2
		rxCap := spacing/2 - 6

		// Room 1 — prefer deep.
		placeRoomAt(midX, rxCap, left, right, true /*deepFirst*/)

		// Room 2 — prefer shallow (different depth, same gap).
		// Only attempt if there's still uncovered solid earth.
		placeRoomAt(midX, rxCap, left, right, false /*deepFirst*/)
	}

	// ── Pass 2: Each shaft tries to connect to more rooms on both sides ───────
	// We do a second sweep where every shaft looks for rooms in the inter-shaft
	// gaps it borders and tries an additional deep room if one is missing.
	rxCap := max(14, sgScaleX(22, cfg))
	for i := range shafts {
		s := &shafts[i]

		// Try placing a room offset from the shaft if none is close yet.
		for _, xOff := range []int{0, rxCap / 2, -rxCap / 2, rxCap, -rxCap} {
			cx := s.CenterX + xOff
			if cx < rxCap+4 || cx >= gridW-rxCap-4 {
				continue
			}
			// Determine which adjacent shaft is on the other side of xOff.
			var otherShaft *sgShaft
			if xOff > 0 && i+1 < len(shafts) {
				otherShaft = &shafts[i+1]
			} else if xOff < 0 && i > 0 {
				otherShaft = &shafts[i-1]
			}
			leftShaft, rightShaft := s, otherShaft
			if xOff < 0 {
				leftShaft, rightShaft = otherShaft, s
			}
			if placeRoomAt(cx, rxCap, leftShaft, rightShaft, true) {
				break
			}
		}
	}

	// ── Pass 3: Reconnect every shaft to all nearby rooms ────────────────────
	// After all rooms are placed, scan every shaft → room combination within
	// horizontal reach and add missing horizontal entry tunnels. This ensures
	// each shaft touches multiple rooms without needing to know room placement
	// in advance.
	maxHorizReach := max(60, sgScaleX(80, cfg))
	for i := range shafts {
		s := &shafts[i]
		for _, r := range placed {
			dx := abs(r.cx - s.CenterX)
			if dx > r.rx+maxHorizReach {
				continue // too far away
			}
			tunnelY := r.cy - r.ry/2
			// Connect shaft to the nearest room edge.
			if s.CenterX < r.cx {
				connectShaftToRoom(s, tunnelY, r.cx-r.rx)
			} else {
				connectShaftToRoom(s, tunnelY, r.cx+r.rx)
			}
		}
	}

	// ── Pass 4: Force-connect isolated shafts ─────────────────────────────────
	// Any shaft that received no horizontal tunnel in Passes 1-3 is isolated.
	// For each isolated shaft, find the nearest boss room and carve:
	//   1. A vertical extension of the shaft down to the room's entry Y.
	//   2. A horizontal tunnel from the shaft to the side of that room.
	// If no boss room was placed at all, fall back to connecting shaft-to-shaft
	// at the deepest common Y.
	for i := range shafts {
		if shaftConnected[i] {
			continue // already has at least one horizontal connection
		}
		si := &shafts[i]

		if len(placed) > 0 {
			// Find the nearest placed boss room.
			nearestRoom := placed[0]
			nearestDist := abs(placed[0].cx - si.CenterX)
			for _, r := range placed[1:] {
				d := abs(r.cx - si.CenterX)
				if d < nearestDist {
					nearestDist = d
					nearestRoom = r
				}
			}
			// Connect to the side of that room.
			tunnelY := nearestRoom.cy - nearestRoom.ry/2
			if si.CenterX <= nearestRoom.cx {
				connectShaftToRoom(si, tunnelY, nearestRoom.cx-nearestRoom.rx)
			} else {
				connectShaftToRoom(si, tunnelY, nearestRoom.cx+nearestRoom.rx)
			}
		} else {
			// No boss rooms at all — connect to the nearest other shaft at the
			// deepest common Y so both shafts get linked at the bottom.
			nearestJ := -1
			nearestDist := gridW + 1
			for j := range shafts {
				if j == i {
					continue
				}
				d := abs(shafts[j].CenterX - si.CenterX)
				if d < nearestDist {
					nearestDist = d
					nearestJ = j
				}
			}
			if nearestJ >= 0 {
				sj := &shafts[nearestJ]
				tunnelY := max(si.BottomY, sj.BottomY)
				// Extend both shafts down to tunnelY if needed.
				if si.BottomY < tunnelY {
					sgCarveLine(grid,
						sgPoint{X: si.CenterX, Y: si.BottomY},
						sgPoint{X: si.CenterX, Y: tunnelY},
						tunnelRadius)
				}
				if sj.BottomY < tunnelY {
					sgCarveLine(grid,
						sgPoint{X: sj.CenterX, Y: sj.BottomY},
						sgPoint{X: sj.CenterX, Y: tunnelY},
						tunnelRadius)
				}
				sgCarveLine(grid,
					sgPoint{X: si.CenterX, Y: tunnelY},
					sgPoint{X: sj.CenterX, Y: tunnelY},
					tunnelRadius)
				shaftConnected[i] = true
				shaftConnected[nearestJ] = true
			}
		}
	}

	// Convert internal roomRec slice to the public [][4]int format.
	result := make([][4]int, len(placed))
	for i, r := range placed {
		result[i] = [4]int{r.cx, r.cy, r.rx, r.ry}
	}
	return result
}

// sgCountUndergroundPassable returns the number of non-solid cells in y ≥ splitY.
func sgCountUndergroundPassable(grid [][]int, splitY int) int {
	count := 0
	for y := splitY; y < len(grid); y++ {
		for x := 0; x < len(grid[y]); x++ {
			if grid[y][x] != sgCellSolid {
				count++
			}
		}
	}
	return count
}

func sgAddWallLedges(grid [][]int, splitY int, cfg ProcGenConfig) {
	threshold := max(3, sgScaleY(6, cfg))
	step := max(2, sgScaleY(5, cfg))

	comps := sgPassableComponents(grid, splitY)
	for _, comp := range comps {
		height := comp.MaxY - comp.MinY + 1
		if height <= threshold {
			continue
		}

		sideLeft := true
		for y := comp.MinY + step; y <= comp.MaxY-step; y += step {
			segments := sgRowPassableSegments(grid, y, comp.MinX, comp.MaxX)
			seg, ok := sgWidestRowSegment(segments, 4)
			if !ok {
				continue
			}

			x0 := seg.Left
			if !sideLeft {
				x0 = seg.Right - 1
			}
			x0 = sgClamp(x0, seg.Left, seg.Right-1)

			for dx := 0; dx < 2; dx++ {
				x := x0 + dx
				if x < seg.Left || x > seg.Right {
					continue
				}
				if sgIsPassable(grid[y][x]) {
					grid[y][x] = sgCellPlatform // one-way wall ledge
				}
			}
			sideLeft = !sideLeft
		}
	}
}

func sgRowPassableSegments(grid [][]int, y, minX, maxX int) []sgRowSegment {
	if y < 0 || y >= len(grid) {
		return nil
	}
	minX = sgClamp(minX, 0, len(grid[y])-1)
	maxX = sgClamp(maxX, 0, len(grid[y])-1)
	if maxX < minX {
		return nil
	}

	segs := make([]sgRowSegment, 0, 4)
	x := minX
	for x <= maxX {
		for x <= maxX && !sgIsPassable(grid[y][x]) {
			x++
		}
		if x > maxX {
			break
		}
		left := x
		for x <= maxX && sgIsPassable(grid[y][x]) {
			x++
		}
		right := x - 1
		segs = append(segs, sgRowSegment{Left: left, Right: right})
	}
	return segs
}

func sgWidestRowSegment(segments []sgRowSegment, minWidth int) (sgRowSegment, bool) {
	best := sgRowSegment{}
	bestW := -1
	for _, seg := range segments {
		w := seg.Right - seg.Left + 1
		if w < minWidth {
			continue
		}
		if w > bestW {
			bestW = w
			best = seg
		}
	}
	if bestW < 0 {
		return sgRowSegment{}, false
	}
	return best, true
}

func sgAddInternalLedges(rng *rand.Rand, grid [][]int, splitY int, cfg ProcGenConfig) {
	const minHeight = 8 // don't add ledges in short spaces
	const maxJumpH = 8  // max jump height (blocks)
	ledgeMinW := max(sgPlayerW, sgScaleX(3, cfg))
	ledgeMaxW := max(ledgeMinW, sgScaleX(5, cfg))

	comps := sgPassableComponents(grid, splitY)
	for _, comp := range comps {
		height := comp.MaxY - comp.MinY + 1
		if height <= minHeight {
			continue
		}

		// Build a reachable chain of ledges starting from the cave floor and
		// stepping up by 5-maxJumpH rows each time. This guarantees every
		// ledge is reachable from the one below (or from the floor).
		stepMin := max(5, maxJumpH-3)
		stepMax := maxJumpH
		prevFloorY := comp.MaxY + 1 // solid floor below component
		sideLeft := rng.Intn(2) == 0

		for {
			dy := stepMin + rng.Intn(stepMax-stepMin+1)
			nextY := prevFloorY - dy - 1 // ledge row (solid), player stands at nextY-1
			if nextY <= comp.MinY+sgPlayerH {
				break
			}

			w := ledgeMinW
			if ledgeMaxW > ledgeMinW {
				w += rng.Intn(ledgeMaxW - ledgeMinW + 1)
			}

			// Bias position left or right to create a zig-zag feel.
			biasX := comp.MinX
			if !sideLeft {
				biasX = comp.MaxX - w
			}
			biasX = sgClamp(biasX, comp.MinX, comp.MaxX-w+1)

			// Component-wide headroom check: if any row in [nextY-sgPlayerH, nextY-1]
			// contains a platform or solid across the FULL component width, another
			// ledge/step already owns that zone. Skip this Y regardless of which side
			// the new ledge would go on — this is what prevents sgAddInternalLedges
			// from inserting a platform between two zigzag staircase steps that sit on
			// opposite sides of a narrow shaft (the per-ledge headroom check in
			// sgIsLedgeReachable only covers the ledge's own columns and misses the
			// other side).
			componentHeadroomBlocked := false
			for dy := 1; dy <= sgPlayerH; dy++ {
				checkY := nextY - dy
				if checkY < 0 {
					componentHeadroomBlocked = true
					break
				}
				for cx := comp.MinX; cx <= comp.MaxX; cx++ {
					c := grid[checkY][cx]
					if c == sgCellPlatform || c == sgCellSolid {
						componentHeadroomBlocked = true
						break
					}
				}
				if componentHeadroomBlocked {
					break
				}
			}
			if componentHeadroomBlocked {
				break
			}

			x0, ok := sgPickLedgeXBiased(rng, grid, comp, nextY, w, biasX)
			if !ok {
				// Try random position as fallback.
				x0, ok = sgPickLedgeX(rng, grid, comp, nextY, w)
			}
			if !ok {
				break
			}

			if !sgIsLedgeReachable(grid, x0, nextY, w, prevFloorY) {
				break
			}

			for dx := 0; dx < w; dx++ {
				x := x0 + dx
				if sgIsPassable(grid[nextY][x]) {
					grid[nextY][x] = sgCellPlatform // one-way internal ledge
				}
			}

			prevFloorY = nextY // this ledge is the floor for the next one
			sideLeft = !sideLeft
		}
	}
}

// sgIsLedgeReachable checks whether a horizontal ledge at (x0, y, width) can be
// reached by the player. compFloorY is the row of the nearest solid floor below
// the component (comp.MaxY+1 for underground, hub.Y+hub.H for hubs). It verifies:
//  1. Headroom: sgPlayerH air cells directly above the ledge.
//  2. Approach: either the component floor is within jump range, OR a previously-
//     placed solid platform exists within jump range below the ledge.
func sgIsLedgeReachable(grid [][]int, x0, y, w, compFloorY int) bool {
	const maxJumpH = 8  // blocks the player can jump upward
	const maxJumpW = 10 // horizontal blocks reachable in a jump

	gridH, gridW := len(grid), len(grid[0])

	// 1. Headroom: sgPlayerH rows above the ledge must be completely clear —
	// no solid AND no one-way platform. A platform in the headroom zone means
	// a player standing here would have another platform inside their body, and
	// it is also an indicator that the spacing to the existing platform is too
	// tight (< sgPlayerH rows). This is the guard that prevents sgAddInternalLedges
	// from inserting a ledge inside the headroom of an existing shaft zigzag step.
	for dy := 1; dy <= sgPlayerH; dy++ {
		checkY := y - dy
		if checkY < 0 {
			return false
		}
		for cx := x0; cx < x0+w && cx < gridW; cx++ {
			c := grid[checkY][cx]
			if c == sgCellSolid || c == sgCellPlatform {
				return false
			}
		}
	}

	// 2a. Component floor within direct jump range.
	if compFloorY-y <= maxJumpH {
		return true
	}

	// 2b. A previously-placed stepping-stone (solid or one-way platform) within
	// jump range below the proposed ledge. Platforms count because the player can
	// land on them and jump from them just like from solid ground.
	for searchY := y + 1; searchY <= min(y+maxJumpH, gridH-1); searchY++ {
		xLo := max(0, x0-maxJumpW)
		xHi := min(gridW-1, x0+w-1+maxJumpW)
		for searchX := xLo; searchX <= xHi; searchX++ {
			c := grid[searchY][searchX]
			if c != sgCellSolid && c != sgCellPlatform {
				continue
			}
			// The stepping-stone must have sgPlayerH standing room above it
			// (clear of both solids and platforms).
			canStand := true
			for dy := 1; dy <= sgPlayerH; dy++ {
				standY := searchY - dy
				if standY < 0 {
					canStand = false
					break
				}
				sc := grid[standY][searchX]
				if sc == sgCellSolid || sc == sgCellPlatform {
					canStand = false
					break
				}
			}
			if canStand {
				return true
			}
		}
	}

	return false
}

func sgApplySkyStrictAntiStacking(grid [][]int, tags *sgSkyTagGrid, splitY int, cfg ProcGenConfig) {
	if len(grid) == 0 || len(grid[0]) == 0 || splitY <= 1 {
		return
	}
	checkH := max(2, sgScaleY(10, cfg))
	gridW := len(grid[0])
	top := min(splitY-1, len(grid)-1)
	for y := top; y >= 1; y-- {
		for x := 0; x < gridW; x++ {
			if grid[y][x] != sgCellSolid {
				continue
			}
			baseTag := sgSkyTag{}
			if tags != nil {
				baseTag = tags.At(x, y)
			}
			y0 := max(0, y-checkH)
			for yy := y0; yy < y; yy++ {
				if grid[yy][x] != sgCellSolid {
					continue
				}
				otherTag := sgSkyTag{}
				if tags != nil {
					otherTag = tags.At(x, yy)
				}
				if baseTag.ObjectID > 0 && otherTag.ObjectID == baseTag.ObjectID {
					continue
				}
				// Never delete stairchain steps — they're critical for traversability.
				isStairStep := otherTag.Kind == sgSkyObjectSkyStepLeft ||
					otherTag.Kind == sgSkyObjectSkyStepRight ||
					otherTag.Kind == sgSkyObjectShaftStep
				if isStairStep {
					continue
				}
				grid[yy][x] = sgCellAir
				if tags != nil {
					tags.ClearCell(x, yy)
				}
			}
		}
	}
}

func sgFinalizeHubAccessibility(grid [][]int, hubs []sgHub, tags *sgSkyTagGrid, cfg ProcGenConfig) {
	stepY := 6
	platW := max(sgPlayerW, sgScaleX(3, cfg))

	for _, h := range hubs {
		// СКАНИРУЕМ ПОТОЛОК на наличие шахт
		ceilingExits := []int{}
		for x := h.X + 1; x < h.X+h.W-1; x++ {
			if h.Y > 0 && sgIsPassable(grid[h.Y-1][x]) {
				// Если нашли начало дырки, берем её центр
				if len(ceilingExits) == 0 || x > ceilingExits[len(ceilingExits)-1]+3 {
					ceilingExits = append(ceilingExits, x)
				}
			}
		}

		// СКАНИРУЕМ СТЕНЫ на наличие боковых туннелей
		sideExits := []int{}
		for y := h.Y + 1; y < h.Y+h.H-2; y++ {
			// Проверяем левую и правую стену
			if (h.X > 0 && sgIsPassable(grid[y][h.X-1])) || (h.X+h.W < len(grid[0]) && sgIsPassable(grid[y][h.X+h.W])) {
				if len(sideExits) == 0 || y > sideExits[len(sideExits)-1]+3 {
					sideExits = append(sideExits, y)
				}
			}
		}

		// Hub floor row for reachability check.
		hubFloorY := h.Y + h.H

		// СТРОИМ ЛЕСТНИЦЫ К ШАХТАМ В ПОТОЛКЕ (снизу вверх — каждая опирается на предыдущую)
		for _, exitX := range ceilingExits {
			side := 1
			for py := h.Y + h.H - stepY; py >= h.Y+stepY; py -= stepY {
				offsetX := 2 * side
				px := exitX - (platW / 2) + offsetX
				px = sgClamp(px, h.X+1, h.X+h.W-platW-1)
				if sgIsLedgeReachable(grid, px, py, platW, hubFloorY) {
					sgPlaceHubLedge(grid, px, py, platW, tags)
					side *= -1
				}
			}
		}

		// СТРОИМ ПОДЪЕМЫ К ВЫСОКИМ БОКОВЫМ ТУННЕЛЯМ (снизу вверх)
		for _, exitY := range sideExits {
			distFromFloor := (h.Y + h.H) - exitY
			if distFromFloor > 6 {
				sideLeft := sgIsPassable(grid[exitY][h.X-1])
				for py := h.Y + h.H - stepY; py >= exitY; py -= stepY {
					px := h.X + 1
					if !sideLeft {
						px = h.X + h.W - platW - 1
					}
					if sgIsLedgeReachable(grid, px, py, platW, hubFloorY) {
						sgPlaceHubLedge(grid, px, py, platW, tags)
					}
				}
			}
		}

		// Если хаб пустой и высокий, делаем зигзаг снизу вверх
		if len(ceilingExits) == 0 && len(sideExits) == 0 && h.H > 8 {
			sideLeft := true
			for py := h.Y + h.H - stepY; py >= h.Y+4; py -= stepY {
				px := h.X + 1
				if !sideLeft {
					px = h.X + h.W - platW - 1
				}
				if sgIsLedgeReachable(grid, px, py, platW, hubFloorY) {
					sgPlaceHubLedge(grid, px, py, platW, tags)
				}
				sideLeft = !sideLeft
			}
		}
	}
}

// Вспомогательная функция для отрисовки с защитным тегом
func sgPlaceHubLedge(grid [][]int, x, y, w int, tags *sgSkyTagGrid) {
	objID := 0
	if tags != nil {
		objID = tags.NewObject(sgSkyObjectShaftStep)
	}
	for dx := 0; dx < w; dx++ {
		nx := x + dx
		if y >= 0 && y < len(grid) && nx >= 0 && nx < len(grid[0]) {
			grid[y][nx] = sgCellPlatform // one-way hub ledge
			if tags != nil && objID > 0 {
				tags.MarkCell(nx, y, objID, sgSkyObjectShaftStep)
			}
		}
	}
}

func sgApplyGroundAirCorridor(grid [][]int, splitY int, cfg ProcGenConfig, inlets []sgInlet, chains []sgStairChain, tags *sgSkyTagGrid) {
	if len(grid) == 0 || len(grid[0]) == 0 || splitY <= 0 {
		return
	}
	gridW := len(grid[0])
	corridorH := max(2, sgScaleY(10, cfg))
	y0 := max(0, splitY-corridorH)
	y1 := min(splitY-1, len(grid)-1)

	// Массив для пометки блоков, которые НЕЛЬЗЯ удалять
	keep := make([][]bool, len(grid))
	for i := range keep {
		keep[i] = make([]bool, gridW)
	}

	// Помечаем зоны входов в пещеры (их не трогаем)
	for _, inlet := range inlets {
		left := sgClamp(inlet.CenterX-inlet.Width/2-1, 0, gridW-1)
		right := sgClamp(left+inlet.Width+1, 0, gridW-1)
		for x := left; x <= right; x++ {
			for y := y0; y <= y1; y++ {
				keep[y][x] = true
			}
		}
	}

	// Помечаем ВСЕ блоки всех ступенек в цепочках
	stepW := max(sgPlayerW, sgScaleX(3, cfg))
	for _, chain := range chains {
		for _, step := range chain.Steps {
			if step.Y >= y0 && step.Y <= y1 {
				for dx := 0; dx < stepW; dx++ {
					nx := step.X + dx
					if nx >= 0 && nx < gridW {
						keep[step.Y][nx] = true
					}
				}
			}
		}
	}

	// Удаляем только то, что не помечено как "нужное"
	for y := y0; y <= y1; y++ {
		for x := 0; x < gridW; x++ {
			if keep[y][x] {
				continue
			}
			if grid[y][x] == sgCellSolid && tags.At(x, y).Kind != sgSkyObjectShaftStep {
				grid[y][x] = sgCellAir
			}
		}
	}
}

func sgApplyTunnelHeadroom(grid [][]int, splitY int, cfg ProcGenConfig, tags *sgSkyTagGrid) {
	if len(grid) == 0 || len(grid[0]) == 0 || splitY <= 0 {
		return
	}
	gridW := len(grid[0])
	gridH := len(grid)
	minHeadroom := max(sgPlayerH, sgScaleY(8, cfg))
	capY := max(0, splitY-2)

	for pass := 0; pass < 3; pass++ {
		changed := false
		for y := splitY + 1; y < gridH-1; y++ {
			for x := 1; x < gridW-1; x++ {
				// Tunnel slot = passable tile directly above a solid floor cell.
				if !sgIsPassable(grid[y][x]) {
					continue
				}
				if grid[y+1][x] != sgCellSolid {
					continue
				}

				clearance := 0
				ceilingY := -1
				for yy := y; yy >= capY; yy-- {
					if !sgIsPassable(grid[yy][x]) {
						ceilingY = yy
						break
					}
					clearance++
				}
				if clearance >= minHeadroom || ceilingY < 0 {
					continue
				}

				need := minHeadroom - clearance
				for n := 0; n < need; n++ {
					ty := ceilingY - n
					if ty < capY || ty >= gridH {
						break
					}
					if grid[ty][x] == sgCellSolid {
						if tags.At(x, ty).Kind == sgSkyObjectShaftStep {
							break // Останавливаемся, не ломаем лестницу
						}
						grid[ty][x] = sgCellBackwall
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}
}

type sgAirComponent struct {
	Cells []sgPoint
}

// sgEnsureGlobalPassableConnectivity merges all isolated air pockets into the
// main playable area. Sky-zone pockets get a tagged horizontal bridge platform;
// underground pockets get a Bezier tunnel for organic appearance.
func sgEnsureGlobalPassableConnectivity(rng *rand.Rand, grid [][]int, splitY int, cfg ProcGenConfig, tags *sgSkyTagGrid) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}
	gridH := len(grid)
	gridW := len(grid[0])
	_ = gridH

	isSkyComp := func(comp sgAirComponent) bool {
		sky := 0
		for _, p := range comp.Cells {
			if p.Y < splitY {
				sky++
			}
		}
		return sky > len(comp.Cells)/2
	}

	// sgNearestUndergroundPoint finds the nearest cell in comp that is at or
	// below splitY. Falls back to any cell if no underground cells exist.
	nearestUnderground := func(from sgPoint, comp sgAirComponent) (sgPoint, int) {
		best := sgPoint{}
		bestDist := 9999999
		step := 1
		if len(comp.Cells) > 500 {
			step = len(comp.Cells) / 200
		}
		for i := 0; i < len(comp.Cells); i += step {
			p := comp.Cells[i]
			if p.Y < splitY {
				continue // skip sky cells when connecting underground
			}
			dx, dy := p.X-from.X, p.Y-from.Y
			d := dx*dx + dy*dy
			if d < bestDist {
				bestDist = d
				best = p
			}
		}
		if bestDist == 9999999 {
			// No underground cells — fall back to any
			return sgNearestPointInComponent(from, comp)
		}
		return best, bestDist
	}

	// ── Pass A: fix isolated UNDERGROUND components ───────────────────────────
	// Anchor = largest underground component. Connect all other underground
	// isolated pockets to it via a Bezier tunnel through solid earth.
	// Key: we only look for anchor cells at y >= splitY so we never accidentally
	// "connect" a shaft to the sky zone (it's already connected to the sky by
	// definition — we need an underground path to the hub network).
	for pass := 0; pass < 100; pass++ {
		comps := sgPassableComponentsAll(grid)

		// Collect underground components (majority of cells below splitY).
		type ugComp struct {
			idx  int
			size int
		}
		underground := make([]ugComp, 0, len(comps))
		for i, c := range comps {
			ugCells := 0
			for _, p := range c.Cells {
				if p.Y >= splitY {
					ugCells++
				}
			}
			if ugCells > 0 {
				underground = append(underground, ugComp{i, ugCells})
			}
		}
		if len(underground) <= 1 {
			break
		}

		// Largest underground component = anchor.
		anchorUG := underground[0]
		for _, u := range underground[1:] {
			if u.size > anchorUG.size {
				anchorUG = u
			}
		}
		anchorComp := comps[anchorUG.idx]

		// Find the closest isolated underground component.
		bestIdx := -1
		minDist := 9999999
		var bestFrom, bestTo sgPoint
		for _, u := range underground {
			if u.idx == anchorUG.idx {
				continue
			}
			comp := comps[u.idx]
			step := 1
			if len(comp.Cells) > 300 {
				step = len(comp.Cells) / 100
			}
			for j := 0; j < len(comp.Cells); j += step {
				p1 := comp.Cells[j]
				if p1.Y < splitY {
					continue // only use underground cells as source
				}
				p2, d := nearestUnderground(p1, anchorComp)
				if d < minDist {
					minDist = d
					bestFrom = p1
					bestTo = p2
					bestIdx = u.idx
				}
			}
		}
		if bestIdx < 0 {
			break
		}

		// Connect isolated underground region to anchor via Bezier, radius 3.
		sgCarveBeziez(rng, grid, bestFrom, bestTo, 3)
	}

	// ── Pass B: fix isolated SKY components ───────────────────────────────────
	// Anchor = largest sky component. Connect isolated sky pockets via bridges.
	for pass := 0; pass < 50; pass++ {
		comps := sgPassableComponentsAll(grid)
		if len(comps) <= 1 {
			return
		}
		anchorIdx := sgLargestAirComponentIndex(comps)
		if anchorIdx < 0 {
			return
		}
		anchorComp := comps[anchorIdx]

		bestTargetIdx := -1
		minDistSq := 9999999
		var bestFrom, bestTo sgPoint

		for i, comp := range comps {
			if i == anchorIdx || !isSkyComp(comp) {
				continue // only fix sky components in this pass
			}
			for j := 0; j < len(comp.Cells); j += 3 {
				p1 := comp.Cells[j]
				if p1.Y >= splitY {
					continue
				}
				p2, distSq := sgNearestPointInComponent(p1, anchorComp)
				if distSq < minDistSq {
					minDistSq = distSq
					bestFrom = p1
					bestTo = p2
					bestTargetIdx = i
				}
			}
		}
		if bestTargetIdx < 0 {
			break
		}

		bridgeY := (bestFrom.Y + bestTo.Y) / 2
		bridgeX1 := min(bestFrom.X, bestTo.X)
		bridgeX2 := max(bestFrom.X, bestTo.X)
		bridgeW := bridgeX2 - bridgeX1 + 3
		bridgeY = sgClamp(bridgeY, 1, splitY-2)
		bridgeX1 = sgClamp(bridgeX1, 1, max(1, gridW-bridgeW-1))
		sgWritePlatformRect(grid, bridgeX1, bridgeY, bridgeW, 1)
		if tags != nil {
			objID := tags.NewObject(sgSkyObjectShaftStep)
			tags.MarkRect(bridgeX1, bridgeY, bridgeW, 1, objID, sgSkyObjectShaftStep)
		}
		for x := bridgeX1; x < bridgeX1+bridgeW && x < gridW; x++ {
			for clearY := max(0, bridgeY-sgPlayerH); clearY < bridgeY; clearY++ {
				sgCarveCell(grid, x, clearY)
			}
		}
	}
}

func sgPassableComponentsAll(grid [][]int) []sgAirComponent {
	gridH := len(grid)
	gridW := len(grid[0])
	visited := make([][]bool, gridH)
	for y := range visited {
		visited[y] = make([]bool, gridW)
	}

	components := make([]sgAirComponent, 0, 8)
	queue := make([]sgPoint, 0, gridW)
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

	for y := 0; y < gridH; y++ {
		for x := 0; x < gridW; x++ {
			if visited[y][x] || !sgIsPassable(grid[y][x]) {
				continue
			}
			comp := sgAirComponent{}
			queue = append(queue[:0], sgPoint{X: x, Y: y})
			visited[y][x] = true

			for head := 0; head < len(queue); head++ {
				p := queue[head]
				comp.Cells = append(comp.Cells, p)
				for _, d := range dirs {
					nx := p.X + d[0]
					ny := p.Y + d[1]
					if nx < 0 || nx >= gridW || ny < 0 || ny >= gridH {
						continue
					}
					if visited[ny][nx] || !sgIsPassable(grid[ny][nx]) {
						continue
					}
					visited[ny][nx] = true
					queue = append(queue, sgPoint{X: nx, Y: ny})
				}
			}
			components = append(components, comp)
		}
	}
	return components
}

func sgLargestAirComponentIndex(comps []sgAirComponent) int {
	bestIdx := -1
	bestSize := -1
	for i := range comps {
		if len(comps[i].Cells) > bestSize {
			bestSize = len(comps[i].Cells)
			bestIdx = i
		}
	}
	return bestIdx
}

func sgNearestAirComponentToAnchor(comps []sgAirComponent, anchorIdx int) int {
	if anchorIdx < 0 || anchorIdx >= len(comps) || len(comps[anchorIdx].Cells) == 0 {
		return -1
	}
	anchorRef := comps[anchorIdx].Cells[len(comps[anchorIdx].Cells)/2]
	bestIdx := -1
	bestDist := int(^uint(0) >> 1)
	for i := range comps {
		if i == anchorIdx || len(comps[i].Cells) == 0 {
			continue
		}
		ref := comps[i].Cells[len(comps[i].Cells)/2]
		dx := ref.X - anchorRef.X
		dy := ref.Y - anchorRef.Y
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return bestIdx
}

func sgNearestPointInComponent(from sgPoint, comp sgAirComponent) (sgPoint, int) {
	best := sgPoint{}
	bestDistSq := 9999999

	step := 1
	if len(comp.Cells) > 500 {
		step = len(comp.Cells) / 100
	}

	for i := 0; i < len(comp.Cells); i += step {
		p := comp.Cells[i]
		dx := p.X - from.X
		dy := p.Y - from.Y
		d := dx*dx + dy*dy
		if d < bestDistSq {
			bestDistSq = d
			best = p
		}
	}
	return best, bestDistSq
}

func sgPickLedgeX(rng *rand.Rand, grid [][]int, comp sgComponent, y int, width int) (int, bool) {
	minX := comp.MinX
	maxX := comp.MaxX - width + 1
	if maxX < minX {
		return 0, false
	}

	bestX := -1
	bestScore := -1
	tries := 24
	for i := 0; i < tries; i++ {
		x0 := minX + rng.Intn(maxX-minX+1)
		score := 0
		for dx := 0; dx < width; dx++ {
			x := x0 + dx
			if y <= 0 || y >= len(grid)-1 {
				continue
			}
			if sgIsPassable(grid[y][x]) && sgIsPassable(grid[y-1][x]) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestX = x0
		}
		if score == width {
			break
		}
	}
	if bestX < 0 || bestScore*2 < width {
		return 0, false
	}
	return bestX, true
}

// sgPickLedgeXBiased is like sgPickLedgeX but prefers positions close to biasX.
func sgPickLedgeXBiased(rng *rand.Rand, grid [][]int, comp sgComponent, y, width, biasX int) (int, bool) {
	minX := comp.MinX
	maxX := comp.MaxX - width + 1
	if maxX < minX {
		return 0, false
	}

	bestX := -1
	bestScore := -1
	tries := 24
	for i := 0; i < tries; i++ {
		// Sample near biasX with some spread.
		spread := (maxX - minX + 1) / 3
		if spread < 1 {
			spread = 1
		}
		x0 := biasX + rng.Intn(spread*2+1) - spread
		x0 = sgClamp(x0, minX, maxX)
		score := 0
		for dx := 0; dx < width; dx++ {
			x := x0 + dx
			if y <= 0 || y >= len(grid)-1 {
				continue
			}
			if sgIsPassable(grid[y][x]) && sgIsPassable(grid[y-1][x]) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestX = x0
		}
		if score == width {
			break
		}
	}
	if bestX < 0 || bestScore*2 < width {
		return 0, false
	}
	return bestX, true
}

type sgComponent struct {
	MinX int
	MaxX int
	MinY int
	MaxY int
}

func sgPassableComponents(grid [][]int, splitY int) []sgComponent {
	gridH := len(grid)
	gridW := len(grid[0])
	visited := make([][]bool, gridH)
	for y := range visited {
		visited[y] = make([]bool, gridW)
	}

	components := make([]sgComponent, 0, 12)
	queue := make([]sgPoint, 0, gridW)

	for y := splitY; y < gridH; y++ {
		for x := 0; x < gridW; x++ {
			if visited[y][x] || !sgIsPassable(grid[y][x]) {
				continue
			}
			comp := sgComponent{MinX: x, MaxX: x, MinY: y, MaxY: y}
			queue = append(queue[:0], sgPoint{X: x, Y: y})
			visited[y][x] = true
			for head := 0; head < len(queue); head++ {
				p := queue[head]
				if p.X < comp.MinX {
					comp.MinX = p.X
				}
				if p.X > comp.MaxX {
					comp.MaxX = p.X
				}
				if p.Y < comp.MinY {
					comp.MinY = p.Y
				}
				if p.Y > comp.MaxY {
					comp.MaxY = p.Y
				}

				next := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
				for _, d := range next {
					nx := p.X + d[0]
					ny := p.Y + d[1]
					if nx < 0 || nx >= gridW || ny < splitY || ny >= gridH {
						continue
					}
					if visited[ny][nx] || !sgIsPassable(grid[ny][nx]) {
						continue
					}
					visited[ny][nx] = true
					queue = append(queue, sgPoint{X: nx, Y: ny})
				}
			}
			components = append(components, comp)
		}
	}
	return components
}

func sgIsPassable(cell int) bool {
	// sgCellPlatform (3) is a one-way platform: passable from below / from any
	// horizontal direction, so it counts as "not solid" for BFS and placement checks.
	return cell != sgCellSolid
}

func sgCellsForValue(grid [][]int, target int) [][2]int {
	cells := make([][2]int, 0)
	for y := range grid {
		for x := range grid[y] {
			if grid[y][x] == target {
				cells = append(cells, [2]int{x, y})
			}
		}
	}
	return cells
}

func sgSpawnPointsFromInlets(inlets []sgInlet, cfg ProcGenConfig) []shared.Vec2 {
	count := cfg.NumRooms
	if count < 2 {
		count = 2
	}
	if len(inlets) == 0 {
		return nil
	}

	clearancePx := cfg.Player.ColliderH + 8
	candidates := make([]shared.Vec2, 0, len(inlets)*3)
	seen := map[[2]int]bool{}

	for _, inlet := range inlets {
		offset := max(1, inlet.Width/3)
		xs := []int{inlet.CenterX, inlet.CenterX - offset, inlet.CenterX + offset}
		for _, x := range xs {
			x = max(2, x)
			key := [2]int{x, inlet.Y}
			if seen[key] {
				continue
			}
			seen[key] = true
			py := float64(inlet.Y*sgBlockSizePx) - clearancePx
			if py < 8 {
				py = 8
			}
			candidates = append(candidates, shared.Vec2{X: float64(x * sgBlockSizePx), Y: py})
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	spawns := make([]shared.Vec2, 0, count)
	for i := 0; i < count; i++ {
		spawns = append(spawns, candidates[i%len(candidates)])
	}
	return spawns
}

func sgDistanceSq(a, b sgPoint) float64 {
	dx := float64(a.X - b.X)
	dy := float64(a.Y - b.Y)
	return dx*dx + dy*dy
}

func sgScaleX(base int, cfg ProcGenConfig) int {
	return max(1, int(math.Round(float64(base)*float64(cfg.GridW)/float64(sgBaseGridW))))
}

func sgScaleY(base int, cfg ProcGenConfig) int {
	return max(1, int(math.Round(float64(base)*float64(cfg.GridH)/float64(sgBaseGridH))))
}

func sgScaledRadius(cfg ProcGenConfig) int {
	scaleW := float64(cfg.GridW) / float64(sgBaseGridW)
	scaleH := float64(cfg.GridH) / float64(sgBaseGridH)
	scale := math.Min(scaleW, scaleH)
	if scale <= 0 {
		scale = 1
	}
	r := int(math.Round(4 * scale))
	if r < 2 {
		r = 2
	}
	return r
}

func sgMinHubGap(cfg ProcGenConfig) int {
	gap := min(sgScaleX(10, cfg), sgScaleY(10, cfg))
	if gap < 4 {
		gap = 4
	}
	return gap
}

func sgEdgeKey(a, b int) [2]int {
	if a > b {
		a, b = b, a
	}
	return [2]int{a, b}
}

func sgClamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// GenerateRects greedily merges same-valued neighbouring cells into large
// block-space rectangles. Returned rect coordinates are expressed in grid cells.
func GenerateRects(grid [][]int, targetValue int) []shared.Rect {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return nil
	}
	h := len(grid)
	w := len(grid[0])
	visited := make([][]bool, h)
	for y := 0; y < h; y++ {
		visited[y] = make([]bool, w)
	}

	rects := make([]shared.Rect, 0, 32)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if visited[y][x] || grid[y][x] != targetValue {
				continue
			}

			rw := 0
			for xx := x; xx < w; xx++ {
				if visited[y][xx] || grid[y][xx] != targetValue {
					break
				}
				rw++
			}

			rh := 1
			for yy := y + 1; yy < h; yy++ {
				ok := true
				for xx := x; xx < x+rw; xx++ {
					if visited[yy][xx] || grid[yy][xx] != targetValue {
						ok = false
						break
					}
				}
				if !ok {
					break
				}
				rh++
			}

			for yy := y; yy < y+rh; yy++ {
				for xx := x; xx < x+rw; xx++ {
					visited[yy][xx] = true
				}
			}

			rects = append(rects, shared.Rect{
				X: float64(x),
				Y: float64(y),
				W: float64(rw),
				H: float64(rh),
			})
		}
	}
	return rects
}

// ─── Simplex Noise (2D, self-contained) ──────────────────────────────────────

type sgSimplexNoise struct {
	perm [512]int
}

func sgNewSimplexNoise(seed int64) *sgSimplexNoise {
	sn := &sgSimplexNoise{}
	// Build a permutation table from the seed using a simple LCG shuffle.
	src := make([]int, 256)
	for i := range src {
		src[i] = i
	}
	r := seed
	for i := 255; i > 0; i-- {
		r = r*6364136223846793005 + 1442695040888963407
		j := int(uint64(r)>>33) % (i + 1)
		src[i], src[j] = src[j], src[i]
	}
	for i := 0; i < 512; i++ {
		sn.perm[i] = src[i&255]
	}
	return sn
}

var sgSimplexGrad2 = [8][2]float64{
	{1, 1}, {-1, 1}, {1, -1}, {-1, -1},
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
}

func sgSimplexDot2(g [2]float64, x, y float64) float64 { return g[0]*x + g[1]*y }

func (sn *sgSimplexNoise) Eval2(x, y float64) float64 {
	const F2 = 0.36602540378 // (sqrt(3)-1)/2
	const G2 = 0.21132486541 // (3-sqrt(3))/6
	s := (x + y) * F2
	i := int(math.Floor(x + s))
	j := int(math.Floor(y + s))
	t := float64(i+j) * G2
	x0 := x - (float64(i) - t)
	y0 := y - (float64(j) - t)
	var i1, j1 int
	if x0 > y0 {
		i1, j1 = 1, 0
	} else {
		i1, j1 = 0, 1
	}
	x1 := x0 - float64(i1) + G2
	y1 := y0 - float64(j1) + G2
	x2 := x0 - 1 + 2*G2
	y2 := y0 - 1 + 2*G2
	ii := i & 255
	jj := j & 255
	gi0 := sn.perm[ii+sn.perm[jj]] % 8
	gi1 := sn.perm[ii+i1+sn.perm[jj+j1]] % 8
	gi2 := sn.perm[ii+1+sn.perm[jj+1]] % 8
	n0, n1, n2 := 0.0, 0.0, 0.0
	if t0 := 0.5 - x0*x0 - y0*y0; t0 >= 0 {
		t0 *= t0
		n0 = t0 * t0 * sgSimplexDot2(sgSimplexGrad2[gi0], x0, y0)
	}
	if t1 := 0.5 - x1*x1 - y1*y1; t1 >= 0 {
		t1 *= t1
		n1 = t1 * t1 * sgSimplexDot2(sgSimplexGrad2[gi1], x1, y1)
	}
	if t2 := 0.5 - x2*x2 - y2*y2; t2 >= 0 {
		t2 *= t2
		n2 = t2 * t2 * sgSimplexDot2(sgSimplexGrad2[gi2], x2, y2)
	}
	return 70.0 * (n0 + n1 + n2) // range roughly [-1, 1]
}

// sgApplySimplexTexture carves organic niche pockets into solid underground walls.
// Called after hub placement and before MST tunneling so that tunnels cut through
// the textured terrain naturally. Only punches holes — never fills existing air.
func sgApplySimplexTexture(rng *rand.Rand, grid [][]int, splitY int) {
	if len(grid) == 0 {
		return
	}
	gridH, gridW := len(grid), len(grid[0])
	noise := sgNewSimplexNoise(rng.Int63())

	const scale1 = 0.07 // large cave-pocket frequency
	const scale2 = 0.18 // small niche frequency
	const threshold = -0.55

	for y := splitY + 3; y < gridH-1; y++ {
		for x := 1; x < gridW-1; x++ {
			if grid[y][x] != sgCellSolid {
				continue
			}
			// Only extend existing passages downward: require the 3 cells above to be
			// air (y-1, y-2, y-3). This ensures the carved cell is always reachable
			// (player can fall in from the passage above) and creates no isolated pockets.
			if y-3 < 0 || grid[y-1][x] != sgCellAir || grid[y-2][x] != sgCellAir || grid[y-3][x] != sgCellAir {
				continue
			}
			n := 0.6*noise.Eval2(float64(x)*scale1, float64(y)*scale1) +
				0.4*noise.Eval2(float64(x)*scale2, float64(y)*scale2)
			if n < threshold {
				grid[y][x] = sgCellAir
			}
		}
	}
}

// sgFillThinGaps closes single-cell horizontal gaps between adjacent solid platforms.
// Extracted from the old sgCleanUpAndOptimizeGeometry so it can be called independently.
func sgFillThinGaps(grid [][]int) {
	if len(grid) == 0 {
		return
	}
	height := len(grid)
	width := len(grid[0])
	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			if grid[y][x] == sgCellAir &&
				grid[y][x-1] == sgCellSolid && grid[y][x+1] == sgCellSolid &&
				grid[y-1][x] == sgCellAir {
				grid[y][x] = sgCellSolid
			}
		}
	}
}

// ─── Unreachable Sky Platform Pruning ─────────────────────────────────────────

// sgPruneUnreachableSkyPlatforms removes solid platform runs in the sky zone
// that the player can never stand on. It runs a full BFS, then iterates over
// every contiguous horizontal solid run in y < splitY. A run is kept if at
// least one player-sized standing position (feet at y-1, width sgPlayerW)
// overlapping the run is marked reachable. Hub cells are never removed.
// Returns the number of cells pruned.
func sgPruneUnreachableSkyPlatforms(grid [][]int, tags *sgSkyTagGrid, splitY int) int {
	gridH, gridW := len(grid), len(grid[0])
	if gridH == 0 || gridW == 0 {
		return 0
	}

	canReach := make([][]bool, gridH)
	for i := range canReach {
		canReach[i] = make([]bool, gridW)
	}
	sgMarkReachableWithTags(grid, canReach, splitY, tags)

	pW := sgPlayerW
	pruned := 0

	for y := 1; y < splitY; y++ {
		x := 0
		for x < gridW {
			c := grid[y][x]
			if c != sgCellSolid && c != sgCellPlatform {
				x++
				continue
			}
			// Only consider floor cells (air directly above).
			// Interior hub walls have solid above them and should not be touched.
			if grid[y-1][x] == sgCellSolid {
				x++
				continue
			}
			// Collect the full contiguous run (solid or platform) at this row.
			runStart := x
			for x < gridW && (grid[y][x] == sgCellSolid || grid[y][x] == sgCellPlatform) {
				x++
			}
			runEnd := x - 1 // inclusive

			// Hub cells are protected — never remove them.
			if tags != nil {
				isHub := false
				for cx := runStart; cx <= runEnd; cx++ {
					if tags.At(cx, y).Kind == sgSkyObjectHub {
						isHub = true
						break
					}
				}
				if isHub {
					continue
				}
			}

			// Check reachability: any canReach position that overlaps this run.
			// Player feet at y-1, left edge cx; occupies columns cx..cx+pW-1.
			// Overlap with [runStart..runEnd] when cx <= runEnd and cx+pW-1 >= runStart.
			reachable := false
			if y-1 >= 0 {
				for cx := runStart - pW + 1; cx <= runEnd; cx++ {
					if cx < 0 || cx+pW-1 >= gridW {
						continue
					}
					if canReach[y-1][cx] {
						reachable = true
						break
					}
				}
			}

			if !reachable {
				// Remove the entire unreachable platform run.
				for cx := runStart; cx <= runEnd; cx++ {
					grid[y][cx] = sgCellAir
					if tags != nil {
						tags.ClearCell(cx, y)
					}
				}
				pruned += runEnd - runStart + 1
			}
		}
	}

	return pruned
}

// ─── Pre-flight Validation ────────────────────────────────────────────────────

// sgRunPreflightValidation runs a reachability BFS from the surface and checks
// that all sky hubs and all inlets are accessible to the player. Returns false
// if any critical area is unreachable — the caller should discard this seed.
func sgRunPreflightValidation(grid [][]int, tags *sgSkyTagGrid, skyHubs []sgSkyHub, inlets []sgInlet, splitY int) bool {
	if len(grid) == 0 {
		return false
	}
	height, width := len(grid), len(grid[0])
	canReach := make([][]bool, height)
	for i := range canReach {
		canReach[i] = make([]bool, width)
	}
	sgMarkReachableWithTags(grid, canReach, splitY, tags)

	// Count reachable cells for diagnostics.
	totalReach := 0
	for y := range canReach {
		for _, b := range canReach[y] {
			if b {
				totalReach++
			}
		}
	}

	// Every sky hub must have at least one reachable cell on the row above it.
	for hi, hub := range skyHubs {
		topY := hub.Y - 1
		if topY < 0 || topY >= height {
			continue
		}
		accessible := false
		for x := hub.X; x < hub.X+hub.W && x < width; x++ {
			if canReach[topY][x] {
				accessible = true
				break
			}
		}
		if !accessible {
			sgPreflightFailReason = "check1:hub_unreachable:idx=" + itoa(hi) + ",reachCells=" + itoa(totalReach)
			return false
		}
	}

	// Every inlet must be reachable from the spawn zone at splitY.
	for ii, inlet := range inlets {
		cx := inlet.CenterX
		if cx < 0 || cx >= width {
			continue
		}
		if !canReach[splitY][cx] {
			sgPreflightFailReason = "check2:inlet_unreachable:idx=" + itoa(ii) + ",reachCells=" + itoa(totalReach)
			return false
		}
	}

	// Full underground connectivity: every floor cell (solid with air directly
	//    above it) must be reachable from spawn. This catches isolated pockets that
	//    the player could fall into but never escape. The simplex texture pass is
	//    designed to only extend existing connected passages, so no isolated niches
	//    should exist after the full pipeline.
	unreachableFloors := 0
	for y := splitY + 2; y < height-2; y++ {
		for x := 1; x < width-1; x++ {
			if grid[y][x] == sgCellSolid && grid[y-1][x] == sgCellAir {
				if !canReach[y-1][x] {
					unreachableFloors++
				}
			}
		}
	}
	if unreachableFloors > 0 {
		sgPreflightFailReason = "check3:unreachable_floor_cells=" + itoa(unreachableFloors) + ",reachCells=" + itoa(totalReach)
		return false
	}
	sgPreflightFailReason = ""
	return true
}
