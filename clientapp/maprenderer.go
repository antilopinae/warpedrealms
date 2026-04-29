// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package clientapp

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"warpedrealms/shared"
)

type roomPreview struct {
	room   shared.RoomState
	rect   shared.Rect
	alpha  float64
	target shared.Vec2
}

type RaidRenderer struct {
	assets           *Assets
	whitePixelImg    *ebiten.Image
	useSpriteRectOps bool
	rectOp           ebiten.DrawImageOptions
}

func NewRaidRenderer(assets *Assets, whitePixelImg *ebiten.Image) *RaidRenderer {
	return &RaidRenderer{
		assets:           assets,
		whitePixelImg:    whitePixelImg,
		useSpriteRectOps: true,
	}
}

func (r *RaidRenderer) SetUseSpriteRectOps(enabled bool) { r.useSpriteRectOps = enabled }

func (r *RaidRenderer) drawRect(screen *ebiten.Image, x, y, w, h float32, clr color.Color) {
	if !r.useSpriteRectOps || r.whitePixelImg == nil {
		vector.DrawFilledRect(screen, x, y, w, h, clr, false)
		return
	}
	r.rectOp.GeoM.Reset()
	r.rectOp.GeoM.Scale(float64(w), float64(h))
	r.rectOp.GeoM.Translate(float64(x), float64(y))
	r.rectOp.ColorScale.Reset()
	r.rectOp.ColorScale.ScaleWithColor(clr)
	screen.DrawImage(r.whitePixelImg, &r.rectOp)
}

// DrawScene renders the active room and its background (if any).
// revealBgRoomID, when non-empty, overrides BelowRoomID as the background room
// to show — used when the local player is inside a RevealZone.
func (r *RaidRenderer) DrawScene(screen *ebiten.Image, layout shared.RaidLayoutState, activeRoomID string, camera shared.Vec2, offset shared.Vec2, scale float64, preview *roomPreview, revealBgRoomID string) {
	activeRoom, ok := layout.RoomByID(activeRoomID)
	if !ok && len(layout.Rooms) > 0 {
		activeRoom = layout.Rooms[0]
		ok = true
	}
	if !ok {
		return
	}

	// 1. Deep cave background (texture, fills screen with parallax).
	r.drawBackgroundForRoom(screen, activeRoom, camera, offset, scale, 1)

	// 2. Background room — the other sublocation visible "through the wall".
	//    Priority: RevealZone override > BelowRoomID chain.
	//    Rendered at bgDepthScale so it looks clearly distant.
	bgRoomID := revealBgRoomID
	if bgRoomID == "" {
		bgRoomID = activeRoom.BelowRoomID
	}
	if bgRoomID != "" {
		if bgRoom, ok := layout.RoomByID(bgRoomID); ok {
			bgSc, bgCam := bgRoomTransform(camera, scale, offset)
			r.drawRoomAsBackground(screen, bgRoom, bgCam, offset, bgSc)
			r.drawBgRoomObjects(screen, bgRoom, bgCam, offset, bgSc)
			r.drawBgDepthHaze(screen, offset)
		}
	}

	// 3. Portal preview (optional pop-up).
	if preview != nil && preview.alpha > 0 {
		r.drawPreviewWindow(screen, *preview, camera, offset, scale)
	}

	// 4. Active room geometry — on top of everything.
	r.drawRoomGeometry(screen, activeRoom, camera, offset, scale, 1)
	r.drawDecorations(screen, activeRoom, camera, offset, scale, 1)
}

// bgDepthScale controls how much smaller the background room appears relative
// to the foreground.  0.4 means it renders at 40 % of the normal world scale,
// so platforms look clearly distant and more of the level is visible at once.
const bgDepthScale = 0.4

// bgRoomTransform returns the (scale, camera) pair for the background-room
// render.  The camera is chosen so that the same world point sits at the
// screen centre in both the foreground and the background renders — the only
// difference is scale, which creates the "far away" depth illusion.
func bgRoomTransform(fgCam shared.Vec2, fgScale float64, offset shared.Vec2) (float64, shared.Vec2) {
	bgSc := fgScale * bgDepthScale
	// World centre visible in the foreground viewport.
	cx := float64(shared.ScreenWidth-int(offset.X)) * 0.5
	cy := float64(shared.ScreenHeight-int(offset.Y)) * 0.5
	bgCam := shared.Vec2{
		X: fgCam.X + cx*(1/fgScale-1/bgSc),
		Y: fgCam.Y + cy*(1/fgScale-1/bgSc),
	}
	return bgSc, bgCam
}

// drawRoomAsBackground renders the background room as glowing platform-surface
// lines rather than solid filled blocks.  Individual 16×16 cells get a subtle
// body fill and a bright top-rim.  Wide boundary rects (floor / walls added by
// the LDtk loader) are drawn as a dark ground-mass so the bottom of the room
// is visible and does not clip into the void.
func (r *RaidRenderer) drawRoomAsBackground(screen *ebiten.Image, room shared.RoomState, camera shared.Vec2, offset shared.Vec2, scale float64) {
	const gs = 16.0 // grid cell size in world pixels

	// Build a lookup of occupied cells (for top-rim detection).
	type cellKey [2]int
	occupied := make(map[cellKey]bool, len(room.Solids))
	for _, s := range room.Solids {
		if s.W == gs && s.H == gs {
			occupied[cellKey{int(math.Round(s.X / gs)), int(math.Round(s.Y / gs))}] = true
		}
	}

	_, rimClr := platformColors(room.Biome, 0.90)
	fillClr, _ := platformColors(room.Biome, 0.12)
	rimH := float32(math.Max(3, scale*gs*0.30)) // bright top-rim height in screen px

	// Boundary fill colour: dark near-black mass for floor / walls.
	fillBig, _ := platformColors(room.Biome, 0.28)
	rimBig := rimClr
	rimBig.A = uint8(float64(rimBig.A) * 0.7)

	for _, s := range room.Solids {
		sx := float32(offset.X + (s.X-camera.X)*scale)
		sy := float32(offset.Y + (s.Y-camera.Y)*scale)
		sw := float32(s.W * scale)
		sh := float32(s.H * scale)

		if s.W == gs && s.H == gs {
			// Individual platform cell — subtle fill + bright top rim.
			col := int(math.Round(s.X / gs))
			row := int(math.Round(s.Y / gs))
			r.drawRect(screen, sx, sy, sw, sh, fillClr)
			if !occupied[cellKey{col, row - 1}] {
				r.drawRect(screen, sx, sy, sw, rimH, rimClr)
			}
		} else if s.W > gs*4 && s.H >= gs {
			// Wide boundary rect (floor or wide ledge) — draw as ground mass.
			r.drawRect(screen, sx, sy, sw, sh, fillBig)
			// Bright top edge so the floor reads as a surface.
			r.drawRect(screen, sx, sy, sw, float32(math.Max(2, float64(rimH)*0.8)), rimBig)
		}
		// Narrow vertical walls (left/right boundary) — deliberately skipped
		// to avoid blocking the view of the background room.
	}

}

// drawBgDepthHaze overlays a dark atmospheric veil over the background room,
// creating the illusion that it is far away.  Must be called after both
// drawRoomAsBackground and drawBgRoomObjects so everything is dimmed uniformly.
func (r *RaidRenderer) drawBgDepthHaze(screen *ebiten.Image, offset shared.Vec2) {
	sb := screen.Bounds()
	r.drawRect(screen,
		float32(offset.X), float32(offset.Y),
		float32(float64(sb.Dx())-offset.X), float32(float64(sb.Dy())-offset.Y),
		color.RGBA{6, 10, 20, 110})
}

// drawBgRoomObjects renders all non-solid game objects of a background room:
// tile layers, one-way platforms, boss spawn markers, portal zones, jump-link
// portals, active rifts, PvP zones and decorations.  Everything is drawn at
// bgAlpha (≈ 50 %) so it looks clearly distant but still readable.
// Must be called BEFORE drawBgDepthHaze so the haze dims it uniformly.
func (r *RaidRenderer) drawBgRoomObjects(screen *ebiten.Image, room shared.RoomState, camera shared.Vec2, offset shared.Vec2, scale float64) {
	bgAlpha := 0.50

	// ── Tile layers ───────────────────────────────────────────────────────────
	r.drawTileLayers(screen, room, camera, offset, scale, bgAlpha)

	// ── One-way platforms ─────────────────────────────────────────────────────
	platFill, platRim := platformOneWayColors(bgAlpha)
	for _, p := range room.Platforms {
		screenX := offset.X + (p.X-camera.X)*scale
		screenY := offset.Y + (p.Y-camera.Y)*scale
		sw := float32(p.W * scale)
		sh := float32(p.H * scale)
		r.drawRect(screen, float32(screenX), float32(screenY), sw, sh, platFill)
		rimH := float32(math.Max(2, scale*2.5))
		r.drawRect(screen, float32(screenX), float32(screenY), sw, rimH, platRim)
	}

	// ── Boss spawn markers ────────────────────────────────────────────────────
	blockPx := 16.0 * scale
	for _, bs := range room.BossSpawns {
		fill, rim := bossSpawnColors(bs.Level, bgAlpha)
		screenX := float32(offset.X + (bs.X-camera.X)*scale)
		screenY := float32(offset.Y + (bs.Y-camera.Y)*scale)
		bw := float32(blockPx)
		bh := float32(blockPx)
		lineW := float32(math.Max(1, scale*1.5))
		cx32 := screenX + bw*0.5
		cy32 := screenY + bh*0.5
		if bs.Flying {
			half := bw * 0.5
			vector.StrokeLine(screen, cx32, cy32-half, cx32+half, cy32, lineW, rim, false)
			vector.StrokeLine(screen, cx32+half, cy32, cx32, cy32+half, lineW, rim, false)
			vector.StrokeLine(screen, cx32, cy32+half, cx32-half, cy32, lineW, rim, false)
			vector.StrokeLine(screen, cx32-half, cy32, cx32, cy32-half, lineW, rim, false)
		} else {
			r.drawRect(screen, screenX, screenY, bw, bh, fill)
			vector.StrokeRect(screen, screenX, screenY, bw, bh, lineW, rim, false)
		}
	}

	// ── Portals (jump-links, cyan outline + portal FX) ───────────────────────
	style, styleOK := r.assets.TileStyles[room.TileStyleID]
	for _, link := range room.JumpLinks {
		topLeft := shared.Vec2{X: offset.X + (link.Area.X-camera.X)*scale, Y: offset.Y + (link.Area.Y-camera.Y)*scale}
		vector.StrokeRect(screen, float32(topLeft.X), float32(topLeft.Y), float32(link.Area.W*scale), float32(link.Area.H*scale), float32(math.Max(1.5, scale*1.5)), color.RGBA{110, 220, 255, uint8(180 * bgAlpha)}, false)
		linkCenter := shared.Vec2{
			X: topLeft.X + link.Area.W*scale*0.5,
			Y: topLeft.Y + link.Area.H*scale*0.5,
		}
		r.drawEffect(screen, r.assets.FX.Portal, linkCenter.X-link.Area.W*scale*0.18, linkCenter.Y-link.Area.H*scale*0.08, scale*0.7, bgAlpha*0.9)
		r.drawEffect(screen, r.assets.FX.PortalGlow, linkCenter.X-link.Area.W*scale*0.1, linkCenter.Y-link.Area.H*scale*0.16, scale*0.64, bgAlpha*0.8)
		if styleOK && style.JumpPreview != nil {
			r.drawImageAlpha(screen, style.JumpPreview, topLeft.X+link.Area.W*scale*0.18, topLeft.Y+link.Area.H*scale*0.15, scale*0.9, scale*0.9, bgAlpha)
		}
	}

	// ── Active rifts ──────────────────────────────────────────────────────────
	for _, rift := range room.Rifts {
		if !rift.IsOpen() {
			continue
		}
		clr := riftColor(rift.Kind, bgAlpha)
		sx := float32(offset.X + (rift.Area.X-camera.X)*scale)
		sy := float32(offset.Y + (rift.Area.Y-camera.Y)*scale)
		sw, sh := float32(rift.Area.W*scale), float32(rift.Area.H*scale)
		r.drawRect(screen, sx, sy, sw, sh, color.RGBA{clr.R, clr.G, clr.B, uint8(float64(clr.A) * 0.18)})
		lineW := float32(math.Max(1.5, scale))
		vector.StrokeRect(screen, sx, sy, sw, sh, lineW, clr, false)
		mid := sx + sw*0.5
		vector.StrokeLine(screen, mid-sw*0.15, sy+2, mid-sw*0.15, sy+sh-2, 1, clr, false)
		vector.StrokeLine(screen, mid+sw*0.15, sy+2, mid+sw*0.15, sy+sh-2, 1, clr, false)
		anchorW := float32(math.Max(2, scale*2))
		vector.StrokeLine(screen, sx-anchorW, sy+sh, sx+sw+anchorW, sy+sh, anchorW, clr, false)
	}

	// ── PvP zones ─────────────────────────────────────────────────────────────
	for _, zone := range room.PvPZones {
		tl := shared.Vec2{X: offset.X + (zone.X-camera.X)*scale, Y: offset.Y + (zone.Y-camera.Y)*scale}
		vector.StrokeRect(screen, float32(tl.X), float32(tl.Y), float32(zone.W*scale), float32(zone.H*scale), float32(math.Max(1, scale)), color.RGBA{245, 86, 86, uint8(160 * bgAlpha)}, false)
	}

	// ── Decorations + exits ───────────────────────────────────────────────────
	r.drawDecorations(screen, room, camera, offset, scale, bgAlpha)
}

func (r *RaidRenderer) drawPreviewWindow(screen *ebiten.Image, preview roomPreview, camera shared.Vec2, offset shared.Vec2, scale float64) {
	rect := preview.rect
	windowWidth := int(math.Max(64, rect.W*scale))
	windowHeight := int(math.Max(64, rect.H*scale))
	canvas := ebiten.NewImage(windowWidth, windowHeight)
	canvas.Fill(color.RGBA{9, 16, 22, 220})

	previewOffset := shared.Vec2{
		X: float64(windowWidth) * 0.1,
		Y: float64(windowHeight) * 0.12,
	}
	previewScale := math.Min(
		float64(windowWidth)/(preview.room.Bounds.W*0.36),
		float64(windowHeight)/(preview.room.Bounds.H*0.34),
	)
	previewCamera := shared.Vec2{
		X: preview.target.X - float64(windowWidth)/previewScale*0.45,
		Y: preview.target.Y - float64(windowHeight)/previewScale*0.48,
	}
	r.drawBackgroundForRoom(canvas, preview.room, previewCamera, previewOffset, previewScale, 0.92*preview.alpha)
	r.drawRoomGeometry(canvas, preview.room, previewCamera, previewOffset, previewScale, 0.78*preview.alpha)
	r.drawDecorations(canvas, preview.room, previewCamera, previewOffset, previewScale, 0.72*preview.alpha)

	topLeft := shared.Vec2{X: offset.X + (rect.X-camera.X)*scale, Y: offset.Y + (rect.Y-camera.Y)*scale}
	r.drawRect(screen, float32(topLeft.X+12), float32(topLeft.Y+14), float32(rect.W*scale), float32(rect.H*scale), color.RGBA{0, 0, 0, uint8(76 * preview.alpha)})
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(topLeft.X, topLeft.Y)
	op.ColorScale.Scale(1, 1, 1, float32(preview.alpha))
	screen.DrawImage(canvas, op)

	glow := color.RGBA{124, 225, 255, uint8(180 * preview.alpha)}
	vector.StrokeRect(screen, float32(topLeft.X), float32(topLeft.Y), float32(rect.W*scale), float32(rect.H*scale), 3, glow, false)
	vector.StrokeRect(screen, float32(topLeft.X-5), float32(topLeft.Y-5), float32(rect.W*scale+10), float32(rect.H*scale+10), 1, color.RGBA{210, 245, 255, uint8(90 * preview.alpha)}, false)
	r.drawRect(screen, float32(topLeft.X), float32(topLeft.Y-24), float32(math.Min(rect.W*scale, 220)), 22, color.RGBA{18, 28, 38, uint8(220 * preview.alpha)})
}

func (r *RaidRenderer) drawBackgroundForRoom(screen *ebiten.Image, room shared.RoomState, camera shared.Vec2, offset shared.Vec2, scale float64, alpha float64) {
	// Determine the visible drawing area from the screen bounds and offset.
	sb := screen.Bounds()
	drawW := float64(sb.Dx()) - offset.X
	drawH := float64(sb.Dy()) - offset.Y
	if drawW <= 0 || drawH <= 0 {
		return
	}

	background, ok := r.assets.Backgrounds[room.BackgroundID]
	if !ok {
		fill := color.RGBA{14, 20, 30, uint8(255 * alpha)}
		r.drawRect(screen,
			float32(offset.X), float32(offset.Y),
			float32(drawW), float32(drawH),
			fill,
		)
		return
	}

	// Camera position relative to the room origin — used to compute parallax
	// offsets so the background scrolls gently as the player moves.
	relCamX := camera.X - room.Bounds.X
	relCamY := camera.Y - room.Bounds.Y

	if background.Base != nil {
		r.drawTiledBg(screen, background.Base,
			offset.X, offset.Y, drawW, drawH,
			relCamX, relCamY, 0.05, alpha)
	}
	for _, layer := range background.Layers {
		parallax := layer.Parallax
		if parallax <= 0 {
			parallax = 0.08
		}
		offY := layer.OffsetY * scale
		r.drawTiledBg(screen, layer.Image,
			offset.X, offset.Y+offY, drawW, math.Max(1, drawH-offY),
			relCamX, relCamY, parallax, alpha*layer.Alpha)
	}
}

// drawTiledBg fills the rectangle [x, y, w, h] on screen with img scaled to
// cover the area, then tiled so it repeats if the scaled image is smaller.
// camX/camY are used with the parallax factor to shift the tile origin.
func (r *RaidRenderer) drawTiledBg(screen *ebiten.Image, img *ebiten.Image, x, y, w, h, camX, camY, parallax, alpha float64) {
	if img == nil {
		return
	}
	b := img.Bounds()
	imgW, imgH := float64(b.Dx()), float64(b.Dy())
	if imgW <= 0 || imgH <= 0 {
		return
	}

	// Scale the image to fully cover the draw area.
	bgScale := math.Max(w/imgW, h/imgH)
	scaledW := imgW * bgScale
	scaledH := imgH * bgScale

	// Parallax scroll: camera movement shifts the tile origin by a small factor.
	// Use modulo to keep the offset within one tile period.
	px := math.Mod(camX*parallax, scaledW)
	py := math.Mod(camY*parallax, scaledH)

	// Find the first tile origin that is at or left/above the draw rect.
	startX := x - px
	if startX > x {
		startX -= scaledW
	}
	startY := y - py
	if startY > y {
		startY -= scaledH
	}

	for ty := startY; ty < y+h; ty += scaledH {
		for tx := startX; tx < x+w; tx += scaledW {
			r.drawImageAlpha(screen, img, tx, ty, bgScale, bgScale, alpha)
		}
	}
}

func (r *RaidRenderer) drawRoomGeometry(screen *ebiten.Image, room shared.RoomState, camera shared.Vec2, offset shared.Vec2, scale float64, alpha float64) {
	style, ok := r.assets.TileStyles[room.TileStyleID]
	// Try full tile rendering from TMX data first.
	if !r.drawTileLayers(screen, room, camera, offset, scale, alpha) {
		// Fallback: draw solid collision rects with a biome-flavoured colour.
		fillClr, rimClr := platformColors(room.Biome, alpha)
		for _, solid := range room.Solids {
			screenX := offset.X + (solid.X-camera.X)*scale
			screenY := offset.Y + (solid.Y-camera.Y)*scale
			sw, sh := float32(solid.W*scale), float32(solid.H*scale)
			if ok && style.Floor != nil {
				r.drawTiledRect(screen, style.Floor, screenX, screenY, solid.W*scale, solid.H*scale, alpha)
			} else {
				r.drawRect(screen, float32(screenX), float32(screenY), sw, sh, fillClr)
				// Bright top-edge highlight so platforms read as surfaces.
				if sh > 2 {
					r.drawRect(screen, float32(screenX), float32(screenY), sw, float32(math.Max(2, scale*2)), rimClr)
				}
			}
		}

		// One-way platforms: distinct teal/green fill with a bright top rim.
		// Drawn thinner than solid blocks to visually communicate pass-through.
		platFill, platRim := platformOneWayColors(alpha)
		for _, p := range room.Platforms {
			screenX := offset.X + (p.X-camera.X)*scale
			screenY := offset.Y + (p.Y-camera.Y)*scale
			sw := float32(p.W * scale)
			sh := float32(p.H * scale)
			r.drawRect(screen, float32(screenX), float32(screenY), sw, sh, platFill)
			// Bright top rim — twice as thick as for solids, making the
			// landing surface clearly visible.
			rimH := float32(math.Max(3, scale*3))
			r.drawRect(screen, float32(screenX), float32(screenY), sw, rimH, platRim)
		}
	}
	// Boss spawn markers — coloured 1-block squares with a bright outline.
	// Level 1 (mini) = soft purple, level 2 (boss) = vivid red, level 3 (super) = gold.
	blockPx := 16.0 * scale // one block in screen pixels
	for _, bs := range room.BossSpawns {
		fill, rim := bossSpawnColors(bs.Level, alpha)
		screenX := float32(offset.X + (bs.X-camera.X)*scale)
		screenY := float32(offset.Y + (bs.Y-camera.Y)*scale)
		bw := float32(blockPx)
		bh := float32(blockPx)
		lineW := float32(math.Max(1.5, scale*1.5))
		cx32 := screenX + bw*0.5
		cy32 := screenY + bh*0.5
		if bs.Flying {
			// Flying bosses: diamond outline — four lines forming a rotated square.
			// No fill inside so it reads clearly as "aerial".
			half := bw * 0.5
			vector.StrokeLine(screen, cx32, cy32-half, cx32+half, cy32, lineW, rim, false)
			vector.StrokeLine(screen, cx32+half, cy32, cx32, cy32+half, lineW, rim, false)
			vector.StrokeLine(screen, cx32, cy32+half, cx32-half, cy32, lineW, rim, false)
			vector.StrokeLine(screen, cx32-half, cy32, cx32, cy32-half, lineW, rim, false)
			// Thin fill: draw shrinking rects inside to give the diamond a tinted body.
			for step := float32(1); step < half-lineW; step += 2 {
				f := fill
				f.A = uint8(float32(fill.A) * (1 - step/half))
				r.drawRect(screen, cx32-step, cy32-step, step*2, step*2, f)
			}
		} else {
			// Ground bosses: solid filled square with bright outline.
			r.drawRect(screen, screenX, screenY, bw, bh, fill)
			vector.StrokeRect(screen, screenX, screenY, bw, bh, lineW, rim, false)
		}
	}
	// PortalZones are drawn only in debug mode (DrawDebugOverlays).
	// Here we only render active JumpLinks (interactive portals) via the section below.
	// Active rifts — rendered once below with color, shimmer, and ground anchor.
	for _, zone := range room.PvPZones {
		topLeft := shared.Vec2{X: offset.X + (zone.X-camera.X)*scale, Y: offset.Y + (zone.Y-camera.Y)*scale}
		vector.StrokeRect(screen, float32(topLeft.X), float32(topLeft.Y), float32(zone.W*scale), float32(zone.H*scale), float32(math.Max(1, scale*2)), color.RGBA{245, 86, 86, uint8(160 * alpha)}, false)
	}
	for _, link := range room.JumpLinks {
		topLeft := shared.Vec2{X: offset.X + (link.Area.X-camera.X)*scale, Y: offset.Y + (link.Area.Y-camera.Y)*scale}
		vector.StrokeRect(screen, float32(topLeft.X), float32(topLeft.Y), float32(link.Area.W*scale), float32(link.Area.H*scale), float32(math.Max(2, scale*2)), color.RGBA{110, 220, 255, uint8(180 * alpha)}, false)
		linkCenter := shared.Vec2{
			X: topLeft.X + link.Area.W*scale*0.5,
			Y: topLeft.Y + link.Area.H*scale*0.5,
		}
		r.drawEffect(screen, r.assets.FX.Portal, linkCenter.X-link.Area.W*scale*0.18, linkCenter.Y-link.Area.H*scale*0.08, scale*0.7, alpha*0.9)
		r.drawEffect(screen, r.assets.FX.PortalGlow, linkCenter.X-link.Area.W*scale*0.1, linkCenter.Y-link.Area.H*scale*0.16, scale*0.64, alpha*0.8)
		if ok && style.JumpPreview != nil {
			r.drawImageAlpha(screen, style.JumpPreview, topLeft.X+link.Area.W*scale*0.18, topLeft.Y+link.Area.H*scale*0.15, scale*0.9, scale*0.9, alpha)
		}
	}

	// Rifts — color-coded transient portals standing on ground/platform.
	for _, rift := range room.Rifts {
		if !rift.IsOpen() {
			continue
		}
		clr := riftColor(rift.Kind, alpha)
		sx := float32(offset.X + (rift.Area.X-camera.X)*scale)
		sy := float32(offset.Y + (rift.Area.Y-camera.Y)*scale)
		sw, sh := float32(rift.Area.W*scale), float32(rift.Area.H*scale)
		// Faint fill.
		r.drawRect(screen, sx, sy, sw, sh, color.RGBA{clr.R, clr.G, clr.B, uint8(float64(clr.A) * 0.18)})
		lineW := float32(math.Max(2, scale*1.5))
		vector.StrokeRect(screen, sx, sy, sw, sh, lineW, clr, false)
		// Vertical shimmer lines.
		mid := sx + sw*0.5
		vector.StrokeLine(screen, mid-sw*0.15, sy+2, mid-sw*0.15, sy+sh-2, 1, clr, false)
		vector.StrokeLine(screen, mid+sw*0.15, sy+2, mid+sw*0.15, sy+sh-2, 1, clr, false)
		// Ground anchor — thick horizontal line at the rift base to ensure it
		// looks flush to whatever surface it stands on.
		anchorW := float32(math.Max(3, scale*3))
		vector.StrokeLine(screen, sx-anchorW, sy+sh, sx+sw+anchorW, sy+sh, anchorW, clr, false)
	}
}

// drawTileLayers renders all visible tile layers for this room.
// Prefers LDtk layers (direct pixel coords) over TMX GID-based layers.
// Returns true if any tile data was found and rendered.
func (r *RaidRenderer) drawTileLayers(screen *ebiten.Image, room shared.RoomState, camera shared.Vec2, offset shared.Vec2, scale float64, alpha float64) bool {
	mapData, ok := r.assets.TileMaps[room.TemplateID]
	if !ok {
		return false
	}

	// ── LDtk path (preferred) ────────────────────────────────────────────────
	if len(mapData.LDtkLayers) > 0 {
		for _, layer := range mapData.LDtkLayers {
			tileImg := r.assets.TileImages[layer.TilesetPath]
			if tileImg == nil {
				continue
			}
			for _, tile := range layer.Tiles {
				src := image.Rect(tile.SrcX, tile.SrcY, tile.SrcX+tile.W, tile.SrcY+tile.H)
				sub := tileImg.SubImage(src).(*ebiten.Image)
				dx := offset.X + (room.Bounds.X+float64(tile.X)-camera.X)*scale
				dy := offset.Y + (room.Bounds.Y+float64(tile.Y)-camera.Y)*scale
				op := &ebiten.DrawImageOptions{}
				scX, scY := scale, scale
				if tile.FlipH {
					scX = -scale
				}
				if tile.FlipV {
					scY = -scale
				}
				op.GeoM.Scale(scX, scY)
				if tile.FlipH {
					op.GeoM.Translate(float64(tile.W)*scale, 0)
				}
				if tile.FlipV {
					op.GeoM.Translate(0, float64(tile.H)*scale)
				}
				op.GeoM.Translate(dx, dy)
				op.ColorScale.Scale(1, 1, 1, float32(alpha*tile.Alpha))
				screen.DrawImage(sub, op)
			}
		}
		return true
	}

	return false
}

func (r *RaidRenderer) drawDecorations(screen *ebiten.Image, room shared.RoomState, camera shared.Vec2, offset shared.Vec2, scale float64, alpha float64) {
	for _, decoration := range room.Decorations {
		frame := r.assets.IdleFrame(decoration.ProfileID)
		if frame == nil {
			continue
		}
		screenPos := shared.Vec2{
			X: offset.X + (decoration.Position.X+decoration.DrawOffset.X-camera.X)*scale,
			Y: offset.Y + (decoration.Position.Y+decoration.DrawOffset.Y-camera.Y)*scale,
		}
		targetScale := scale * maxf(0.1, decoration.Scale)
		r.drawImageAlpha(screen, frame, screenPos.X, screenPos.Y, targetScale, targetScale, alpha*maxf(0.1, decoration.Alpha+1))
	}
	for _, exit := range room.Exits {
		topLeft := shared.Vec2{X: offset.X + (exit.Area.X-camera.X)*scale, Y: offset.Y + (exit.Area.Y-camera.Y)*scale}
		vector.StrokeRect(screen, float32(topLeft.X), float32(topLeft.Y), float32(exit.Area.W*scale), float32(exit.Area.H*scale), 2, color.RGBA{120, 255, 145, uint8(200 * alpha)}, false)
	}
}

// DrawDebugOverlays renders zone overlays that are only useful during debugging:
// rift spawn zones (dim teal) and reveal zones (dim yellow). Call only when
// the debug-physics flag is active.
func (r *RaidRenderer) DrawDebugOverlays(screen *ebiten.Image, room shared.RoomState, camera shared.Vec2, offset shared.Vec2, scale float64) {
	const alpha = 1.0
	// Rift spawn zones — dim teal fill + outline.
	for _, zone := range room.RiftZones {
		sx := float32(offset.X + (zone.X-camera.X)*scale)
		sy := float32(offset.Y + (zone.Y-camera.Y)*scale)
		sw := float32(zone.W * scale)
		sh := float32(zone.H * scale)
		r.drawRect(screen, sx, sy, sw, sh, color.RGBA{30, 180, 160, 40})
		vector.StrokeRect(screen, sx, sy, sw, sh, float32(math.Max(1, scale)), color.RGBA{40, 220, 190, 120}, false)
	}
	// Portal zones — gold fill (unconnected portal positions).
	for _, zone := range room.PortalZones {
		sx := float32(offset.X + (zone.X-camera.X)*scale)
		sy := float32(offset.Y + (zone.Y-camera.Y)*scale)
		sw := float32(zone.W * scale)
		sh := float32(zone.H * scale)
		r.drawRect(screen, sx, sy, sw, sh, color.RGBA{255, 200, 60, 30})
		vector.StrokeRect(screen, sx, sy, sw, sh, float32(math.Max(1, scale)), color.RGBA{255, 220, 80, 100}, false)
		cx32 := sx + sw*0.5
		vector.StrokeLine(screen, cx32, sy+2, cx32, sy+sh-2, float32(math.Max(1, scale*0.5)), color.RGBA{255, 240, 140, 160}, false)
	}
	// Reveal zones — dim yellow outline so portal detection areas are visible.
	for _, rz := range room.RevealZones {
		sx := float32(offset.X + (rz.Area.X-camera.X)*scale)
		sy := float32(offset.Y + (rz.Area.Y-camera.Y)*scale)
		sw := float32(rz.Area.W * scale)
		sh := float32(rz.Area.H * scale)
		vector.StrokeRect(screen, sx, sy, sw, sh, float32(math.Max(1, scale)), color.RGBA{255, 230, 80, 100}, false)
	}
	_ = alpha
}

// DrawLowerEntities renders players and bots that are in the background room
// as dim ghost-like figures on top of that room's geometry.
// bgRoomID is the room currently rendered in the background (already resolved:
// either a RevealZone target or BelowRoomID). Pass "" to skip rendering.
func (r *RaidRenderer) DrawLowerEntities(screen *ebiten.Image, bgRoomID string, entities []shared.EntityState, camera shared.Vec2, offset shared.Vec2, scale float64) {
	if bgRoomID == "" {
		return
	}

	// Use exactly the same transform as drawRoomAsBackground so entities
	// sit correctly on the background terrain.
	bgSc, bgCam := bgRoomTransform(camera, scale, offset)

	for _, entity := range entities {
		if entity.RoomID != bgRoomID {
			continue
		}
		frame := r.assets.IdleFrame(entity.ProfileID)
		if frame == nil {
			continue
		}
		bounds := frame.Bounds()
		entityScale := bgSc * maxf(0.1, entity.Scale)
		// Position.Y is the FEET of the entity; sprite must be drawn ABOVE it.
		// Also center horizontally on Position.X (same convention as foreground drawEntity).
		screenPos := shared.Vec2{
			X: offset.X + (entity.Position.X-bgCam.X)*bgSc - float64(bounds.Dx())*entityScale*0.5,
			Y: offset.Y + (entity.Position.Y-bgCam.Y)*bgSc - float64(bounds.Dy())*entityScale,
		}
		op := &ebiten.DrawImageOptions{}
		if entity.Facing < 0 {
			op.GeoM.Scale(-entityScale, entityScale)
			op.GeoM.Translate(float64(bounds.Dx())*entityScale, 0)
		} else {
			op.GeoM.Scale(entityScale, entityScale)
		}
		op.GeoM.Translate(screenPos.X, screenPos.Y)
		// Entities in the background room use a strongly-shifted colour so the
		// player can immediately tell they are in a different location.
		// Players → cyan-blue (friendly / recognisable at a glance).
		// Rats    → red-orange (enemy silhouette — danger visible from afar).
		if entity.Kind == shared.EntityKindPlayer {
			op.ColorScale.Scale(0.15, 0.65, 1.0, 0.78)
		} else {
			op.ColorScale.Scale(1.0, 0.28, 0.12, 0.78)
		}
		screen.DrawImage(frame, op)
	}
}

func (r *RaidRenderer) drawTiledRect(screen *ebiten.Image, image *ebiten.Image, x float64, y float64, width float64, height float64, alpha float64) {
	if image == nil {
		return
	}
	bounds := image.Bounds()
	imgW := float64(bounds.Dx())
	imgH := float64(bounds.Dy())
	if imgW <= 0 || imgH <= 0 {
		return
	}
	tilesX := int(math.Ceil(width / imgW))
	tilesY := int(math.Ceil(height / imgH))
	for ty := 0; ty < tilesY; ty++ {
		for tx := 0; tx < tilesX; tx++ {
			drawW := math.Min(imgW, width-float64(tx)*imgW)
			drawH := math.Min(imgH, height-float64(ty)*imgH)
			if drawW <= 0 || drawH <= 0 {
				continue
			}
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(drawW/imgW, drawH/imgH)
			op.GeoM.Translate(x+float64(tx)*imgW, y+float64(ty)*imgH)
			op.ColorScale.Scale(1, 1, 1, float32(alpha))
			screen.DrawImage(image, op)
		}
	}
}

func (r *RaidRenderer) drawImageAlpha(screen *ebiten.Image, image *ebiten.Image, x float64, y float64, scaleX float64, scaleY float64, alpha float64) {
	if image == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scaleX, scaleY)
	op.GeoM.Translate(x, y)
	op.ColorScale.Scale(1, 1, 1, float32(alpha))
	screen.DrawImage(image, op)
}

func (r *RaidRenderer) drawEffect(screen *ebiten.Image, image *ebiten.Image, x float64, y float64, scale float64, alpha float64) {
	if image == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x, y)
	op.ColorScale.Scale(1, 1, 1, float32(alpha))
	screen.DrawImage(image, op)
}

// riftColor returns the stroke colour for a rift based on its kind.
// Red = 5 uses, Blue = 2 uses, Green = 1 use.
func riftColor(kind shared.RiftKind, alpha float64) color.RGBA {
	a := uint8(255 * alpha)
	switch kind {
	case shared.RiftKindRed:
		return color.RGBA{255, 60, 60, a}
	case shared.RiftKindBlue:
		return color.RGBA{80, 140, 255, a}
	case shared.RiftKindGreen:
		return color.RGBA{60, 210, 90, a}
	}
	return color.RGBA{200, 200, 200, a}
}

// bossSpawnColors returns a fill and rim colour for a boss spawn marker based on
// boss level. Level 1 (mini) = muted purple; 2 (boss) = vivid crimson;
// 3 (super) = bright gold. Unknown levels default to a neutral grey.
func bossSpawnColors(level int, alpha float64) (fill, rim color.RGBA) {
	a := uint8(255 * alpha)
	switch level {
	case 1: // mini boss — soft lavender
		fill = color.RGBA{90, 55, 140, a}
		rim = color.RGBA{180, 130, 255, a}
	case 2: // boss — deep crimson
		fill = color.RGBA{160, 30, 30, a}
		rim = color.RGBA{255, 80, 80, a}
	case 3: // super boss — molten gold
		fill = color.RGBA{160, 100, 10, a}
		rim = color.RGBA{255, 200, 50, a}
	default:
		fill = color.RGBA{80, 80, 80, a}
		rim = color.RGBA{180, 180, 180, a}
	}
	return fill, rim
}

// platformColors returns a fill colour and a bright top-edge rim colour for
// platformOneWayColors returns fill and bright-rim colours for one-way platforms
// (sgCellPlatform). Fixed teal palette — independent of biome — so the player
// can immediately recognise pass-through surfaces regardless of location theme.
func platformOneWayColors(alpha float64) (fill, rim color.RGBA) {
	a := uint8(255 * alpha)
	fill = color.RGBA{30, 110, 90, a} // dark teal body
	rim = color.RGBA{80, 220, 160, a} // bright mint top edge
	return fill, rim
}

// platformColors returns solid-terrain fill and rim colours for the fallback
// (no-tileset) solid-rect renderer.  Each biome gets a distinct palette so
// locations feel visually different even without tile art.
func platformColors(biome string, alpha float64) (fill, rim color.RGBA) {
	a := uint8(255 * alpha)
	switch biome {
	case "crystal":
		// Icy blue-violet
		fill = color.RGBA{55, 70, 120, a}
		rim = color.RGBA{140, 180, 255, a}
	case "forest":
		// Dark earthy green
		fill = color.RGBA{38, 72, 42, a}
		rim = color.RGBA{80, 160, 70, a}
	default: // "ruins" and anything else
		// Worn stone
		fill = color.RGBA{80, 72, 60, a}
		rim = color.RGBA{180, 165, 130, a}
	}
	return fill, rim
}
