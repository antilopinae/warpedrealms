package content

import (
	"math"
	"math/rand"
	"sort"

	"warpedrealms/shared"
	"warpedrealms/world"
)

const (
	sgBaseGridW = 200
	sgBaseGridH = 150

	sgCellAir      = 0
	sgCellSolid    = 1
	sgCellBackwall = 2

	sgBlockSizePx = 16
)

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
	return sgPoint{X: h.X + h.W/2, Y: h.Y + h.H/2}
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
	PrimarySpawn  shared.Vec2
	SpawnPoints   []shared.Vec2
}

// GenerateSandwichLocation builds one room using HK-style cave hubs.
// Geometry is produced in block-space and exported as LDtkWriteLevel solid cells.
func GenerateSandwichLocation(rng *rand.Rand, id string, cfg ProcGenConfig) SandwichLocation {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
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

	inlets := sgBuildInlets(gridW, splitY)
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

	carveRadius := sgScaledRadius(cfg)
	edges := make(map[[2]int]bool)

	for _, inlet := range inlets {
		nearest := sgNearestHub(inlet, hubs)
		hubCenter := hubs[nearest].Center()
		shaftBottom := sgClamp(hubCenter.Y, splitY+max(4, sgScaleY(6, cfg)), gridH-2)
		shaft := sgCarveInletShaft(grid, inlet, shaftBottom)
		sgAddInletZigZagPlatforms(grid, shaft, cfg, skyTags)

		bottom := sgPoint{X: shaft.CenterX, Y: shaft.BottomY}
		sgCarveLine(grid, bottom, sgPoint{X: hubCenter.X, Y: shaft.BottomY}, carveRadius)
		sgCarveLine(grid, sgPoint{X: hubCenter.X, Y: shaft.BottomY}, hubCenter, carveRadius)
	}

	sgConnectHubMST(rng, grid, hubs, carveRadius, edges)
	if len(hubs) >= 2 && rng.Float64() < 0.20 {
		a, b := sgPickLoopHubPair(rng, len(hubs), edges)
		if a >= 0 && b >= 0 {
			sgCarveL(rng, grid, hubs[a].Center(), hubs[b].Center(), carveRadius)
		}
	}

	sgAddReturnLoops(rng, grid, splitY, inlets, hubs, carveRadius, cfg)
	sgAddExitTunnels(rng, grid, splitY, cfg, inlets)
	sgAddWallLedges(grid, splitY, cfg)
	sgAddInternalLedges(rng, grid, splitY, cfg)
	skyDbg := sgPopulateSkyAndObjectsWithTags(rng, grid, cfg, splitY, skyTags)
	sgApplySkyStrictAntiStacking(grid, skyTags, splitY, cfg)
	sgApplyGroundAirCorridor(grid, splitY, cfg, inlets, skyDbg.Chains)
	sgApplyTunnelHeadroom(grid, splitY, cfg)
	sgApplyTunnelHeadroom(grid, splitY, cfg)
	sgEnsureJumpConnectivity(grid, splitY, cfg)
	sgEnsureGlobalPassableConnectivity(grid, splitY, cfg)
	sgEnsureGroundToSkyAccessibility(rng, grid, skyTags, skyDbg.Hubs, skyDbg.Chains, inlets, splitY, cfg)
	sgAddSkyPenthouses(rng, grid, skyTags, skyDbg.Hubs, cfg)

	solidCells := sgCellsForValue(grid, sgCellSolid)
	spawnPoints := sgSpawnPointsFromInlets(inlets, cfg)
	primarySpawn := shared.Vec2{}
	if len(spawnPoints) > 0 {
		primarySpawn = spawnPoints[0]
	}

	entities := make([]world.LDtkWriteEntity, 0, 1)
	if len(spawnPoints) > 0 {
		entities = append(entities, world.LDtkWriteEntity{
			Identifier: "Player",
			PX:         int(primarySpawn.X),
			PY:         int(primarySpawn.Y),
			W:          26,
			H:          76,
		})
	}

	level := world.LDtkWriteLevel{
		ID:         id,
		GridW:      gridW,
		GridH:      gridH,
		GridSize:   sgBlockSizePx,
		SolidCells: solidCells,
		Entities:   entities,
	}

	return SandwichLocation{
		Grid:          grid,
		Inlets:        inlets,
		Hubs:          hubs,
		Level:         level,
		SolidRects:    GenerateRects(grid, sgCellSolid),
		BackwallRects: GenerateRects(grid, sgCellBackwall),
		PrimarySpawn:  primarySpawn,
		SpawnPoints:   spawnPoints,
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

		if !modified {
			break
		}
	}
}

// sgDigToSafety копает строгие прямоугольные коридоры и шахты.
func sgDigToSafety(grid [][]int, canEscape [][]bool, splitY, startX, startY int) {
	width := len(grid[0])
	height := len(grid)

	// 1. Ищем ближайшую безопасную точку (по воздуху и полу)
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

	// 2. Копаем ВЕРТИКАЛЬНУЮ ШАХТУ ВВЕРХ
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

	// Ширина шахты = 6 блоков
	for y := digStartY; y != targetY+stepY; y += stepY {
		for dx := -2; dx <= 3; dx++ {
			nx := startX + dx
			if nx >= 1 && nx < width-1 && y >= 1 && y < height-1 {
				if grid[y][nx] == sgCellSolid {
					grid[y][nx] = sgCellBackwall
				}
			}
		}
	}

	// 3. Копаем ГОРИЗОНТАЛЬНЫЙ ТУННЕЛЬ ВБОК
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

	// 4. Если прорубили наверх — ставим зигзагообразные платформы
	if targetY < startY-6 {
		shaftLeft := startX - 2
		shaftRight := startX + 2
		platW := 2
		sideLeft := true

		for y := startY - 6; y >= targetY; y -= 6 {
			platX := shaftLeft
			if !sideLeft {
				platX = shaftRight - platW + 1
			}

			if platX < 1 {
				platX = 1
			}
			if platX+platW > width-1 {
				platX = width - platW - 1
			}

			for i := 0; i < platW; i++ {
				px := platX + i
				if px >= 1 && px < width-1 {
					grid[y][px] = sgCellSolid
					// Прорубаем 3 блока воздуха строго над платформой для прыжка
					for pDy := 1; pDy <= 3; pDy++ {
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
	width, height := len(grid[0]), len(grid)
	pW, pH := 2, 4 // Реальные габариты игрока

	// Проверка: влезет ли "тело" игрока 2х5 в точку x,y
	canFit := func(x, y int) bool {
		for dy := 0; dy < pH; dy++ {
			for dx := 0; dx < pW; dx++ {
				tx, ty := x+dx, y-dy
				if ty < 0 || ty >= height || tx < 0 || tx >= width || grid[ty][tx] == sgCellSolid {
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

		// 1. ПЕРЕМЕЩЕНИЕ ПО ПОЛУ (влево-вправо)
		for _, dx := range []int{-1, 1} {
			nx := curr.x + dx
			if nx >= 0 && nx < width-pW && canFit(nx, curr.y) && !canEscape[curr.y][nx] {
				// Проверяем наличие пола под ногами игрока (2 блока ширины)
				if grid[curr.y+1][nx] == sgCellSolid || grid[curr.y+1][nx+1] == sgCellSolid {
					canEscape[curr.y][nx] = true
					queue = append(queue, point{nx, curr.y})
				}
			}
		}

		// 2. ПРЫЖКИ (вертикаль до 7, горизонталь до 4)
		for dy := -7; dy <= 7; dy++ {
			for dx := -4; dx <= 4; dx++ {
				nx, ny := curr.x+dx, curr.y+dy
				if nx < 0 || nx >= width-pW || ny < pH || ny >= height-1 {
					continue
				}

				if !canEscape[ny][nx] && canFit(nx, ny) {
					// Приземление только на твердый пол
					if grid[ny+1][nx] == sgCellSolid || grid[ny+1][nx+1] == sgCellSolid {
						// Проверка траектории: не летим ли сквозь стену
						if sgIsJumpPathClear(grid, curr.x, curr.y, nx, ny, pW, pH) {
							canEscape[ny][nx] = true
							queue = append(queue, point{nx, ny})
						}
					}
				}
			}
		}
	}
}

// Упрощенная проверка: чист ли прямоугольник между точками прыжка
func sgIsJumpPathClear(grid [][]int, x1, y1, x2, y2, pW, pH int) bool {
	minX, maxX := min(x1, x2), max(x1, x2)
	minY, maxY := min(y1, y2)-2, max(y1, y2) // -2 блока запаса над головой для дуги
	for tx := minX; tx <= maxX+pW-1; tx++ {
		for ty := minY; ty <= maxY; ty++ {
			if tx < 0 || tx >= len(grid[0]) || ty < 0 || ty >= len(grid) {
				return false
			}
			if grid[ty][tx] == sgCellSolid {
				return false
			}
		}
	}
	return true
}

func sgEnsureGroundToSkyAccessibility(rng *rand.Rand, grid [][]int, tags *sgSkyTagGrid, hubs []sgSkyHub, chains []sgStairChain, inlets []sgInlet, splitY int, cfg ProcGenConfig) {
	if len(grid) == 0 {
		return
	}
	stepW := max(2, sgScaleX(3, cfg))

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

	for {
		// 1. Ищем РЕАЛЬНУЮ землю под следующим шагом (учитывая туннели)
		nx := curr.X + ch.TrendDir*(hMin+rng.Intn(hMax-hMin+1))
		nx = sgClamp(nx, 2, gridW-stepW-2)

		realGroundY := sgFindDynamicGroundY(grid, nx, splitY)

		// 2. Рассчитываем Y следующей ступеньки
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

		// 3. Ставим платформу
		sgWriteSolidRect(grid, nx, ny, stepW, 1)

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

func sgAddSkyPenthouses(rng *rand.Rand, grid [][]int, tags *sgSkyTagGrid, hubs []sgSkyHub, cfg ProcGenConfig) {
	if tags == nil || len(hubs) == 0 {
		return
	}

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

						sgWriteSolidRect(grid, lx, py, landingW, 1)
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
}

func sgBuildInlets(gridW int, splitY int) []sgInlet {
	anchors := []float64{0.2, 0.5, 0.8}
	width := max(4, int(math.Round(6.0*float64(gridW)/float64(sgBaseGridW))))
	if width%2 != 0 {
		width++
	}
	inlets := make([]sgInlet, 0, len(anchors))
	for _, a := range anchors {
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

	// 1. Размещаем основные хабы (летающие острова)
	hubs := sgPlaceSkyHubs(rng, grid, tags, hubCount, skyTopMin, skyTopMax, splitY, cfg)
	dbg.Hubs = append(dbg.Hubs, hubs...)

	// 2. Генерируем только крутые начальные "хвосты" для каждого хаба
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
	hubCount = max(2, gridW/100)
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
	stepW := max(2, sgScaleX(3, cfg))

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

		sgWriteSolidRect(grid, x, y, stepW, 1)
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

			// ПРОВЕРКА: Если цепочка уже внизу, не добавляем ей мусора
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
					stepW := max(2, sgScaleX(3, cfg))
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

	stepW := max(2, sgScaleX(3, cfg))
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
			sgWriteSolidRect(grid, px, ty, restW, 1)
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
			sgWriteSolidRect(grid, px, forceY, restW, 1)
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

	stepW := max(2, sgScaleX(3, cfg))
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
		sgWriteSolidRect(grid, nx, y, stepW, 1)
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

	hasNonSolid := false
	for xx := x; xx < x+stepW; xx++ {
		if grid[y][xx] != sgCellSolid {
			hasNonSolid = true
			break
		}
	}
	return hasNonSolid
}

func sgHasExternalSolidNearby(grid [][]int, x, y, w, h, radius int, ignore map[[2]int]bool) bool {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return false
	}
	gridW := len(grid[0])
	gridH := len(grid)
	for yy := y - radius; yy <= y+h-1+radius; yy++ {
		if yy < 0 || yy >= gridH {
			continue
		}
		for xx := x - radius; xx <= x+w-1+radius; xx++ {
			if xx < 0 || xx >= gridW {
				continue
			}
			if grid[yy][xx] != sgCellSolid {
				continue
			}
			if ignore != nil && ignore[[2]int{xx, yy}] {
				continue
			}
			return true
		}
	}
	return false
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

func sgPlaceHubs(rng *rand.Rand, grid [][]int, splitY int, cfg ProcGenConfig) []sgHub {
	gridW := len(grid[0])
	gridH := len(grid)

	hubMinW := max(12, sgScaleX(20, cfg))
	hubMaxW := max(hubMinW+1, sgScaleX(35, cfg))
	hubMinH := max(8, sgScaleY(12, cfg))
	hubMaxH := max(hubMinH+1, sgScaleY(18, cfg))
	minGap := sgMinHubGap(cfg)

	target := 5 + rng.Intn(3) // 5..7
	yMin := splitY + max(2, sgScaleY(10, cfg))
	yMax := max(yMin, gridH-hubMaxH-2)

	hubs := make([]sgHub, 0, target)
	maxAttempts := 1600
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
		sgCarveL(rng, grid, hubs[bestA].Center(), hubs[bestB].Center(), radius)
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

	return sgShaft{
		CenterX: inlet.CenterX,
		Left:    left,
		Right:   right,
		TopY:    topY,
		BottomY: bottomY,
	}
}

func sgAddInletZigZagPlatforms(grid [][]int, shaft sgShaft, cfg ProcGenConfig, tags *sgSkyTagGrid) {
	step := max(3, sgScaleY(6, cfg))
	ledgeW := max(2, sgScaleX(3, cfg))
	sideGap := max(1, sgScaleX(3, cfg))
	shaftW := shaft.Right - shaft.Left + 1
	if ledgeW >= shaftW {
		ledgeW = max(1, shaftW-1)
	}
	if ledgeW <= 0 {
		return
	}

	sideLeft := true
	for y := shaft.TopY + step; y <= shaft.BottomY-1; y += step {
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
				grid[y][x] = sgCellSolid
				if tags != nil && objectID > 0 {
					tags.MarkCell(x, y, objectID, sgSkyObjectShaftStep)
				}
			}
		}
		sideLeft = !sideLeft
	}
}

func sgAddReturnLoops(rng *rand.Rand, grid [][]int, splitY int, inlets []sgInlet, hubs []sgHub, carveRadius int, cfg ProcGenConfig) {
	if len(hubs) == 0 {
		return
	}

	deep := sgDeepestHubIndices(hubs, 2)
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
	}
}

// sgAddExitTunnels carves guaranteed vertical escape shafts in the tunnel layer.
// Each shaft is an 8-block wide well with 4x1 alternating steps every ~6 blocks.
func sgAddExitTunnels(rng *rand.Rand, grid [][]int, splitY int, cfg ProcGenConfig, inlets []sgInlet) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}
	gridW := len(grid[0])
	gridH := len(grid)
	if splitY <= 1 || splitY >= gridH-2 {
		return
	}

	shaftCount := 3
	shaftW := max(6, sgScaleX(8, cfg))
	if shaftW%2 != 0 {
		shaftW++
	}
	stepY := max(3, sgScaleY(6, cfg))
	stepW := max(2, sgScaleX(4, cfg))
	if stepW >= shaftW {
		stepW = max(2, shaftW-2)
	}
	if stepW <= 0 {
		return
	}

	used := map[int]bool{}
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
				sgCarveCell(grid, x, y)
				if y < splitY {
					grid[y][x] = sgCellAir
				}
			}
		}
		sgOpenSurfaceExit(grid, centerX, splitY, shaftW)

		sideLeft := true
		for y := splitY + stepY; y <= bottomY-1; y += stepY {
			x0 := left
			if !sideLeft {
				x0 = right - stepW + 1
			}
			x0 = sgClamp(x0, left, right-stepW+1)
			for dx := 0; dx < stepW; dx++ {
				x := x0 + dx
				if x < left || x > right || y < 0 || y >= gridH {
					continue
				}
				if sgIsPassable(grid[y][x]) {
					grid[y][x] = sgCellSolid
				}
			}
			sideLeft = !sideLeft
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
	gridW := len(grid[0])
	gridH := len(grid)
	left := centerX - width/2
	right := left + width - 1
	left = sgClamp(left, 1, gridW-2)
	right = sgClamp(right, 1, gridW-2)
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
					grid[y][x] = sgCellSolid
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
	threshold := max(4, sgScaleY(8, cfg))
	ledgeMinW := max(2, sgScaleX(3, cfg))
	ledgeMaxW := max(ledgeMinW, sgScaleX(5, cfg))

	comps := sgPassableComponents(grid, splitY)
	for _, comp := range comps {
		height := comp.MaxY - comp.MinY + 1
		if height <= threshold {
			continue
		}
		count := 1 + height/(threshold*2)
		if count > 3 {
			count = 3
		}
		for i := 0; i < count; i++ {
			y := comp.MinY + (i+1)*height/(count+1)
			y += rng.Intn(3) - 1
			y = sgClamp(y, comp.MinY+1, comp.MaxY-1)

			w := ledgeMinW
			if ledgeMaxW > ledgeMinW {
				w += rng.Intn(ledgeMaxW - ledgeMinW + 1)
			}

			x0, ok := sgPickLedgeX(rng, grid, comp, y, w)
			if !ok {
				continue
			}
			for dx := 0; dx < w; dx++ {
				x := x0 + dx
				if sgIsPassable(grid[y][x]) {
					grid[y][x] = sgCellSolid
				}
			}
		}
	}
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
				grid[yy][x] = sgCellAir
				if tags != nil {
					tags.ClearCell(x, yy)
				}
			}
		}
	}
}

func sgApplyGroundAirCorridor(grid [][]int, splitY int, cfg ProcGenConfig, inlets []sgInlet, chains []sgStairChain) {
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

	// 1. Помечаем зоны входов в пещеры (их не трогаем)
	for _, inlet := range inlets {
		left := sgClamp(inlet.CenterX-inlet.Width/2-1, 0, gridW-1)
		right := sgClamp(left+inlet.Width+1, 0, gridW-1)
		for x := left; x <= right; x++ {
			for y := y0; y <= y1; y++ {
				keep[y][x] = true
			}
		}
	}

	// 2. Помечаем ВСЕ блоки всех ступенек в цепочках
	stepW := max(2, sgScaleX(3, cfg))
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

	// 3. Удаляем только то, что не помечено как "нужное"
	for y := y0; y <= y1; y++ {
		for x := 0; x < gridW; x++ {
			if keep[y][x] {
				continue
			}
			if grid[y][x] == sgCellSolid {
				grid[y][x] = sgCellAir
			}
		}
	}
}

func sgApplyTunnelHeadroom(grid [][]int, splitY int, cfg ProcGenConfig) {
	if len(grid) == 0 || len(grid[0]) == 0 || splitY <= 0 {
		return
	}
	gridW := len(grid[0])
	gridH := len(grid)
	minHeadroom := max(2, sgScaleY(7, cfg))
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

func sgEnsureGlobalPassableConnectivity(grid [][]int, splitY int, cfg ProcGenConfig) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}
	carveRadius := max(2, sgScaledRadius(cfg))
	maxPasses := 24
	for pass := 0; pass < maxPasses; pass++ {
		comps := sgPassableComponentsAll(grid)
		if len(comps) <= 1 {
			return
		}

		anchorIdx := sgLargestAirComponentIndex(comps)
		if anchorIdx < 0 {
			return
		}
		targetIdx := sgNearestAirComponentToAnchor(comps, anchorIdx)
		if targetIdx < 0 {
			return
		}

		targetComp := comps[targetIdx]
		anchorComp := comps[anchorIdx]
		if len(targetComp.Cells) == 0 || len(anchorComp.Cells) == 0 {
			return
		}

		from := targetComp.Cells[len(targetComp.Cells)/2]
		to, _ := sgNearestPointInComponent(from, anchorComp)

		sgCarveLine(grid, from, sgPoint{X: to.X, Y: from.Y}, carveRadius)
		sgCarveLine(grid, sgPoint{X: to.X, Y: from.Y}, to, carveRadius)

		// Keep split line passable around forced links.
		if splitY >= 0 && splitY < len(grid) {
			x0 := sgClamp(min(from.X, to.X)-1, 0, len(grid[0])-1)
			x1 := sgClamp(max(from.X, to.X)+1, 0, len(grid[0])-1)
			for x := x0; x <= x1; x++ {
				if grid[splitY][x] == sgCellSolid {
					grid[splitY][x] = sgCellBackwall
				}
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
	bestDist := int(^uint(0) >> 1)
	for _, p := range comp.Cells {
		dx := p.X - from.X
		dy := p.Y - from.Y
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			best = p
		}
	}
	return best, bestDist
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
	return cell == sgCellAir || cell == sgCellBackwall
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
