package editorapp

import (
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"warpedrealms/clientapp"
	"warpedrealms/content"
	"warpedrealms/shared"
)

type mode int

const (
	modeAsset mode = iota
	modeRoom
	modePreview
)

type assetField int

const (
	assetFieldScale assetField = iota
	assetFieldSpriteOffset
	assetFieldSpriteSize
	assetFieldCollider
	assetFieldHurtbox
	assetFieldHitbox
)

type roomField int

const (
	roomFieldSolid roomField = iota
	roomFieldJumpLink
	roomFieldRevealZone
	roomFieldPvP
	roomFieldDecoration
	roomFieldMob
	roomFieldMimic
	roomFieldBoss
	roomFieldLoot
)

type dragKind int

const (
	dragNone dragKind = iota
	dragMoveRect
	dragResizeRect
	dragMoveVec
	dragMovePoint
	dragPan
)

type dragState struct {
	kind       dragKind
	tag        string
	startMouse shared.Vec2
	startRect  shared.Rect
	startVec   shared.Vec2
	startPan   shared.Vec2
}

type App struct {
	manifestPath string
	roomsDir     string
	bundle       *content.Bundle
	assets       *clientapp.Assets
	renderer     *clientapp.RaidRenderer

	mode          mode
	status        string
	lastSave      time.Time
	seed          int64
	preview       *content.GeneratedRaid
	profileIDs    []string
	profileIndex  int
	templateIndex int
	assetField    assetField
	roomField     roomField
	itemIndex     int

	roomZoom float64
	roomPan  shared.Vec2
	drag     dragState
}

func NewApp(manifestPath string, roomsDir string) (*App, error) {
	app := &App{
		manifestPath: manifestPath,
		roomsDir:     roomsDir,
		seed:         time.Now().UnixNano(),
		status:       "Mouse-first editor: click tabs, drag boxes, wheel zooms, right-drag pans, Cmd/Ctrl+S saves.",
		roomZoom:     1,
	}
	if err := app.reload(); err != nil {
		return nil, err
	}
	return app, nil
}

func (a *App) reload() error {
	bundle, err := content.LoadBundle(a.manifestPath, a.roomsDir)
	if err != nil {
		return err
	}
	assets, err := clientapp.LoadAssets(bundle.Manifest)
	if err != nil {
		return err
	}
	a.bundle = bundle
	a.assets = assets
	a.renderer = clientapp.NewRaidRenderer(assets)
	a.profileIDs = bundle.Manifest.SortedProfileIDs()
	if len(a.profileIDs) > 0 && a.profileIndex >= len(a.profileIDs) {
		a.profileIndex = len(a.profileIDs) - 1
	}
	if len(a.bundle.RoomTemplates) > 0 && a.templateIndex >= len(a.bundle.RoomTemplates) {
		a.templateIndex = len(a.bundle.RoomTemplates) - 1
	}
	a.roomZoom = 1
	a.roomPan = shared.Vec2{}
	a.drag = dragState{}
	preview, err := a.bundle.GenerateRaid(a.seed)
	if err != nil {
		return err
	}
	a.preview = preview
	return nil
}

func (a *App) Layout(_, _ int) (int, int) {
	return shared.ScreenWidth, shared.ScreenHeight
}

func (a *App) Update() error {
	a.handleGlobalUI()

	switch a.mode {
	case modeAsset:
		a.updateAssetMode()
	case modeRoom:
		a.updateRoomMode()
	case modePreview:
		a.updatePreviewMode()
	}

	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) &&
		!ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) &&
		!ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		a.drag = dragState{}
	}
	return nil
}

func (a *App) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{11, 16, 22, 255})
	a.drawChrome(screen)
	switch a.mode {
	case modeAsset:
		a.drawAssetMode(screen)
	case modeRoom:
		a.drawRoomMode(screen)
	case modePreview:
		a.drawPreviewMode(screen)
	}
	ebitenutil.DebugPrintAt(screen, a.status, 20, shared.ScreenHeight-24)
}

func (a *App) handleGlobalUI() {
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		a.mode = (a.mode + 1) % 3
		a.itemIndex = 0
		a.drag = dragState{}
	}
	for currentMode, rect := range a.modeTabRects() {
		if uiClick(rect) {
			a.mode = currentMode
			a.itemIndex = 0
			a.drag = dragState{}
		}
	}
	if uiClick(a.saveButtonRect()) || (inpututil.IsKeyJustPressed(ebiten.KeyS) && (ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta))) {
		if err := a.save(); err != nil {
			a.status = fmt.Sprintf("Save failed: %v", err)
		} else {
			a.lastSave = time.Now()
			a.status = fmt.Sprintf("Saved at %s", a.lastSave.Format("15:04:05"))
		}
	}
}

func (a *App) updateAssetMode() {
	for index, rect := range a.profileListRects() {
		if uiClick(rect) {
			a.profileIndex = index
			a.drag = dragState{}
		}
	}
	for field, rect := range a.assetFieldRects() {
		if uiClick(rect) {
			a.assetField = field
			a.drag = dragState{}
		}
	}

	profileID := a.currentProfileID()
	if profileID == "" {
		return
	}
	profile := a.bundle.Manifest.Profiles[profileID]
	canvas, previewScale, anchor, spriteRect, colliderRect, hurtboxRect, hitboxRect := a.assetPreviewGeometry(profile)
	mouse := editorMouse()
	_, wheelY := ebiten.Wheel()

	if uiHover(canvas) && wheelY != 0 {
		switch a.assetField {
		case assetFieldScale:
			profile.Scale = clampf(profile.Scale+wheelY*0.05, 0.1, 3)
		case assetFieldSpriteSize:
			profile.SpriteSize.X = maxf(12, profile.SpriteSize.X+wheelY*8)
			profile.SpriteSize.Y = maxf(12, profile.SpriteSize.Y+wheelY*8)
		}
		a.bundle.Manifest.Profiles[profileID] = profile
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && uiHover(canvas) {
		if field, ok := assetFieldForClick(mouse, spriteRect, colliderRect, hurtboxRect, hitboxRect); ok {
			a.assetField = field
		}
		switch a.assetField {
		case assetFieldSpriteOffset:
			if spriteRect.ContainsPoint(mouse) {
				a.drag = dragState{kind: dragMoveVec, tag: "sprite_offset", startMouse: mouse, startVec: profile.SpriteOffset}
			}
		case assetFieldSpriteSize:
			if rectHandle(spriteRect).ContainsPoint(mouse) {
				a.drag = dragState{kind: dragMoveVec, tag: "sprite_size", startMouse: mouse, startVec: profile.SpriteSize}
			}
		case assetFieldCollider:
			a.beginRectDrag(mouse, colliderRect, profile.Collider, "collider")
		case assetFieldHurtbox:
			a.beginRectDrag(mouse, hurtboxRect, profile.Hurtbox, "hurtbox")
		case assetFieldHitbox:
			if len(profile.Hitboxes) > 0 && len(profile.Hitboxes[0].Frames) > 0 {
				a.beginRectDrag(mouse, hitboxRect, profile.Hitboxes[0].Frames[0].Box, "hitbox")
			}
		}
	}

	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && a.drag.kind != dragNone {
		delta := mouse.Sub(a.drag.startMouse).Div(previewScale)
		switch a.drag.kind {
		case dragMoveVec:
			switch a.drag.tag {
			case "sprite_offset":
				profile.SpriteOffset = a.drag.startVec.Add(delta)
			case "sprite_size":
				profile.SpriteSize = shared.Vec2{
					X: maxf(8, a.drag.startVec.X+delta.X),
					Y: maxf(8, a.drag.startVec.Y+delta.Y),
				}
			}
		case dragMoveRect, dragResizeRect:
			updated := applyDragToRect(a.drag, delta)
			switch a.drag.tag {
			case "collider":
				profile.Collider = updated
			case "hurtbox":
				profile.Hurtbox = updated
			case "hitbox":
				if len(profile.Hitboxes) > 0 && len(profile.Hitboxes[0].Frames) > 0 {
					profile.Hitboxes[0].Frames[0].Box = updated
				}
			}
		}
		a.bundle.Manifest.Profiles[profileID] = profile
	}

	_ = anchor
}

func (a *App) updateRoomMode() {
	for index, rect := range a.templateListRects() {
		if uiClick(rect) {
			a.templateIndex = index
			a.itemIndex = 0
			a.roomZoom = 1
			a.roomPan = shared.Vec2{}
			a.drag = dragState{}
		}
	}
	for field, rect := range a.roomFieldRects() {
		if uiClick(rect) {
			a.roomField = field
			a.itemIndex = 0
			a.drag = dragState{}
		}
	}

	template := a.currentTemplate()
	if template.ID == "" {
		return
	}

	canvas := a.roomCanvasRect()
	mouse := editorMouse()
	scale, origin := a.roomViewTransform(template)
	_, wheelY := ebiten.Wheel()

	if uiHover(canvas) && wheelY != 0 {
		if a.roomField == roomFieldDecoration && len(template.Decorations) > 0 {
			index := wrapIndex(a.itemIndex, len(template.Decorations))
			current := 1.0
			if template.Decorations[index].Override.Scale != nil {
				current = *template.Decorations[index].Override.Scale
			}
			current = clampf(current+wheelY*0.05, 0.2, 3)
			template.Decorations[index].Override.Scale = &current
		} else {
			a.roomZoom = clampf(a.roomZoom*(1+wheelY*0.09), 0.45, 5.2)
		}
	}

	if (inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle)) && uiHover(canvas) {
		a.drag = dragState{kind: dragPan, startMouse: mouse, startPan: a.roomPan}
	}
	if (ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) || ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle)) && a.drag.kind == dragPan {
		a.roomPan = a.drag.startPan.Add(mouse.Sub(a.drag.startMouse))
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && uiHover(canvas) {
		a.beginRoomDrag(&template, mouse, scale, origin)
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && a.drag.kind != dragNone && a.drag.kind != dragPan {
		deltaWorld := mouse.Sub(a.drag.startMouse).Div(scale)
		a.applyRoomDrag(&template, deltaWorld)
	}

	a.bundle.RoomTemplates[a.templateIndex] = template
}

func (a *App) updatePreviewMode() {
	if uiClick(shared.Rect{X: 280, Y: 82, W: 42, H: 34}) || inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) {
		a.seed--
		if preview, err := a.bundle.GenerateRaid(a.seed); err == nil {
			a.preview = preview
		}
	}
	if uiClick(shared.Rect{X: 330, Y: 82, W: 42, H: 34}) || inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) {
		a.seed++
		if preview, err := a.bundle.GenerateRaid(a.seed); err == nil {
			a.preview = preview
		}
	}
	if uiClick(shared.Rect{X: 388, Y: 82, W: 112, H: 34}) {
		a.seed = time.Now().UnixNano()
		if preview, err := a.bundle.GenerateRaid(a.seed); err == nil {
			a.preview = preview
		}
	}
}

func (a *App) drawChrome(screen *ebiten.Image) {
	drawPanel(screen, shared.Rect{X: 0, Y: 0, W: shared.ScreenWidth, H: 64}, color.RGBA{13, 19, 28, 245}, color.RGBA{52, 78, 102, 255}, 1)
	ebitenutil.DebugPrintAt(screen, "WarpedRealms Editor", 20, 18)
	ebitenutil.DebugPrintAt(screen, "Closer to Unity than debug-hotkeys: click tabs, drag boxes, wheel zooms, save often.", 20, 38)

	for currentMode, rect := range a.modeTabRects() {
		drawButton(screen, rect, a.mode == currentMode, uiHover(rect))
		ebitenutil.DebugPrintAt(screen, a.modeLabel(currentMode), int(rect.X)+14, int(rect.Y)+10)
	}

	saveRect := a.saveButtonRect()
	drawButton(screen, saveRect, false, uiHover(saveRect))
	ebitenutil.DebugPrintAt(screen, "Save", int(saveRect.X)+18, int(saveRect.Y)+10)
}

func (a *App) drawAssetMode(screen *ebiten.Image) {
	ebitenutil.DebugPrintAt(screen, "Asset Profile Editor", 20, 82)
	ebitenutil.DebugPrintAt(screen, "Wheel on canvas changes scale/size. Drag boxes or sprite. Bottom-right handle resizes.", 20, 100)

	a.drawLeftList(screen, a.profileListRects(), a.profileIDs, a.profileIndex)
	for field, rect := range a.assetFieldRects() {
		drawButton(screen, rect, a.assetField == field, uiHover(rect))
		ebitenutil.DebugPrintAt(screen, a.assetFieldLabel(field), int(rect.X)+12, int(rect.Y)+9)
	}

	profileID := a.currentProfileID()
	if profileID == "" {
		return
	}
	profile := a.bundle.Manifest.Profiles[profileID]
	canvas, previewScale, anchor, spriteRect, colliderRect, hurtboxRect, hitboxRect := a.assetPreviewGeometry(profile)
	drawPanel(screen, canvas, color.RGBA{14, 22, 32, 255}, color.RGBA{80, 140, 190, 255}, 1)

	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s (%s)", profile.Name, profile.ID), int(canvas.X)+18, int(canvas.Y)+16)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("scale %.2f  sprite %.0fx%.0f  offset %.0f,%.0f", profile.Scale, profile.SpriteSize.X, profile.SpriteSize.Y, profile.SpriteOffset.X, profile.SpriteOffset.Y), int(canvas.X)+18, int(canvas.Y)+36)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("collider %.0f,%.0f %.0fx%.0f  hurt %.0f,%.0f %.0fx%.0f", profile.Collider.X, profile.Collider.Y, profile.Collider.W, profile.Collider.H, profile.Hurtbox.X, profile.Hurtbox.Y, profile.Hurtbox.W, profile.Hurtbox.H), int(canvas.X)+18, int(canvas.Y)+56)

	if frame := a.assets.IdleFrame(profileID); frame != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(spriteRect.W/float64(frame.Bounds().Dx()), spriteRect.H/float64(frame.Bounds().Dy()))
		op.GeoM.Translate(spriteRect.X, spriteRect.Y)
		screen.DrawImage(frame, op)
	}
	if composite, ok := a.assets.Composites[profileID]; ok {
		a.drawCompositePreview(screen, anchor, previewScale*0.82, composite)
	}

	drawRect(screen, spriteRect, color.RGBA{120, 160, 220, 160})
	drawHandle(screen, rectHandle(spriteRect), color.RGBA{120, 160, 220, 255})

	if a.assetField == assetFieldCollider || a.assetField == assetFieldScale {
		drawRect(screen, colliderRect, color.RGBA{100, 220, 160, 220})
		drawHandle(screen, rectHandle(colliderRect), color.RGBA{100, 220, 160, 255})
	}
	if a.assetField == assetFieldHurtbox || a.assetField == assetFieldScale {
		drawRect(screen, hurtboxRect, color.RGBA{255, 120, 120, 220})
		drawHandle(screen, rectHandle(hurtboxRect), color.RGBA{255, 120, 120, 255})
	}
	if len(profile.Hitboxes) > 0 && len(profile.Hitboxes[0].Frames) > 0 {
		drawRect(screen, hitboxRect, color.RGBA{255, 225, 110, 220})
		drawHandle(screen, rectHandle(hitboxRect), color.RGBA{255, 225, 110, 255})
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("hitbox %s %.0f,%.0f %.0fx%.0f", profile.Hitboxes[0].State, profile.Hitboxes[0].Frames[0].Box.X, profile.Hitboxes[0].Frames[0].Box.Y, profile.Hitboxes[0].Frames[0].Box.W, profile.Hitboxes[0].Frames[0].Box.H), int(canvas.X)+18, int(canvas.Y)+78)
	}
	a.drawAssetInspector(screen, profile, spriteRect, colliderRect, hurtboxRect, hitboxRect)
}

func (a *App) drawRoomMode(screen *ebiten.Image) {
	ebitenutil.DebugPrintAt(screen, "Room Template Editor", 20, 82)
	ebitenutil.DebugPrintAt(screen, "Left-click moves, handle resizes, wheel zooms, right-drag pans. Decorations scale with wheel.", 20, 100)

	templateNames := make([]string, 0, len(a.bundle.RoomTemplates))
	for _, template := range a.bundle.RoomTemplates {
		templateNames = append(templateNames, template.Name)
	}
	a.drawLeftList(screen, a.templateListRects(), templateNames, a.templateIndex)
	for field, rect := range a.roomFieldRects() {
		drawButton(screen, rect, a.roomField == field, uiHover(rect))
		ebitenutil.DebugPrintAt(screen, a.roomFieldLabel(field), int(rect.X)+10, int(rect.Y)+9)
	}

	template := a.currentTemplate()
	if template.ID == "" {
		return
	}
	canvas := a.roomCanvasRect()
	scale, origin := a.roomViewTransform(template)
	drawPanel(screen, canvas, color.RGBA{14, 22, 32, 255}, color.RGBA{80, 140, 190, 255}, 1)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s (%s)  zoom %.2fx", template.Name, template.ID, a.roomZoom), int(canvas.X)+16, int(canvas.Y)+16)

	roomRect := shared.Rect{X: origin.X, Y: origin.Y, W: template.Size.X * scale, H: template.Size.Y * scale}
	vector.DrawFilledRect(screen, float32(roomRect.X), float32(roomRect.Y), float32(roomRect.W), float32(roomRect.H), color.RGBA{18, 26, 38, 255}, false)
	vector.StrokeRect(screen, float32(roomRect.X), float32(roomRect.Y), float32(roomRect.W), float32(roomRect.H), 1, color.RGBA{86, 134, 172, 255}, false)

	for index, solid := range template.Solids {
		stroke := color.RGBA{94, 180, 132, 220}
		if a.roomField == roomFieldSolid && index == wrapIndex(a.itemIndex, len(template.Solids)) {
			stroke = color.RGBA{190, 255, 215, 255}
		}
		rect := worldRectToScreen(solid, origin, scale)
		drawRect(screen, rect, stroke)
		if a.roomField == roomFieldSolid && index == wrapIndex(a.itemIndex, len(template.Solids)) {
			drawHandle(screen, rectHandle(rect), stroke)
		}
	}
	for index, link := range template.JumpLinks {
		stroke := color.RGBA{120, 220, 255, 220}
		if a.roomField == roomFieldJumpLink && index == wrapIndex(a.itemIndex, len(template.JumpLinks)) {
			stroke = color.RGBA{255, 255, 255, 255}
		}
		areaRect := worldRectToScreen(link.Area, origin, scale)
		drawRect(screen, areaRect, stroke)
		drawHandle(screen, rectHandle(areaRect), stroke)
		previewRect := worldRectToScreen(link.PreviewRect, origin, scale)
		drawRect(screen, previewRect, color.RGBA{200, 235, 255, 90})
		arrival := worldToScreen(link.Arrival, origin, scale)
		drawDot(screen, arrival, 5, stroke)
	}
	for index, zone := range template.RevealZones {
		stroke := color.RGBA{235, 225, 120, 220}
		if a.roomField == roomFieldRevealZone && index == wrapIndex(a.itemIndex, len(template.RevealZones)) {
			stroke = color.RGBA{255, 255, 255, 255}
		}
		rect := worldRectToScreen(zone.Area, origin, scale)
		drawRect(screen, rect, stroke)
		drawHandle(screen, rectHandle(rect), stroke)
	}
	for index, zone := range template.PvPZones {
		stroke := color.RGBA{255, 110, 110, 220}
		if a.roomField == roomFieldPvP && index == wrapIndex(a.itemIndex, len(template.PvPZones)) {
			stroke = color.RGBA{255, 220, 220, 255}
		}
		rect := worldRectToScreen(zone, origin, scale)
		drawRect(screen, rect, stroke)
		drawHandle(screen, rectHandle(rect), stroke)
	}

	a.drawPlacedAssets(screen, template.Decorations, origin, scale, a.roomField == roomFieldDecoration)
	a.drawSpawnPoints(screen, template.Mobs, origin, scale, color.RGBA{120, 220, 255, 255}, a.roomField == roomFieldMob)
	a.drawMimics(screen, template.Mimics, origin, scale, a.roomField == roomFieldMimic)
	a.drawBoss(screen, template.Boss, origin, scale, a.roomField == roomFieldBoss)
	a.drawLoot(screen, template.Loot, origin, scale, a.roomField == roomFieldLoot)

	a.drawRoomInspector(screen, template)
}

func (a *App) drawPreviewMode(screen *ebiten.Image) {
	ebitenutil.DebugPrintAt(screen, "Raid Preview", 20, 82)
	ebitenutil.DebugPrintAt(screen, "Stacked rooms and reveal windows should be visible here before runtime.", 20, 100)

	buttons := []struct {
		rect  shared.Rect
		label string
	}{
		{rect: shared.Rect{X: 280, Y: 82, W: 42, H: 34}, label: "-"},
		{rect: shared.Rect{X: 330, Y: 82, W: 42, H: 34}, label: "+"},
		{rect: shared.Rect{X: 388, Y: 82, W: 112, H: 34}, label: "New Seed"},
	}
	for _, button := range buttons {
		drawButton(screen, button.rect, false, uiHover(button.rect))
		ebitenutil.DebugPrintAt(screen, button.label, int(button.rect.X)+14, int(button.rect.Y)+10)
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Seed: %d", a.seed), 516, 92)

	if a.preview == nil || len(a.preview.Layout.Rooms) == 0 {
		return
	}
	room := a.preview.Layout.Rooms[0]
	camera := shared.Vec2{X: room.Bounds.X, Y: room.Bounds.Y}
	a.renderer.DrawScene(screen, a.preview.Layout, room.ID, camera, shared.Vec2{}, 0.18, nil)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Generated rooms: %d", len(a.preview.Layout.Rooms)), 20, 124)
}

func (a *App) drawPlacedAssets(screen *ebiten.Image, decorations []content.PlacedAsset, origin shared.Vec2, scale float64, selected bool) {
	for index, decoration := range decorations {
		point := worldToScreen(decoration.Position, origin, scale)
		frame := a.assets.IdleFrame(decoration.ProfileID)
		if frame != nil {
			op := &ebiten.DrawImageOptions{}
			drawScale := scale
			overrideScale := 1.0
			if decoration.Override.Scale != nil {
				overrideScale = *decoration.Override.Scale
			}
			op.GeoM.Scale(drawScale*overrideScale, drawScale*overrideScale)
			op.GeoM.Translate(point.X, point.Y)
			op.ColorScale.Scale(1, 1, 1, 0.75)
			screen.DrawImage(frame, op)
		}
		stroke := color.RGBA{240, 210, 120, 220}
		if selected && index == wrapIndex(a.itemIndex, len(decorations)) {
			stroke = color.RGBA{255, 255, 255, 255}
		}
		drawDot(screen, point, 5, stroke)
	}
}

func (a *App) drawSpawnPoints(screen *ebiten.Image, mobs []content.MobSpawn, origin shared.Vec2, scale float64, stroke color.RGBA, selected bool) {
	for index, mob := range mobs {
		point := worldToScreen(mob.Position, origin, scale)
		frame := a.assets.IdleFrame(mob.ProfileID)
		if frame != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(scale*0.9, scale*0.9)
			op.GeoM.Translate(point.X, point.Y)
			op.ColorScale.Scale(1, 1, 1, 0.9)
			screen.DrawImage(frame, op)
		}
		if selected && index == wrapIndex(a.itemIndex, len(mobs)) {
			drawDot(screen, point, 6, color.RGBA{255, 255, 255, 255})
		} else {
			drawDot(screen, point, 5, stroke)
		}
	}
}

func (a *App) drawMimics(screen *ebiten.Image, mimics []content.MimicSpawn, origin shared.Vec2, scale float64, selected bool) {
	for index, mimic := range mimics {
		point := worldToScreen(mimic.Position, origin, scale)
		frame := a.assets.IdleFrame(mimic.DisguiseProfileID)
		if frame != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(scale*0.95, scale*0.95)
			op.GeoM.Translate(point.X, point.Y)
			screen.DrawImage(frame, op)
		}
		if selected && index == wrapIndex(a.itemIndex, len(mimics)) {
			drawDot(screen, point, 6, color.RGBA{255, 255, 255, 255})
		} else {
			drawDot(screen, point, 5, color.RGBA{255, 190, 90, 255})
		}
	}
}

func (a *App) drawBoss(screen *ebiten.Image, boss content.BossSpawn, origin shared.Vec2, scale float64, selected bool) {
	point := worldToScreen(boss.Position, origin, scale)
	frame := a.assets.IdleFrame(boss.ProfileID)
	if frame != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale*1.35, scale*1.35)
		op.GeoM.Translate(point.X, point.Y)
		screen.DrawImage(frame, op)
	}
	if selected {
		drawDot(screen, point, 7, color.RGBA{255, 255, 255, 255})
	} else {
		drawDot(screen, point, 6, color.RGBA{255, 94, 94, 255})
	}
}

func (a *App) drawLoot(screen *ebiten.Image, loot []content.LootSpawn, origin shared.Vec2, scale float64, selected bool) {
	for index, drop := range loot {
		point := worldToScreen(drop.Position, origin, scale)
		frame := a.assets.IdleFrame(drop.ProfileID)
		if frame != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(scale*0.9, scale*0.9)
			op.GeoM.Translate(point.X, point.Y)
			screen.DrawImage(frame, op)
		}
		if selected && index == wrapIndex(a.itemIndex, len(loot)) {
			drawDot(screen, point, 6, color.RGBA{255, 255, 255, 255})
		} else {
			drawDot(screen, point, 5, color.RGBA{220, 240, 120, 255})
		}
	}
}

func (a *App) drawLeftList(screen *ebiten.Image, rects []shared.Rect, labels []string, selected int) {
	drawPanel(screen, shared.Rect{X: 16, Y: 124, W: 228, H: 548}, color.RGBA{14, 22, 32, 255}, color.RGBA{80, 140, 190, 255}, 1)
	for index, rect := range rects {
		drawButton(screen, rect, index == selected, uiHover(rect))
		if index < len(labels) {
			ebitenutil.DebugPrintAt(screen, labels[index], int(rect.X)+12, int(rect.Y)+7)
		}
	}
}

func (a *App) drawCompositePreview(screen *ebiten.Image, anchor shared.Vec2, scale float64, composite clientapp.CompositeAsset) {
	for _, part := range []clientapp.LayerSprite{composite.LeftLeg, composite.RightLeg, composite.LeftArm, composite.RightArm, composite.Body, composite.Detail, composite.Eyes, composite.Mouth} {
		if part.Image == nil {
			continue
		}
		op := &ebiten.DrawImageOptions{}
		partScale := scale * maxf(0.4, part.Scale)
		op.GeoM.Scale(partScale, partScale)
		op.GeoM.Translate(anchor.X+part.Offset.X*scale, anchor.Y+part.Offset.Y*scale)
		screen.DrawImage(part.Image, op)
	}
}

func (a *App) save() error {
	if err := content.SaveManifest(a.manifestPath, a.bundle.Manifest); err != nil {
		return err
	}
	for _, template := range a.bundle.RoomTemplates {
		path := filepath.Join(a.roomsDir, template.ID+".json")
		if err := content.SaveRoomTemplate(path, template); err != nil {
			return err
		}
	}
	return a.reload()
}

func (a *App) currentProfileID() string {
	if len(a.profileIDs) == 0 {
		return ""
	}
	return a.profileIDs[wrapIndex(a.profileIndex, len(a.profileIDs))]
}

func (a *App) currentTemplate() content.RoomTemplate {
	if len(a.bundle.RoomTemplates) == 0 {
		return content.RoomTemplate{}
	}
	return a.bundle.RoomTemplates[wrapIndex(a.templateIndex, len(a.bundle.RoomTemplates))]
}

func (a *App) modeTabRects() map[mode]shared.Rect {
	return map[mode]shared.Rect{
		modeAsset:   {X: 416, Y: 14, W: 150, H: 34},
		modeRoom:    {X: 578, Y: 14, W: 166, H: 34},
		modePreview: {X: 756, Y: 14, W: 146, H: 34},
	}
}

func (a *App) saveButtonRect() shared.Rect {
	return shared.Rect{X: 1140, Y: 14, W: 112, H: 34}
}

func (a *App) profileListRects() []shared.Rect {
	rects := make([]shared.Rect, 0, len(a.profileIDs))
	y := 138.0
	for range a.profileIDs {
		rects = append(rects, shared.Rect{X: 24, Y: y, W: 212, H: 28})
		y += 32
		if y > 634 {
			break
		}
	}
	return rects
}

func (a *App) templateListRects() []shared.Rect {
	rects := make([]shared.Rect, 0, len(a.bundle.RoomTemplates))
	y := 138.0
	for range a.bundle.RoomTemplates {
		rects = append(rects, shared.Rect{X: 24, Y: y, W: 212, H: 28})
		y += 32
		if y > 634 {
			break
		}
	}
	return rects
}

func (a *App) assetFieldRects() map[assetField]shared.Rect {
	return map[assetField]shared.Rect{
		assetFieldScale:        {X: 268, Y: 124, W: 104, H: 30},
		assetFieldSpriteOffset: {X: 382, Y: 124, W: 128, H: 30},
		assetFieldSpriteSize:   {X: 520, Y: 124, W: 112, H: 30},
		assetFieldCollider:     {X: 642, Y: 124, W: 102, H: 30},
		assetFieldHurtbox:      {X: 754, Y: 124, W: 104, H: 30},
		assetFieldHitbox:       {X: 868, Y: 124, W: 96, H: 30},
	}
}

func (a *App) roomFieldRects() map[roomField]shared.Rect {
	return map[roomField]shared.Rect{
		roomFieldSolid:      {X: 268, Y: 124, W: 86, H: 30},
		roomFieldJumpLink:   {X: 362, Y: 124, W: 98, H: 30},
		roomFieldRevealZone: {X: 468, Y: 124, W: 110, H: 30},
		roomFieldPvP:        {X: 586, Y: 124, W: 76, H: 30},
		roomFieldDecoration: {X: 670, Y: 124, W: 106, H: 30},
		roomFieldMob:        {X: 784, Y: 124, W: 70, H: 30},
		roomFieldMimic:      {X: 862, Y: 124, W: 84, H: 30},
		roomFieldBoss:       {X: 954, Y: 124, W: 70, H: 30},
		roomFieldLoot:       {X: 1032, Y: 124, W: 70, H: 30},
	}
}

func (a *App) assetCanvasRect() shared.Rect {
	return shared.Rect{X: 262, Y: 166, W: 758, H: 520}
}

func (a *App) roomCanvasRect() shared.Rect {
	return shared.Rect{X: 262, Y: 166, W: 758, H: 520}
}

func (a *App) inspectorRect() shared.Rect {
	return shared.Rect{X: 1032, Y: 166, W: 228, H: 520}
}

func (a *App) assetPreviewGeometry(profile content.AssetProfile) (shared.Rect, float64, shared.Vec2, shared.Rect, shared.Rect, shared.Rect, shared.Rect) {
	canvas := a.assetCanvasRect()
	previewScale := 2.18
	anchor := shared.Vec2{
		X: canvas.X + canvas.W*0.5 - profile.Collider.W*previewScale*0.5,
		Y: canvas.Y + canvas.H*0.54 - profile.Collider.H*previewScale*0.5,
	}
	spriteRect := shared.Rect{
		X: anchor.X + profile.SpriteOffset.X*previewScale,
		Y: anchor.Y + profile.SpriteOffset.Y*previewScale,
		W: profile.SpriteSize.X * previewScale,
		H: profile.SpriteSize.Y * previewScale,
	}
	colliderRect := rectToPreview(profile.Collider, anchor, previewScale)
	hurtboxRect := rectToPreview(profile.Hurtbox, anchor, previewScale)
	hitboxRect := shared.Rect{}
	if len(profile.Hitboxes) > 0 && len(profile.Hitboxes[0].Frames) > 0 {
		hitboxRect = rectToPreview(profile.Hitboxes[0].Frames[0].Box, anchor, previewScale)
	}
	return canvas, previewScale, anchor, spriteRect, colliderRect, hurtboxRect, hitboxRect
}

func (a *App) roomViewTransform(template content.RoomTemplate) (float64, shared.Vec2) {
	canvas := a.roomCanvasRect()
	fitScale := math.Min((canvas.W-40)/template.Size.X, (canvas.H-40)/template.Size.Y)
	if fitScale <= 0 {
		fitScale = 0.05
	}
	scale := fitScale * a.roomZoom
	contentW := template.Size.X * scale
	contentH := template.Size.Y * scale
	base := shared.Vec2{
		X: canvas.X + (canvas.W-contentW)*0.5,
		Y: canvas.Y + (canvas.H-contentH)*0.5,
	}
	return scale, base.Add(a.roomPan)
}

func (a *App) beginRectDrag(mouse shared.Vec2, previewRect shared.Rect, source shared.Rect, tag string) {
	if rectHandle(previewRect).ContainsPoint(mouse) {
		a.drag = dragState{kind: dragResizeRect, tag: tag, startMouse: mouse, startRect: source}
		return
	}
	if previewRect.ContainsPoint(mouse) {
		a.drag = dragState{kind: dragMoveRect, tag: tag, startMouse: mouse, startRect: source}
	}
}

func (a *App) beginRoomDrag(template *content.RoomTemplate, mouse shared.Vec2, scale float64, origin shared.Vec2) {
	switch a.roomField {
	case roomFieldSolid:
		for index, rect := range template.Solids {
			screenRect := worldRectToScreen(rect, origin, scale)
			if rectHandle(screenRect).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragResizeRect, tag: "solid", startMouse: mouse, startRect: rect}
				return
			}
			if screenRect.ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMoveRect, tag: "solid", startMouse: mouse, startRect: rect}
				return
			}
		}
	case roomFieldJumpLink:
		for index, link := range template.JumpLinks {
			arrival := worldToScreen(link.Arrival, origin, scale)
			if pointRect(arrival, 8).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMoveVec, tag: "jump_arrival", startMouse: mouse, startVec: link.Arrival}
				return
			}
			screenRect := worldRectToScreen(link.Area, origin, scale)
			if rectHandle(screenRect).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragResizeRect, tag: "jump_area", startMouse: mouse, startRect: link.Area}
				return
			}
			if screenRect.ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMoveRect, tag: "jump_area", startMouse: mouse, startRect: link.Area}
				return
			}
			previewRect := worldRectToScreen(link.PreviewRect, origin, scale)
			if previewRect.ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMoveRect, tag: "jump_preview", startMouse: mouse, startRect: link.PreviewRect}
				return
			}
		}
	case roomFieldRevealZone:
		for index, zone := range template.RevealZones {
			screenRect := worldRectToScreen(zone.Area, origin, scale)
			if rectHandle(screenRect).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragResizeRect, tag: "reveal", startMouse: mouse, startRect: zone.Area}
				return
			}
			if screenRect.ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMoveRect, tag: "reveal", startMouse: mouse, startRect: zone.Area}
				return
			}
		}
	case roomFieldPvP:
		for index, zone := range template.PvPZones {
			screenRect := worldRectToScreen(zone, origin, scale)
			if rectHandle(screenRect).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragResizeRect, tag: "pvp", startMouse: mouse, startRect: zone}
				return
			}
			if screenRect.ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMoveRect, tag: "pvp", startMouse: mouse, startRect: zone}
				return
			}
		}
	case roomFieldDecoration:
		for index, decoration := range template.Decorations {
			point := worldToScreen(decoration.Position, origin, scale)
			if pointRect(point, 10).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMovePoint, tag: "decoration", startMouse: mouse, startVec: decoration.Position}
				return
			}
		}
	case roomFieldMob:
		for index, mob := range template.Mobs {
			point := worldToScreen(mob.Position, origin, scale)
			if pointRect(point, 10).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMovePoint, tag: "mob", startMouse: mouse, startVec: mob.Position}
				return
			}
		}
	case roomFieldMimic:
		for index, mimic := range template.Mimics {
			point := worldToScreen(mimic.Position, origin, scale)
			if pointRect(point, 10).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMovePoint, tag: "mimic", startMouse: mouse, startVec: mimic.Position}
				return
			}
		}
	case roomFieldBoss:
		point := worldToScreen(template.Boss.Position, origin, scale)
		if pointRect(point, 11).ContainsPoint(mouse) {
			a.drag = dragState{kind: dragMovePoint, tag: "boss", startMouse: mouse, startVec: template.Boss.Position}
		}
	case roomFieldLoot:
		for index, drop := range template.Loot {
			point := worldToScreen(drop.Position, origin, scale)
			if pointRect(point, 10).ContainsPoint(mouse) {
				a.itemIndex = index
				a.drag = dragState{kind: dragMovePoint, tag: "loot", startMouse: mouse, startVec: drop.Position}
				return
			}
		}
	}
}

func (a *App) applyRoomDrag(template *content.RoomTemplate, deltaWorld shared.Vec2) {
	switch a.drag.tag {
	case "solid":
		index := wrapIndex(a.itemIndex, len(template.Solids))
		template.Solids[index] = applyDragToRect(a.drag, deltaWorld)
	case "jump_area":
		index := wrapIndex(a.itemIndex, len(template.JumpLinks))
		template.JumpLinks[index].Area = applyDragToRect(a.drag, deltaWorld)
	case "jump_preview":
		index := wrapIndex(a.itemIndex, len(template.JumpLinks))
		template.JumpLinks[index].PreviewRect = applyDragToRect(a.drag, deltaWorld)
	case "jump_arrival":
		index := wrapIndex(a.itemIndex, len(template.JumpLinks))
		template.JumpLinks[index].Arrival = a.drag.startVec.Add(deltaWorld)
	case "reveal":
		index := wrapIndex(a.itemIndex, len(template.RevealZones))
		template.RevealZones[index].Area = applyDragToRect(a.drag, deltaWorld)
	case "pvp":
		index := wrapIndex(a.itemIndex, len(template.PvPZones))
		template.PvPZones[index] = applyDragToRect(a.drag, deltaWorld)
	case "decoration":
		index := wrapIndex(a.itemIndex, len(template.Decorations))
		template.Decorations[index].Position = a.drag.startVec.Add(deltaWorld)
	case "mob":
		index := wrapIndex(a.itemIndex, len(template.Mobs))
		template.Mobs[index].Position = a.drag.startVec.Add(deltaWorld)
	case "mimic":
		index := wrapIndex(a.itemIndex, len(template.Mimics))
		template.Mimics[index].Position = a.drag.startVec.Add(deltaWorld)
	case "boss":
		template.Boss.Position = a.drag.startVec.Add(deltaWorld)
	case "loot":
		index := wrapIndex(a.itemIndex, len(template.Loot))
		template.Loot[index].Position = a.drag.startVec.Add(deltaWorld)
	}
}

func (a *App) roomSelectionInfo(template content.RoomTemplate) string {
	switch a.roomField {
	case roomFieldSolid:
		if len(template.Solids) == 0 {
			return "No solids."
		}
		rect := template.Solids[wrapIndex(a.itemIndex, len(template.Solids))]
		return fmt.Sprintf("Solid %d: x=%.0f y=%.0f w=%.0f h=%.0f", wrapIndex(a.itemIndex, len(template.Solids))+1, rect.X, rect.Y, rect.W, rect.H)
	case roomFieldJumpLink:
		if len(template.JumpLinks) == 0 {
			return "No jump links."
		}
		link := template.JumpLinks[wrapIndex(a.itemIndex, len(template.JumpLinks))]
		return fmt.Sprintf("Jump %s: area %.0f,%.0f %.0fx%.0f arrival %.0f,%.0f", link.Label, link.Area.X, link.Area.Y, link.Area.W, link.Area.H, link.Arrival.X, link.Arrival.Y)
	case roomFieldRevealZone:
		if len(template.RevealZones) == 0 {
			return "No reveal zones."
		}
		rect := template.RevealZones[wrapIndex(a.itemIndex, len(template.RevealZones))].Area
		return fmt.Sprintf("Reveal: x=%.0f y=%.0f w=%.0f h=%.0f", rect.X, rect.Y, rect.W, rect.H)
	case roomFieldPvP:
		if len(template.PvPZones) == 0 {
			return "No PvP zones."
		}
		rect := template.PvPZones[wrapIndex(a.itemIndex, len(template.PvPZones))]
		return fmt.Sprintf("PvP: x=%.0f y=%.0f w=%.0f h=%.0f", rect.X, rect.Y, rect.W, rect.H)
	case roomFieldDecoration:
		if len(template.Decorations) == 0 {
			return "No decorations."
		}
		decoration := template.Decorations[wrapIndex(a.itemIndex, len(template.Decorations))]
		scale := 1.0
		if decoration.Override.Scale != nil {
			scale = *decoration.Override.Scale
		}
		return fmt.Sprintf("Decoration %s: x=%.0f y=%.0f scale=%.2f", decoration.ProfileID, decoration.Position.X, decoration.Position.Y, scale)
	case roomFieldMob:
		if len(template.Mobs) == 0 {
			return "No mobs."
		}
		mob := template.Mobs[wrapIndex(a.itemIndex, len(template.Mobs))]
		return fmt.Sprintf("Mob %s: x=%.0f y=%.0f", mob.ProfileID, mob.Position.X, mob.Position.Y)
	case roomFieldMimic:
		if len(template.Mimics) == 0 {
			return "No mimics."
		}
		mimic := template.Mimics[wrapIndex(a.itemIndex, len(template.Mimics))]
		return fmt.Sprintf("Mimic %s/%s: x=%.0f y=%.0f", mimic.DisguiseProfileID, mimic.CombatProfileID, mimic.Position.X, mimic.Position.Y)
	case roomFieldBoss:
		return fmt.Sprintf("Boss %s: x=%.0f y=%.0f", template.Boss.ProfileID, template.Boss.Position.X, template.Boss.Position.Y)
	case roomFieldLoot:
		if len(template.Loot) == 0 {
			return "No loot."
		}
		loot := template.Loot[wrapIndex(a.itemIndex, len(template.Loot))]
		return fmt.Sprintf("Loot %s: x=%.0f y=%.0f value=%d", loot.ProfileID, loot.Position.X, loot.Position.Y, loot.Value)
	default:
		return ""
	}
}

func (a *App) drawAssetInspector(screen *ebiten.Image, profile content.AssetProfile, spriteRect shared.Rect, colliderRect shared.Rect, hurtboxRect shared.Rect, hitboxRect shared.Rect) {
	rect := a.inspectorRect()
	drawPanel(screen, rect, color.RGBA{12, 18, 27, 240}, color.RGBA{74, 118, 150, 255}, 1)
	lines := []string{
		"Inspector",
		"",
		fmt.Sprintf("Field: %s", a.assetFieldLabel(a.assetField)),
		fmt.Sprintf("Scale: %.2f", profile.Scale),
		fmt.Sprintf("Sprite: %.0fx%.0f", profile.SpriteSize.X, profile.SpriteSize.Y),
		fmt.Sprintf("Offset: %.0f, %.0f", profile.SpriteOffset.X, profile.SpriteOffset.Y),
		fmt.Sprintf("Collider: %.0f, %.0f", profile.Collider.W, profile.Collider.H),
		fmt.Sprintf("Hurtbox: %.0f, %.0f", profile.Hurtbox.W, profile.Hurtbox.H),
		"",
		"Mouse",
		"LMB: select + drag",
		"Wheel: scale/size",
		"Handle: resize box",
		"",
		"Rects",
		fmt.Sprintf("Sprite @ %.0f,%.0f", spriteRect.X, spriteRect.Y),
		fmt.Sprintf("Collider @ %.0f,%.0f", colliderRect.X, colliderRect.Y),
		fmt.Sprintf("Hurt @ %.0f,%.0f", hurtboxRect.X, hurtboxRect.Y),
	}
	if hitboxRect.W > 0 && hitboxRect.H > 0 {
		lines = append(lines, fmt.Sprintf("Hit @ %.0f,%.0f", hitboxRect.X, hitboxRect.Y))
	}
	drawInspectorLines(screen, rect, lines)
}

func (a *App) drawRoomInspector(screen *ebiten.Image, template content.RoomTemplate) {
	rect := a.inspectorRect()
	drawPanel(screen, rect, color.RGBA{12, 18, 27, 240}, color.RGBA{74, 118, 150, 255}, 1)
	lines := []string{
		"Inspector",
		"",
		fmt.Sprintf("Template: %s", template.Name),
		fmt.Sprintf("Field: %s", a.roomFieldLabel(a.roomField)),
		fmt.Sprintf("Zoom: %.2fx", a.roomZoom),
		fmt.Sprintf("Pan: %.0f, %.0f", a.roomPan.X, a.roomPan.Y),
		"",
		"Counts",
		fmt.Sprintf("Solids: %d", len(template.Solids)),
		fmt.Sprintf("Links: %d", len(template.JumpLinks)),
		fmt.Sprintf("Reveal: %d", len(template.RevealZones)),
		fmt.Sprintf("PvP: %d", len(template.PvPZones)),
		fmt.Sprintf("Decor: %d", len(template.Decorations)),
		fmt.Sprintf("Mobs: %d", len(template.Mobs)),
		fmt.Sprintf("Mimics: %d", len(template.Mimics)),
		fmt.Sprintf("Loot: %d", len(template.Loot)),
		"",
		"Selection",
		a.roomSelectionInfo(template),
		"",
		"Mouse",
		"LMB: move/select",
		"Handle: resize",
		"Wheel: zoom or decor scale",
		"RMB/MMB drag: pan",
	}
	drawInspectorLines(screen, rect, lines)
}

func drawInspectorLines(screen *ebiten.Image, rect shared.Rect, lines []string) {
	y := int(rect.Y) + 18
	for _, line := range lines {
		if line == "" {
			y += 12
			continue
		}
		ebitenutil.DebugPrintAt(screen, line, int(rect.X)+12, y)
		y += 18
		if y > int(rect.Bottom())-18 {
			break
		}
	}
}

func (a *App) modeLabel(current mode) string {
	switch current {
	case modeAsset:
		return "Asset Profiles"
	case modeRoom:
		return "Room Templates"
	default:
		return "Raid Preview"
	}
}

func (a *App) assetFieldLabel(field assetField) string {
	switch field {
	case assetFieldScale:
		return "Scale"
	case assetFieldSpriteOffset:
		return "Sprite Offset"
	case assetFieldSpriteSize:
		return "Sprite Size"
	case assetFieldCollider:
		return "Collider"
	case assetFieldHurtbox:
		return "Hurtbox"
	default:
		return "Hitbox"
	}
}

func (a *App) roomFieldLabel(field roomField) string {
	switch field {
	case roomFieldSolid:
		return "Solids"
	case roomFieldJumpLink:
		return "Jump Links"
	case roomFieldRevealZone:
		return "Reveal"
	case roomFieldPvP:
		return "PvP"
	case roomFieldDecoration:
		return "Decor"
	case roomFieldMob:
		return "Mobs"
	case roomFieldMimic:
		return "Mimics"
	case roomFieldBoss:
		return "Boss"
	default:
		return "Loot"
	}
}

func editorMouse() shared.Vec2 {
	x, y := ebiten.CursorPosition()
	return shared.Vec2{X: float64(x), Y: float64(y)}
}

func uiHover(rect shared.Rect) bool {
	return rect.ContainsPoint(editorMouse())
}

func uiClick(rect shared.Rect) bool {
	return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && uiHover(rect)
}

func drawPanel(screen *ebiten.Image, rect shared.Rect, fill color.RGBA, stroke color.RGBA, strokeWidth float32) {
	vector.DrawFilledRect(screen, float32(rect.X), float32(rect.Y), float32(rect.W), float32(rect.H), fill, false)
	vector.StrokeRect(screen, float32(rect.X), float32(rect.Y), float32(rect.W), float32(rect.H), strokeWidth, stroke, false)
}

func drawButton(screen *ebiten.Image, rect shared.Rect, active bool, hovered bool) {
	fill := color.RGBA{22, 34, 48, 230}
	stroke := color.RGBA{74, 114, 150, 255}
	if hovered {
		fill = color.RGBA{28, 42, 60, 235}
		stroke = color.RGBA{120, 180, 220, 255}
	}
	if active {
		fill = color.RGBA{34, 52, 74, 245}
		stroke = color.RGBA{150, 215, 255, 255}
	}
	drawPanel(screen, rect, fill, stroke, 1)
}

func drawRect(screen *ebiten.Image, rect shared.Rect, stroke color.RGBA) {
	vector.StrokeRect(screen, float32(rect.X), float32(rect.Y), float32(rect.W), float32(rect.H), 2, stroke, false)
}

func drawHandle(screen *ebiten.Image, rect shared.Rect, fill color.RGBA) {
	vector.DrawFilledRect(screen, float32(rect.X), float32(rect.Y), float32(rect.W), float32(rect.H), fill, false)
}

func drawDot(screen *ebiten.Image, point shared.Vec2, radius float64, fill color.RGBA) {
	vector.DrawFilledRect(screen, float32(point.X-radius), float32(point.Y-radius), float32(radius*2), float32(radius*2), fill, false)
}

func assetFieldForClick(mouse shared.Vec2, spriteRect shared.Rect, colliderRect shared.Rect, hurtboxRect shared.Rect, hitboxRect shared.Rect) (assetField, bool) {
	switch {
	case hitboxRect.W > 0 && hitboxRect.H > 0 && hitboxRect.ContainsPoint(mouse):
		return assetFieldHitbox, true
	case hurtboxRect.ContainsPoint(mouse):
		return assetFieldHurtbox, true
	case colliderRect.ContainsPoint(mouse):
		return assetFieldCollider, true
	case rectHandle(spriteRect).ContainsPoint(mouse):
		return assetFieldSpriteSize, true
	case spriteRect.ContainsPoint(mouse):
		return assetFieldSpriteOffset, true
	default:
		return assetFieldScale, false
	}
}

func rectToPreview(rect shared.Rect, anchor shared.Vec2, scale float64) shared.Rect {
	return shared.Rect{
		X: anchor.X + rect.X*scale,
		Y: anchor.Y + rect.Y*scale,
		W: rect.W * scale,
		H: rect.H * scale,
	}
}

func worldRectToScreen(rect shared.Rect, origin shared.Vec2, scale float64) shared.Rect {
	return shared.Rect{
		X: origin.X + rect.X*scale,
		Y: origin.Y + rect.Y*scale,
		W: rect.W * scale,
		H: rect.H * scale,
	}
}

func worldToScreen(point shared.Vec2, origin shared.Vec2, scale float64) shared.Vec2 {
	return shared.Vec2{
		X: origin.X + point.X*scale,
		Y: origin.Y + point.Y*scale,
	}
}

func rectHandle(rect shared.Rect) shared.Rect {
	return shared.Rect{X: rect.Right() - 8, Y: rect.Bottom() - 8, W: 12, H: 12}
}

func pointRect(point shared.Vec2, radius float64) shared.Rect {
	return shared.Rect{X: point.X - radius, Y: point.Y - radius, W: radius * 2, H: radius * 2}
}

func applyDragToRect(drag dragState, delta shared.Vec2) shared.Rect {
	rect := drag.startRect
	switch drag.kind {
	case dragMoveRect:
		rect.X += delta.X
		rect.Y += delta.Y
	case dragResizeRect:
		rect.W = maxf(8, rect.W+delta.X)
		rect.H = maxf(8, rect.H+delta.Y)
	}
	return rect
}

func wrapIndex(index int, length int) int {
	if length == 0 {
		return 0
	}
	for index < 0 {
		index += length
	}
	return index % length
}

func clampf(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func maxf(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
