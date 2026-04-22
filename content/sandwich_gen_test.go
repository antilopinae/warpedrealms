package content

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

func TestGenerateSandwichLocation_BasicLayout(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150

	out := GenerateSandwichLocation(rand.New(rand.NewSource(42)), "room_01", cfg)
	if len(out.Grid) != cfg.GridH {
		t.Fatalf("grid height mismatch: got %d want %d", len(out.Grid), cfg.GridH)
	}
	if gotW := len(out.Grid[0]); gotW != cfg.GridW {
		t.Fatalf("grid width mismatch: got %d want %d", gotW, cfg.GridW)
	}

	splitY := cfg.GridH / 2
	skyBandH, _, _, _ := sgSkyDensity(cfg, cfg.GridW, cfg.GridH, splitY)
	skySolids := 0
	for y := 0; y < min(splitY, skyBandH); y++ {
		for x := 0; x < cfg.GridW; x++ {
			if out.Grid[y][x] == sgCellSolid {
				skySolids++
			}
		}
	}
	if skySolids == 0 {
		t.Fatalf("expected sky solids in upper layer")
	}

	if len(out.Inlets) != 3 {
		t.Fatalf("expected 3 inlets, got %d", len(out.Inlets))
	}
	expectedX := []int{40, 100, 160}
	for i, inlet := range out.Inlets {
		if inlet.Y != splitY {
			t.Fatalf("inlet %d Y mismatch: got %d want %d", i, inlet.Y, splitY)
		}
		if testAbs(inlet.CenterX-expectedX[i]) > 3 {
			t.Fatalf("inlet %d X mismatch: got %d want ~%d", i, inlet.CenterX, expectedX[i])
		}
		left := inlet.CenterX - inlet.Width/2
		right := left + inlet.Width - 1
		passable := 0
		for x := left; x <= right; x++ {
			if x >= 0 && x < cfg.GridW && sgIsPassable(out.Grid[splitY][x]) {
				passable++
			}
		}
		if passable < inlet.Width-1 {
			t.Fatalf("inlet %d was not carved wide enough: got %d passable, width %d", i, passable, inlet.Width)
		}
	}
}

func TestPopulateSkyAndObjects_DensityScaling(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	dbg := sgPopulateSkyAndObjects(rand.New(rand.NewSource(100)), grid, cfg, splitY)
	if dbg.HubCount != 2 {
		t.Fatalf("hub count mismatch for 200w: got %d want 2", dbg.HubCount)
	}
	if dbg.ExtraStepCount != 10 {
		t.Fatalf("extra step count mismatch for 200w: got %d want 10", dbg.ExtraStepCount)
	}

	cfg2 := cfg
	cfg2.GridW = 400
	splitY2 := cfg2.GridH / 2
	grid2 := makeBaseSandwichGrid(cfg2.GridW, cfg2.GridH, splitY2)
	dbg2 := sgPopulateSkyAndObjects(rand.New(rand.NewSource(101)), grid2, cfg2, splitY2)
	if dbg2.HubCount != 4 {
		t.Fatalf("hub count mismatch for 400w: got %d want 4", dbg2.HubCount)
	}
	if dbg2.ExtraStepCount != 20 {
		t.Fatalf("extra step count mismatch for 400w: got %d want 20", dbg2.ExtraStepCount)
	}
	if dbg2.HubCount <= dbg.HubCount || dbg2.ExtraStepCount <= dbg.ExtraStepCount {
		t.Fatalf("expected density-derived counts to scale with width")
	}
}

func TestPopulateSkyAndObjects_HubsEvenlySectorized(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	dbg := sgPopulateSkyAndObjects(rand.New(rand.NewSource(102)), grid, cfg, splitY)
	if len(dbg.Hubs) != dbg.HubCount {
		t.Fatalf("expected %d sky hubs, got %d", dbg.HubCount, len(dbg.Hubs))
	}
	if dbg.HubCount == 0 {
		t.Fatal("hubCount should be positive")
	}

	sectorW := cfg.GridW / dbg.HubCount
	minW := max(8, sgScaleX(30, cfg))
	maxW := max(minW, sgScaleX(50, cfg))
	for i, hub := range dbg.Hubs {
		center := hub.X + hub.W/2
		secStart := i * sectorW
		secEnd := (i + 1) * sectorW
		if i == dbg.HubCount-1 {
			secEnd = cfg.GridW
		}
		if center < secStart || center >= secEnd {
			t.Fatalf("hub %d center=%d escaped sector [%d,%d)", i, center, secStart, secEnd)
		}
		if hub.W < minW || hub.W > maxW {
			t.Fatalf("hub %d width out of range: got %d want [%d,%d]", i, hub.W, minW, maxW)
		}
	}
}

func TestPopulateSkyAndObjects_TwoWingsPerHub(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	dbg := sgPopulateSkyAndObjects(rand.New(rand.NewSource(103)), grid, cfg, splitY)
	if len(dbg.Chains) != len(dbg.Hubs)*2 {
		t.Fatalf("expected two chains per sky hub: chains=%d hubs=%d", len(dbg.Chains), len(dbg.Hubs))
	}
	if len(dbg.Chains) == 0 {
		t.Fatal("expected at least one sky chain")
	}

	type sideCount struct {
		left  int
		right int
	}
	counts := make(map[int]sideCount, len(dbg.Hubs))
	for _, chain := range dbg.Chains {
		c := counts[chain.HubIndex]
		switch chain.Side {
		case sgStairSideLeft:
			c.left++
		case sgStairSideRight:
			c.right++
		default:
			t.Fatalf("unexpected chain side: %q", chain.Side)
		}
		counts[chain.HubIndex] = c
	}
	for i := range dbg.Hubs {
		c := counts[i]
		if c.left != 1 || c.right != 1 {
			t.Fatalf("hub %d must have exactly one left and one right wing, got left=%d right=%d", i, c.left, c.right)
		}
	}
}

func TestPopulateSkyAndObjects_WingTrendDirections(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	dbg := sgPopulateSkyAndObjects(rand.New(rand.NewSource(203)), grid, cfg, splitY)
	if len(dbg.Chains) == 0 {
		t.Fatal("expected sky chains")
	}
	for i, chain := range dbg.Chains {
		if chain.HubIndex < 0 || chain.HubIndex >= len(dbg.Hubs) {
			t.Fatalf("chain %d has invalid hub index %d", i, chain.HubIndex)
		}
		hub := dbg.Hubs[chain.HubIndex]
		driftX := 0
		if len(chain.Steps) >= 2 {
			for stepIdx := 1; stepIdx < len(chain.Steps); stepIdx++ {
				driftX += chain.Steps[stepIdx].X - chain.Steps[stepIdx-1].X
			}
		} else {
			anchorX := hub.X
			if chain.Side == sgStairSideRight {
				anchorX = hub.X + hub.W - 1
			}
			driftX = chain.Steps[0].X - anchorX
		}
		switch chain.Side {
		case sgStairSideLeft:
			if driftX >= 0 {
				t.Fatalf("left wing %d drift must be negative, got %d", i, driftX)
			}
		case sgStairSideRight:
			if driftX <= 0 {
				t.Fatalf("right wing %d drift must be positive, got %d", i, driftX)
			}
		default:
			t.Fatalf("chain %d has unknown side %q", i, chain.Side)
		}
	}
}

func TestPopulateSkyAndObjects_NoForeignSolidInProjectionWindow(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	dbg := sgPopulateSkyAndObjects(rand.New(rand.NewSource(2026)), grid, cfg, splitY)
	if dbg.Tags == nil {
		t.Fatalf("expected sky tags in debug data")
	}
	projectionH := max(2, sgScaleY(10, cfg))
	stepW := max(2, sgScaleX(3, cfg))

	for _, hub := range dbg.Hubs {
		for x := hub.X; x < hub.X+hub.W; x++ {
			if !sgProjectionWindowCleanForObject(grid, dbg.Tags, x, hub.Y, 1, projectionH, hub.ObjectID, splitY) {
				t.Fatalf("hub %d has foreign solid in projection window", hub.ObjectID)
			}
		}
	}
	for i, chain := range dbg.Chains {
		for _, step := range chain.Steps {
			if !sgProjectionWindowCleanForObject(grid, dbg.Tags, step.X, step.Y, stepW, projectionH, chain.ObjectID, splitY) {
				t.Fatalf("chain %d step at (%d,%d) has foreign solid in projection window", i, step.X, step.Y)
			}
		}
	}
}

func TestPopulateSkyAndObjects_StairChainsReachGroundOrSurface(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	dbg := sgPopulateSkyAndObjects(rand.New(rand.NewSource(303)), grid, cfg, splitY)
	if len(dbg.Chains) != len(dbg.Hubs)*2 {
		t.Fatalf("expected two chains per sky hub: chains=%d hubs=%d", len(dbg.Chains), len(dbg.Hubs))
	}

	stepW := max(2, sgScaleX(3, cfg))
	nearR := max(2, min(sgScaleX(6, cfg), sgScaleY(6, cfg)))
	stepDy := max(2, sgScaleY(6, cfg))
	minStepDx := max(stepW+1, sgScaleX(7, cfg))
	maxStepDx := max(minStepDx, sgScaleX(10, cfg))
	minGap := max(1, sgScaleX(4, cfg))
	for i, chain := range dbg.Chains {
		if chain.BaseSteps == 0 || len(chain.Steps) == 0 {
			t.Fatalf("chain %d has no base steps", i)
		}
		if !chain.Connected {
			t.Fatalf("chain %d is not marked connected", i)
		}
		if chain.HubIndex < 0 || chain.HubIndex >= len(dbg.Hubs) {
			t.Fatalf("chain %d has invalid hub index %d", i, chain.HubIndex)
		}
		for stepIdx := 1; stepIdx < len(chain.Steps); stepIdx++ {
			prev := chain.Steps[stepIdx-1]
			cur := chain.Steps[stepIdx]
			dy := cur.Y - prev.Y
			if stepIdx < chain.BaseSteps {
				if dy != stepDy {
					t.Fatalf("chain %d step %d has invalid dY: got %d want %d", i, stepIdx, dy, stepDy)
				}
			} else if testAbs(dy) > stepDy {
				t.Fatalf("chain %d extra step %d has invalid dY: got %d max %d", i, stepIdx, dy, stepDy)
			}
			dx := testAbs(cur.X - prev.X)
			if stepIdx < chain.BaseSteps {
				if dx < minStepDx || dx > maxStepDx {
					t.Fatalf("chain %d step %d has invalid |dX|: got %d want [%d,%d]", i, stepIdx, dx, minStepDx, maxStepDx)
				}
				if sgHorizontalGap(prev.X, cur.X, stepW) < minGap {
					t.Fatalf("chain %d step %d violates horizontal gap: got %d want >=%d", i, stepIdx, sgHorizontalGap(prev.X, cur.X, stepW), minGap)
				}
			}
		}

		last := chain.Steps[chain.BaseSteps-1]
		ignore := buildChainIgnoreMap(dbg.Hubs[chain.HubIndex], chain, stepW)
		reachedGround := last.Y >= splitY-1
		externalNear := sgHasExternalSolidNearby(grid, last.X, last.Y, stepW, 1, nearR, ignore)
		if !reachedGround && !externalNear {
			t.Fatalf("chain %d did not reach ground or external solid", i)
		}
	}
}

func TestPopulateSkyAndObjects_ExtraStepsConnectedAndCount(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	dbg := sgPopulateSkyAndObjects(rand.New(rand.NewSource(104)), grid, cfg, splitY)
	if dbg.ExtraPlaced != dbg.ExtraStepCount {
		t.Fatalf("extra step placement mismatch: placed=%d target=%d", dbg.ExtraPlaced, dbg.ExtraStepCount)
	}

	totalExtra := 0
	maxJumpDx := max(1, sgScaleX(10, cfg))
	maxJumpDy := max(1, sgScaleY(6, cfg))
	for i, chain := range dbg.Chains {
		totalExtra += chain.ExtraSteps
		if len(chain.Steps) < chain.BaseSteps+chain.ExtraSteps {
			t.Fatalf("chain %d has inconsistent step counters", i)
		}
		for idx := chain.BaseSteps; idx < len(chain.Steps); idx++ {
			if idx == 0 {
				t.Fatalf("chain %d extra step cannot be first step", i)
			}
			prev := chain.Steps[idx-1]
			cur := chain.Steps[idx]
			dx := testAbs(cur.X - prev.X)
			dyLocal := testAbs(cur.Y - prev.Y)
			if dx > maxJumpDx {
				t.Fatalf("chain %d extra step %d too far in X: %d max=%d", i, idx, dx, maxJumpDx)
			}
			if dyLocal > maxJumpDy {
				t.Fatalf("chain %d extra step %d too far in Y: %d max=%d", i, idx, dyLocal, maxJumpDy)
			}
		}
	}
	if totalExtra != dbg.ExtraPlaced {
		t.Fatalf("total chain extras mismatch: got %d want %d", totalExtra, dbg.ExtraPlaced)
	}
}

func TestPopulateSkyAndObjects_GapFillBetweenWingEndpoints(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	hubs := []sgSkyHub{
		{X: 20, Y: 30, W: 18, H: 2},
		{X: 150, Y: 32, W: 18, H: 2},
	}
	chains := []sgStairChain{
		{HubIndex: 0, Side: sgStairSideLeft, TrendDir: -1, Steps: []sgPoint{{X: 30, Y: 58}}, BaseSteps: 1, Connected: true},
		{HubIndex: 0, Side: sgStairSideRight, TrendDir: 1, Steps: []sgPoint{{X: 60, Y: 60}}, BaseSteps: 1, Connected: true},
		{HubIndex: 1, Side: sgStairSideLeft, TrendDir: -1, Steps: []sgPoint{{X: 130, Y: 56}}, BaseSteps: 1, Connected: true},
		{HubIndex: 1, Side: sgStairSideRight, TrendDir: 1, Steps: []sgPoint{{X: 168, Y: 59}}, BaseSteps: 1, Connected: true},
	}
	refs := []sgSkyWingRefs{
		{LeftIdx: 0, RightIdx: 1},
		{LeftIdx: 2, RightIdx: 3},
	}

	placed := sgAddSkyGapPlatforms(grid, hubs, chains, refs, splitY, cfg)
	if placed != 1 {
		t.Fatalf("expected one gap platform, got %d", placed)
	}

	restW := max(3, sgScaleX(6, cfg))
	stepW := max(2, sgScaleX(3, cfg))
	leftEdge := chains[1].Steps[0].X + stepW - 1
	rightEdge := chains[2].Steps[0].X
	expectedY := (chains[1].Steps[0].Y + chains[2].Steps[0].Y) / 2
	found := false
	for y := expectedY - 4; y <= expectedY+4 && !found; y++ {
		if y < 0 || y >= len(grid) {
			continue
		}
		for x := leftEdge + 1; x <= rightEdge-restW; x++ {
			ok := true
			for dx := 0; dx < restW; dx++ {
				if grid[y][x+dx] != sgCellSolid {
					ok = false
					break
				}
			}
			if ok {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("gap-fill platform 6x1 (scaled) not found between wing endpoints")
	}
}

func TestGenerateSandwichLocation_HubsCountAndSpacing(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 220
	cfg.GridH = 160

	out := GenerateSandwichLocation(rand.New(rand.NewSource(7)), "room_01", cfg)
	if len(out.Hubs) < 5 || len(out.Hubs) > 7 {
		t.Fatalf("hub count out of range: got %d", len(out.Hubs))
	}

	minGap := sgMinHubGap(cfg)
	for i := 0; i < len(out.Hubs); i++ {
		for j := i + 1; j < len(out.Hubs); j++ {
			if sgRectIntersectsInflated(out.Hubs[i], out.Hubs[j], minGap) {
				t.Fatalf("hubs %d and %d overlap/too close", i, j)
			}
		}
	}
}

func TestGenerateSandwichLocation_ConnectivityFromFirstInlet(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150

	out := GenerateSandwichLocation(rand.New(rand.NewSource(11)), "room_01", cfg)
	if len(out.Inlets) == 0 || len(out.Hubs) == 0 {
		t.Fatalf("expected inlets and hubs to exist")
	}

	start, ok := findPassableNear(out.Grid, sgPoint{X: out.Inlets[0].CenterX, Y: out.Inlets[0].Y}, 6)
	if !ok {
		t.Fatalf("could not find passable start near first inlet")
	}
	seen := bfsPassable(out.Grid, start)

	for i, inlet := range out.Inlets {
		p, ok := findPassableNear(out.Grid, sgPoint{X: inlet.CenterX, Y: inlet.Y}, 8)
		if !ok {
			t.Fatalf("could not find passable point for inlet %d", i)
		}
		if !seen[[2]int{p.X, p.Y}] {
			t.Fatalf("inlet %d is not connected to first inlet", i)
		}
	}
	for i, hub := range out.Hubs {
		center := hub.Center()
		p, ok := findPassableNear(out.Grid, center, 8)
		if !ok {
			t.Fatalf("could not find passable point near hub %d", i)
		}
		if !seen[[2]int{p.X, p.Y}] {
			t.Fatalf("hub %d is not connected to inlet graph", i)
		}
	}
}

func TestGenerateSandwichLocation_AllPassableCellsConnected(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150

	out := GenerateSandwichLocation(rand.New(rand.NewSource(111)), "room_01", cfg)
	start, ok := findPassableNear(out.Grid, sgPoint{X: cfg.GridW / 2, Y: cfg.GridH / 2}, max(cfg.GridW, cfg.GridH))
	if !ok {
		t.Fatalf("no passable start cell found")
	}
	seen := bfsPassable(out.Grid, start)

	for y := range out.Grid {
		for x := range out.Grid[y] {
			if !sgIsPassable(out.Grid[y][x]) {
				continue
			}
			if !seen[[2]int{x, y}] {
				t.Fatalf("passable cell (%d,%d) is disconnected from global passable graph", x, y)
			}
		}
	}
}

func TestSkyStrictAntiStacking_RemovesForeignOverlaps(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)
	tags := sgNewSkyTagGrid(cfg.GridW, cfg.GridH)

	_ = sgPopulateSkyAndObjectsWithTags(rand.New(rand.NewSource(222)), grid, cfg, splitY, tags)
	sgApplySkyStrictAntiStacking(grid, tags, splitY, cfg)

	checkH := max(2, sgScaleY(10, cfg))
	for y := 1; y < splitY; y++ {
		for x := 0; x < cfg.GridW; x++ {
			if grid[y][x] != sgCellSolid {
				continue
			}
			baseTag := tags.At(x, y)
			for yy := max(0, y-checkH); yy < y; yy++ {
				if grid[yy][x] != sgCellSolid {
					continue
				}
				otherTag := tags.At(x, yy)
				if baseTag.ObjectID > 0 && otherTag.ObjectID == baseTag.ObjectID {
					continue
				}
				t.Fatalf("foreign sky overlap remains at x=%d y=%d above y=%d", x, yy, y)
			}
		}
	}
}

func TestSandwichInletShaftsHaveZigZagPlatforms(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150

	splitY := cfg.GridH / 2
	step := max(3, sgScaleY(6, cfg))
	sideGap := max(1, sgScaleX(3, cfg))
	platformW := max(2, sgScaleX(3, cfg))
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)
	inlet := sgBuildInlets(cfg.GridW, splitY)[0]
	shaft := sgCarveInletShaft(grid, inlet, splitY+40)
	tags := sgNewSkyTagGrid(cfg.GridW, cfg.GridH)
	sgAddInletZigZagPlatforms(grid, shaft, cfg, tags)

	rows := detectShaftPlatformRows(grid, shaft.Left, shaft.Right, shaft.TopY+1, shaft.BottomY, platformW)
	if len(rows) < 2 {
		t.Fatalf("shaft has too few zigzag platforms: %d", len(rows))
	}

	filtered := make([]shaftPlatformRow, 0, len(rows))
	for _, row := range rows {
		if len(filtered) == 0 || row.y-filtered[len(filtered)-1].y >= max(2, step-1) {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) < 2 {
		t.Fatalf("shaft does not have spaced platforms")
	}

	for j := 1; j < len(filtered); j++ {
		if filtered[j].side == filtered[j-1].side {
			t.Fatalf("platforms must strictly alternate sides")
		}
		if filtered[j].y-filtered[j-1].y < max(2, step-1) {
			t.Fatalf("platforms must keep vertical spacing ~%d", step)
		}
		startGap := testAbs(filtered[j].start - filtered[j-1].start)
		if startGap < sideGap {
			t.Fatalf("platforms must keep side gap >=%d, got %d", sideGap, startGap)
		}
	}
}

func TestSandwichGroundAirCorridorClearsOutsideWhitelist(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)
	inlets := sgBuildInlets(cfg.GridW, splitY)
	stepW := max(2, sgScaleX(3, cfg))
	corridorH := max(2, sgScaleY(10, cfg))

	for y := max(0, splitY-corridorH); y < splitY; y++ {
		for x := 0; x < cfg.GridW; x++ {
			grid[y][x] = sgCellSolid
		}
	}
	chains := []sgStairChain{
		{BaseSteps: 1, Steps: []sgPoint{{X: cfg.GridW / 2, Y: splitY - 3}}},
	}
	sgApplyGroundAirCorridor(grid, splitY, cfg, inlets, chains)

	preserve := make([]bool, cfg.GridW)
	for _, inlet := range inlets {
		left := sgClamp(inlet.CenterX-inlet.Width/2-1, 0, cfg.GridW-1)
		right := sgClamp(left+inlet.Width+2, 0, cfg.GridW-1)
		for x := left; x <= right; x++ {
			preserve[x] = true
		}
	}
	last := chains[0].Steps[0]
	for x := max(0, last.X-1); x <= min(cfg.GridW-1, last.X+stepW); x++ {
		preserve[x] = true
	}

	for y := max(0, splitY-corridorH); y < splitY; y++ {
		for x := 0; x < cfg.GridW; x++ {
			if preserve[x] {
				continue
			}
			if grid[y][x] == sgCellSolid {
				t.Fatalf("ground corridor not cleaned at (%d,%d)", x, y)
			}
		}
	}
}

func TestSandwichTunnelHeadroomPassMaintainsMinimumClearance(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2
	grid := makeBaseSandwichGrid(cfg.GridW, cfg.GridH, splitY)

	// Create intentionally low tunnel: floor at splitY+7 with only ~3 tiles headroom.
	sgCarveRect(grid, 40, splitY+4, 20, 3)
	sgApplyTunnelHeadroom(grid, splitY, cfg)

	minHeadroom := max(2, sgScaleY(7, cfg))
	floorY := splitY + 7
	for x := 40; x < 60; x++ {
		if grid[floorY][x] != sgCellSolid || !sgIsPassable(grid[floorY-1][x]) {
			continue
		}
		clear := 0
		for yy := floorY - 1; yy >= max(0, splitY-2); yy-- {
			if !sgIsPassable(grid[yy][x]) {
				break
			}
			clear++
		}
		if clear < minHeadroom {
			t.Fatalf("insufficient tunnel headroom at x=%d: got %d want >=%d", x, clear, minHeadroom)
		}
	}

	for y := 0; y < max(0, splitY-2); y++ {
		for x := 0; x < cfg.GridW; x++ {
			if grid[y][x] == sgCellBackwall {
				t.Fatalf("headroom pass modified cell above splitY-2 at (%d,%d)", x, y)
			}
		}
	}
}

func TestSandwichReturnLoopsReachSurfaceFromDeepHubs(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150

	out := GenerateSandwichLocation(rand.New(rand.NewSource(29)), "room_01", cfg)
	splitY := cfg.GridH / 2

	extra := surfaceSegmentsOutsideInlets(out.Grid, splitY, out.Inlets)
	if len(extra) < 2 {
		t.Fatalf("expected at least 2 additional surface exits, got %d", len(extra))
	}

	deep := sgDeepestHubIndices(out.Hubs, 2)
	reachable := map[[2]int]bool{}
	for _, idx := range deep {
		start, ok := findPassableNear(out.Grid, out.Hubs[idx].Center(), 8)
		if !ok {
			t.Fatalf("could not find passable start near deep hub %d", idx)
		}
		seen := bfsPassable(out.Grid, start)
		for key := range seen {
			reachable[key] = true
		}
	}

	reachableSegments := 0
	for _, seg := range extra {
		segReachable := false
		for x := seg.Left; x <= seg.Right; x++ {
			if reachable[[2]int{x, splitY}] {
				segReachable = true
				break
			}
		}
		if segReachable {
			reachableSegments++
		}
	}
	if reachableSegments < 2 {
		t.Fatalf("expected 2 reachable return exits, got %d", reachableSegments)
	}
}

func TestSandwichWallLedgesInTallVerticalSpan(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2

	grid := make([][]int, cfg.GridH)
	for y := 0; y < cfg.GridH; y++ {
		grid[y] = make([]int, cfg.GridW)
		for x := 0; x < cfg.GridW; x++ {
			if y < splitY {
				grid[y][x] = sgCellAir
			} else {
				grid[y][x] = sgCellSolid
			}
		}
	}

	shaftX := 64
	shaftW := 8
	sgCarveRect(grid, shaftX, splitY+2, shaftW, 46)
	sgAddWallLedges(grid, splitY, cfg)

	ledgeRows := 0
	for y := splitY + 4; y < splitY+44; y++ {
		leftPair := grid[y][shaftX] == sgCellSolid && grid[y][shaftX+1] == sgCellSolid
		rightPair := grid[y][shaftX+shaftW-2] == sgCellSolid && grid[y][shaftX+shaftW-1] == sgCellSolid
		if leftPair || rightPair {
			ledgeRows++
		}
	}
	if ledgeRows == 0 {
		t.Fatalf("expected wall ledges in tall vertical span")
	}
}

func TestSandwichCarveRadiusProducesComfortWidth(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	r := sgScaledRadius(cfg)

	gridW, gridH := 80, 50
	grid := make([][]int, gridH)
	for y := 0; y < gridH; y++ {
		grid[y] = make([]int, gridW)
		for x := 0; x < gridW; x++ {
			grid[y][x] = sgCellSolid
		}
	}

	sgCarveLine(grid, sgPoint{X: 10, Y: 22}, sgPoint{X: 70, Y: 22}, r)

	passableVertical := 0
	for y := 0; y < gridH; y++ {
		if sgIsPassable(grid[y][40]) {
			passableVertical++
		}
	}
	minComfort := 2*r + 1
	if passableVertical < minComfort {
		t.Fatalf("carved corridor too narrow: got %d want at least %d", passableVertical, minComfort)
	}
}

func TestSandwichInternalLedgesAddedForTallSpaces(t *testing.T) {
	cfg := DefaultProcGenConfig
	cfg.GridW = 200
	cfg.GridH = 150
	splitY := cfg.GridH / 2

	grid := make([][]int, cfg.GridH)
	for y := 0; y < cfg.GridH; y++ {
		grid[y] = make([]int, cfg.GridW)
		for x := 0; x < cfg.GridW; x++ {
			if y < splitY {
				grid[y][x] = sgCellAir
			} else {
				grid[y][x] = sgCellSolid
			}
		}
	}

	// One very tall carved chamber guarantees ledge placement.
	sgCarveRect(grid, 50, splitY+5, 80, 55)
	sgAddInternalLedges(rand.New(rand.NewSource(3)), grid, splitY, cfg)

	ledgeCells := 0
	for y := splitY + 1; y < cfg.GridH-1; y++ {
		for x := 1; x < cfg.GridW-1; x++ {
			if grid[y][x] != sgCellSolid {
				continue
			}
			if sgIsPassable(grid[y-1][x]) && sgIsPassable(grid[y+1][x]) {
				ledgeCells++
			}
		}
	}
	if ledgeCells == 0 {
		t.Fatalf("expected internal ledges to be added in tall cavity")
	}
}

func TestGenerateRects_CoverageAndNoOverlap(t *testing.T) {
	grid := [][]int{
		{1, 1, 0, 0, 1, 1, 1, 0},
		{1, 1, 0, 0, 1, 1, 1, 0},
		{0, 0, 0, 0, 1, 1, 1, 0},
		{0, 1, 1, 0, 0, 0, 0, 0},
		{0, 1, 1, 0, 0, 0, 0, 0},
	}
	rects := GenerateRects(grid, 1)

	covered := make(map[[2]int]bool)
	for _, r := range rects {
		x0 := int(math.Round(r.X))
		y0 := int(math.Round(r.Y))
		w := int(math.Round(r.W))
		h := int(math.Round(r.H))
		for y := y0; y < y0+h; y++ {
			for x := x0; x < x0+w; x++ {
				key := [2]int{x, y}
				if covered[key] {
					t.Fatalf("rectangles overlap at (%d,%d)", x, y)
				}
				covered[key] = true
			}
		}
	}

	for y := range grid {
		for x := range grid[y] {
			key := [2]int{x, y}
			if grid[y][x] == 1 && !covered[key] {
				t.Fatalf("target cell (%d,%d) was not covered", x, y)
			}
			if grid[y][x] != 1 && covered[key] {
				t.Fatalf("non-target cell (%d,%d) was covered", x, y)
			}
		}
	}
}

func TestGenerateRaidProcGenWith_UsesSandwichDefaults(t *testing.T) {
	bundle := &Bundle{}
	cfg := DefaultProcGenConfig

	raid, err := bundle.GenerateRaidProcGenWith(12345, cfg)
	if err != nil {
		t.Fatalf("GenerateRaidProcGenWith failed: %v", err)
	}
	if len(raid.Layout.Rooms) != cfg.NumRooms {
		t.Fatalf("unexpected room count: got %d want %d", len(raid.Layout.Rooms), cfg.NumRooms)
	}
	if len(raid.Layout.Rooms) == 0 {
		t.Fatal("no rooms generated")
	}

	first := raid.Layout.Rooms[0]
	expectedW := float64(cfg.GridW * sgBlockSizePx)
	expectedH := float64(cfg.GridH * sgBlockSizePx)
	if first.Bounds.W != expectedW || first.Bounds.H != expectedH {
		t.Fatalf("room bounds mismatch: got %.0fx%.0f want %.0fx%.0f", first.Bounds.W, first.Bounds.H, expectedW, expectedH)
	}
	if len(first.Solids) == 0 {
		t.Fatalf("first room has no solids")
	}

	roomsWithBackwalls := 0
	for _, room := range raid.Layout.Rooms {
		if len(room.Backwalls) > 0 {
			roomsWithBackwalls++
		}
	}
	if roomsWithBackwalls == 0 {
		t.Fatalf("expected at least one room with backwalls in runtime layout")
	}
	if len(first.Backwalls) == 0 {
		t.Fatalf("expected first room to include backwalls")
	}

	if len(raid.PlayerSpawns) < 2 {
		t.Fatalf("expected multiple spawn points, got %d", len(raid.PlayerSpawns))
	}
	expectedSpawnX := float64(int(math.Round(float64(cfg.GridW)*0.2)) * sgBlockSizePx)
	if math.Abs(raid.PlayerSpawn.X-expectedSpawnX) > float64(6*sgBlockSizePx) {
		t.Fatalf("primary spawn X too far from first inlet anchor: got %.1f want around %.1f", raid.PlayerSpawn.X, expectedSpawnX)
	}
}

type shaftPlatformRow struct {
	y     int
	side  int // -1 left, +1 right
	start int
	end   int
}

func makeBaseSandwichGrid(w, h, splitY int) [][]int {
	grid := make([][]int, h)
	for y := 0; y < h; y++ {
		grid[y] = make([]int, w)
		for x := 0; x < w; x++ {
			if y < splitY {
				grid[y][x] = sgCellAir
			} else {
				grid[y][x] = sgCellSolid
			}
		}
	}
	return grid
}

func buildChainIgnoreMap(hub sgSkyHub, chain sgStairChain, stepW int) map[[2]int]bool {
	ignore := make(map[[2]int]bool, hub.W*hub.H+len(chain.Steps)*stepW)
	for y := hub.Y; y < hub.Y+hub.H; y++ {
		for x := hub.X; x < hub.X+hub.W; x++ {
			ignore[[2]int{x, y}] = true
		}
	}
	for _, step := range chain.Steps {
		for x := step.X; x < step.X+stepW; x++ {
			ignore[[2]int{x, step.Y}] = true
		}
	}
	return ignore
}

func sgProjectionWindowCleanForObject(grid [][]int, tags *sgSkyTagGrid, x, y, width, projectionH, objectID, splitY int) bool {
	if tags == nil || objectID <= 0 {
		return false
	}
	if y <= 0 || y >= len(grid) {
		return true
	}
	x0 := max(0, x-1)
	x1 := min(len(grid[0])-1, x+width)
	y0 := max(0, y-projectionH)
	for yy := y0; yy < y; yy++ {
		if yy >= splitY {
			continue
		}
		for xx := x0; xx <= x1; xx++ {
			if grid[yy][xx] != sgCellSolid {
				continue
			}
			tag := tags.At(xx, yy)
			if tag.ObjectID != 0 && tag.ObjectID != objectID {
				return false
			}
		}
	}
	return true
}

func detectShaftPlatformRows(grid [][]int, left, right, yMin, yMax, minWidth int) []shaftPlatformRow {
	if len(grid) == 0 {
		return nil
	}
	yMin = max(0, yMin)
	yMax = min(len(grid)-1, yMax)
	rows := make([]shaftPlatformRow, 0, 8)
	for y := yMin; y <= yMax; y++ {
		x := left
		for x <= right {
			for x <= right && grid[y][x] != sgCellSolid {
				x++
			}
			if x > right {
				break
			}
			start := x
			for x <= right && grid[y][x] == sgCellSolid {
				x++
			}
			end := x - 1
			if end-start+1 < minWidth {
				continue
			}

			side := 0
			if start <= left+1 {
				side = -1
			}
			if end >= right-1 {
				if side == 0 {
					side = 1
				} else {
					leftGap := start - left
					rightGap := right - end
					if rightGap < leftGap {
						side = 1
					}
				}
			}
			if side == 0 {
				continue
			}
			rows = append(rows, shaftPlatformRow{y: y, side: side, start: start, end: end})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].y < rows[j].y })
	return rows
}

func surfaceSegmentsOutsideInlets(grid [][]int, y int, inlets []sgInlet) []sgRowSegment {
	if y < 0 || y >= len(grid) || len(grid[y]) == 0 {
		return nil
	}
	w := len(grid[y])
	blocked := make([]bool, w)
	for _, inlet := range inlets {
		left := max(0, inlet.CenterX-inlet.Width/2-1)
		right := min(w-1, left+inlet.Width+1)
		for x := left; x <= right; x++ {
			blocked[x] = true
		}
	}

	segments := make([]sgRowSegment, 0, 4)
	x := 0
	for x < w {
		for x < w && (blocked[x] || !sgIsPassable(grid[y][x])) {
			x++
		}
		if x >= w {
			break
		}
		left := x
		for x < w && !blocked[x] && sgIsPassable(grid[y][x]) {
			x++
		}
		right := x - 1
		if right-left+1 >= 2 {
			segments = append(segments, sgRowSegment{Left: left, Right: right})
		}
	}
	return segments
}

func bfsPassable(grid [][]int, start sgPoint) map[[2]int]bool {
	seen := map[[2]int]bool{}
	queue := []sgPoint{start}
	seen[[2]int{start.X, start.Y}] = true
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
		for _, d := range dirs {
			nx := p.X + d[0]
			ny := p.Y + d[1]
			if ny < 0 || ny >= len(grid) || nx < 0 || nx >= len(grid[ny]) {
				continue
			}
			if !sgIsPassable(grid[ny][nx]) {
				continue
			}
			key := [2]int{nx, ny}
			if seen[key] {
				continue
			}
			seen[key] = true
			queue = append(queue, sgPoint{X: nx, Y: ny})
		}
	}
	return seen
}

func findPassableNear(grid [][]int, p sgPoint, radius int) (sgPoint, bool) {
	for r := 0; r <= radius; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				x := p.X + dx
				y := p.Y + dy
				if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
					continue
				}
				if sgIsPassable(grid[y][x]) {
					return sgPoint{X: x, Y: y}, true
				}
			}
		}
	}
	return sgPoint{}, false
}

func testAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
