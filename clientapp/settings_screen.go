package clientapp

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"warpedrealms/shared"
)

func (g *Game) openSettings() {
	g.previousScreen = g.screen
	g.screen = screenSettings
	g.awaitingBinding = false
	g.bindingCapturePrimed = false
	g.settingsStatus = "Click action to rebind. Backspace resets one action. R resets all. Esc closes."
}

func (g *Game) closeSettings() {
	g.awaitingBinding = false
	g.screen = g.previousScreen
}

func (g *Game) updateSettings() {
	if g.awaitingBinding {
		g.updateBindingCapture()
		return
	}

	for index := range actionOrder {
		rowRect := shared.Rect{X: 208, Y: float64(212 + index*72), W: 864, H: 58}
		if uiClick(rowRect) {
			g.settingsSelection = index
			g.awaitingBinding = true
			g.bindingCapturePrimed = false
			g.settingsStatus = fmt.Sprintf("Press a key or mouse button for %s. Esc cancels.", ActionLabel(actionOrder[g.settingsSelection]))
			return
		}
	}
	if uiClick(shared.Rect{X: 208, Y: 152, W: 156, H: 36}) {
		g.awaitingBinding = true
		g.bindingCapturePrimed = false
		g.settingsStatus = fmt.Sprintf("Press a key or mouse button for %s. Esc cancels.", ActionLabel(actionOrder[g.settingsSelection]))
		return
	}
	if uiClick(shared.Rect{X: 382, Y: 152, W: 166, H: 36}) {
		action := actionOrder[g.settingsSelection]
		g.controls.Reset(action)
		g.saveControls(fmt.Sprintf("%s reset to %s.", ActionLabel(action), BindingLabel(g.controls, action)))
		return
	}
	if uiClick(shared.Rect{X: 566, Y: 152, W: 166, H: 36}) {
		g.controls.ResetAll()
		g.saveControls("All actions restored to defaults.")
		return
	}
	if uiClick(shared.Rect{X: 950, Y: 152, W: 122, H: 36}) {
		g.closeSettings()
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closeSettings()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		g.settingsSelection--
		if g.settingsSelection < 0 {
			g.settingsSelection = len(actionOrder) - 1
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		g.settingsSelection = (g.settingsSelection + 1) % len(actionOrder)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.awaitingBinding = true
		g.bindingCapturePrimed = false
		g.settingsStatus = fmt.Sprintf("Press a key or mouse button for %s. Esc cancels.", ActionLabel(actionOrder[g.settingsSelection]))
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		action := actionOrder[g.settingsSelection]
		g.controls.Reset(action)
		g.saveControls(fmt.Sprintf("%s reset to %s.", ActionLabel(action), BindingLabel(g.controls, action)))
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.controls.ResetAll()
		g.saveControls("All actions restored to defaults.")
	}
}

func (g *Game) updateBindingCapture() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.awaitingBinding = false
		g.bindingCapturePrimed = false
		g.settingsStatus = "Binding cancelled."
		return
	}

	if !g.bindingCapturePrimed {
		if anyBindingSourcePressed() {
			return
		}
		g.bindingCapturePrimed = true
		return
	}

	for _, button := range []ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonRight, ebiten.MouseButtonMiddle, ebiten.MouseButton3, ebiten.MouseButton4} {
		if !inpututil.IsMouseButtonJustPressed(button) {
			continue
		}
		action := actionOrder[g.settingsSelection]
		g.controls.Set(action, MouseBinding(button))
		g.awaitingBinding = false
		g.bindingCapturePrimed = false
		g.saveControls(fmt.Sprintf("%s bound to %s.", ActionLabel(action), MouseButtonLabel(button)))
		return
	}

	keys := inpututil.AppendJustPressedKeys(nil)
	if len(keys) == 0 {
		return
	}

	for _, key := range keys {
		if key == ebiten.KeyEscape || key == ebiten.KeyF1 {
			continue
		}
		action := actionOrder[g.settingsSelection]
		g.controls.Set(action, KeyBinding(key))
		g.awaitingBinding = false
		g.bindingCapturePrimed = false
		g.saveControls(fmt.Sprintf("%s bound to %s.", ActionLabel(action), KeyLabel(key)))
		return
	}
}

func (g *Game) saveControls(successMessage string) {
	if err := g.controls.Save(g.settingsPath); err != nil {
		g.settingsStatus = fmt.Sprintf("Save settings failed: %v", err)
		return
	}
	g.settingsStatus = successMessage
}

func (g *Game) drawSettings(screen *ebiten.Image) {
	screen.Fill(color.RGBA{11, 15, 21, 255})
	drawPanel(screen, shared.Rect{X: 170, Y: 72, W: 940, H: 576}, color.RGBA{17, 27, 38, 240}, color.RGBA{90, 150, 210, 255}, 2)

	ebitenutil.DebugPrintAt(screen, "Client Settings", 208, 104)
	ebitenutil.DebugPrintAt(screen, "Click an action row, then press a key or mouse button.", 208, 128)

	actions := []struct {
		label string
		rect  shared.Rect
	}{
		{label: "Rebind", rect: shared.Rect{X: 208, Y: 152, W: 156, H: 36}},
		{label: "Reset Row", rect: shared.Rect{X: 382, Y: 152, W: 166, H: 36}},
		{label: "Reset All", rect: shared.Rect{X: 566, Y: 152, W: 166, H: 36}},
		{label: "Close", rect: shared.Rect{X: 950, Y: 152, W: 122, H: 36}},
	}
	for _, button := range actions {
		drawButton(screen, button.rect, false, uiHover(button.rect))
		ebitenutil.DebugPrintAt(screen, button.label, int(button.rect.X)+16, int(button.rect.Y)+11)
	}

	y := 212
	for index, action := range actionOrder {
		fill := color.RGBA{18, 30, 42, 220}
		stroke := color.RGBA{60, 90, 120, 255}
		rowRect := shared.Rect{X: 208, Y: float64(y), W: 864, H: 58}
		if index == g.settingsSelection {
			fill = color.RGBA{34, 50, 70, 235}
			stroke = color.RGBA{120, 190, 235, 255}
		} else if uiHover(rowRect) {
			fill = color.RGBA{28, 42, 58, 230}
			stroke = color.RGBA{96, 156, 200, 255}
		}
		vector.DrawFilledRect(screen, 208, float32(y), 864, 58, fill, false)
		vector.StrokeRect(screen, 208, float32(y), 864, 58, 1, stroke, false)
		ebitenutil.DebugPrintAt(screen, ActionLabel(action), 232, y+12)
		ebitenutil.DebugPrintAt(screen, BindingLabel(g.controls, action), 856, y+12)
		y += 72
	}

	if g.awaitingBinding {
		vector.DrawFilledRect(screen, 276, 512, 728, 70, color.RGBA{0, 0, 0, 190}, false)
		vector.StrokeRect(screen, 276, 512, 728, 70, 1, color.RGBA{130, 190, 240, 255}, false)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Waiting for key or mouse: %s", ActionLabel(actionOrder[g.settingsSelection])), 304, 538)
	}

	ebitenutil.DebugPrintAt(screen, g.settingsStatus, 208, 614)
}

func anyBindingSourcePressed() bool {
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButton3) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButton4) {
		return true
	}
	return len(inpututil.AppendPressedKeys(nil)) > 0
}
