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
	assets *Assets
}

func NewRaidRenderer(assets *Assets) *RaidRenderer {
	return &RaidRenderer{assets: assets}
}

func (r *RaidRenderer) DrawScene(screen *ebiten.Image, layout shared.RaidLayoutState, activeRoomID string, camera shared.Vec2, offset shared.Vec2, scale float64, preview *roomPreview) {
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
	//    Rendered at bgDepthScale so it looks clearly distant: platforms are
	//    smaller, more of the level is visible at once, entities look far away.
	if activeRoom.BelowRoomID != "" {
		if bgRoom, ok := layout.RoomByID(activeRoom.BelowRoomID); ok {
			bgSc, bgCam := bgRoomTransform(camera, scale, offset)
			r.drawRoomAsBackground(screen, bgRoom, bgCam, offset, bgSc)
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
// lines rather than solid filled blocks.  Only the TOP EDGE of each platform
// cell is drawn (bright biome rim colour); the cell body is a very subtle fill.
// This avoids the solid-fill problem that occurs when many 16×16 cells pack
// together and cover the entire screen with a single colour.
func (r *RaidRenderer) drawRoomAsBackground(screen *ebiten.Image, room shared.RoomState, camera shared.Vec2, offset shared.Vec2, scale float64) {
	const gs = 16.0 // grid cell size in world pixels

	// Build a lookup of occupied cells (skip large boundary-wall rects).
	type cellKey [2]int
	occupied := make(map[cellKey]bool, len(room.Solids))
	for _, s := range room.Solids {
		if s.W != gs || s.H != gs {
			continue
		}
		occupied[cellKey{int(math.Round(s.X / gs)), int(math.Round(s.Y / gs))}] = true
	}

	_, rimClr := platformColors(room.Biome, 0.90)
	fillClr, _ := platformColors(room.Biome, 0.12)
	rimH := float32(math.Max(3, scale*gs*0.30)) // bright top-rim height in screen px

	for _, s := range room.Solids {
		if s.W != gs || s.H != gs {
			continue
		}
		col := int(math.Round(s.X / gs))
		row := int(math.Round(s.Y / gs))

		sx := float32(offset.X + (s.X-camera.X)*scale)
		sy := float32(offset.Y + (s.Y-camera.Y)*scale)
		sw := float32(gs * scale)
		sh := float32(gs * scale)

		// Very subtle body fill — just enough to imply volume.
		vector.DrawFilledRect(screen, sx, sy, sw, sh, fillClr, false)

		// Bright surface rim only on the top-exposed face of each cell.
		if !occupied[cellKey{col, row - 1}] {
			vector.DrawFilledRect(screen, sx, sy, sw, rimH, rimClr, false)
		}
	}

	// Atmospheric depth haze over the whole background layer.
	sb := screen.Bounds()
	vector.DrawFilledRect(screen,
		float32(offset.X), float32(offset.Y),
		float32(float64(sb.Dx())-offset.X), float32(float64(sb.Dy())-offset.Y),
		color.RGBA{6, 10, 20, 110}, false)
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
	vector.DrawFilledRect(screen, float32(topLeft.X+12), float32(topLeft.Y+14), float32(rect.W*scale), float32(rect.H*scale), color.RGBA{0, 0, 0, uint8(76 * preview.alpha)}, false)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(topLeft.X, topLeft.Y)
	op.ColorScale.Scale(1, 1, 1, float32(preview.alpha))
	screen.DrawImage(canvas, op)

	glow := color.RGBA{124, 225, 255, uint8(180 * preview.alpha)}
	vector.StrokeRect(screen, float32(topLeft.X), float32(topLeft.Y), float32(rect.W*scale), float32(rect.H*scale), 3, glow, false)
	vector.StrokeRect(screen, float32(topLeft.X-5), float32(topLeft.Y-5), float32(rect.W*scale+10), float32(rect.H*scale+10), 1, color.RGBA{210, 245, 255, uint8(90 * preview.alpha)}, false)
	vector.DrawFilledRect(screen, float32(topLeft.X), float32(topLeft.Y-24), float32(math.Min(rect.W*scale, 220)), 22, color.RGBA{18, 28, 38, uint8(220 * preview.alpha)}, false)
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
		vector.DrawFilledRect(screen,
			float32(offset.X), float32(offset.Y),
			float32(drawW), float32(drawH),
			fill, false,
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
				vector.DrawFilledRect(screen, float32(screenX), float32(screenY), sw, sh, fillClr, false)
				// Bright top-edge highlight so platforms read as surfaces.
				if sh > 2 {
					vector.DrawFilledRect(screen, float32(screenX), float32(screenY), sw, float32(math.Max(2, scale*2)), rimClr, false)
				}
			}
		}
	}
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

// DrawLowerEntities renders players and rats that are in the background room
// (activeRoom.BelowRoomID) as dim ghost-like figures on top of that room's
// geometry.  Uses the same bgCameraFor transform as drawRoomAsBackground so
// entities sit exactly on the background terrain.
func (r *RaidRenderer) DrawLowerEntities(screen *ebiten.Image, layout shared.RaidLayoutState, activeRoomID string, entities []shared.EntityState, camera shared.Vec2, offset shared.Vec2, scale float64) {
	activeRoom, ok := layout.RoomByID(activeRoomID)
	if !ok || activeRoom.BelowRoomID == "" {
		return
	}
	belowRoom, ok := layout.RoomByID(activeRoom.BelowRoomID)
	if !ok {
		return
	}

	// Use exactly the same transform as drawRoomAsBackground so entities
	// sit correctly on the background terrain.
	bgSc, bgCam := bgRoomTransform(camera, scale, offset)

	for _, entity := range entities {
		if entity.RoomID != belowRoom.ID {
			continue
		}
		if entity.Kind != shared.EntityKindPlayer && entity.Kind != shared.EntityKindRat {
			continue
		}
		frame := r.assets.IdleFrame(entity.ProfileID)
		if frame == nil {
			continue
		}
		screenPos := shared.Vec2{
			X: offset.X + (entity.Position.X-bgCam.X)*bgSc,
			Y: offset.Y + (entity.Position.Y-bgCam.Y)*bgSc,
		}
		bounds := frame.Bounds()
		entityScale := bgSc * maxf(0.1, entity.Scale)
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

// platformColors returns a fill colour and a bright top-edge rim colour for
// the fallback (no-tileset) solid-rect renderer.  Each biome gets a distinct
// palette so locations feel visually different even without tile art.
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
