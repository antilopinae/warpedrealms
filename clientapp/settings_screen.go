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
	g.settingsStatus = "Click a button to rebind. ESC to return."
}

// closeSettings возвращает игрока на экран, с которого он пришел
func (g *Game) closeSettings() {
	g.awaitingBinding = false
	g.screen = g.previousScreen
}

func (g *Game) updateSettings() {
	if g.awaitingBinding {
		g.updateBindingCapture()
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closeSettings()
		return
	}

	startX, startY, rowH := 200.0, 140.0, 45.0

	for i, action := range actionOrder {
		// Область кнопки (справа от названия действия)
		bindRect := shared.Rect{X: startX + 400, Y: startY + float64(i)*rowH, W: 250, H: 35}

		if uiClick(bindRect) {
			g.settingsSelection = i
			g.awaitingBinding = true
			g.bindingCapturePrimed = false // Нужно для игнорирования текущего клика
			return
		}

		if uiSecondaryClick(bindRect) {
			g.controls.Reset(action)
			g.saveControls(fmt.Sprintf("%s reset to default.", ActionLabel(action)))
		}
	}

	// Кнопка Reset All
	resetAllRect := shared.Rect{X: startX, Y: 620, W: 180, H: 35}
	if uiClick(resetAllRect) {
		g.controls.ResetAll()
		g.saveControls("All controls reset to defaults.")
	}
}

// updateBindingCapture ловит нажатие новой клавиши или кнопки мыши
func (g *Game) updateBindingCapture() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.awaitingBinding = false
		g.settingsStatus = "Binding cancelled."
		return
	}

	// Ждем, пока игрок отпустит кнопку мыши, которой он нажал на "Bind"
	if !g.bindingCapturePrimed {
		if anyBindingSourcePressed() {
			return
		}
		g.bindingCapturePrimed = true
		return
	}

	action := actionOrder[g.settingsSelection]

	// Проверяем кнопки мыши
	mouseButtons := []ebiten.MouseButton{
		ebiten.MouseButtonLeft, ebiten.MouseButtonRight, ebiten.MouseButtonMiddle,
		ebiten.MouseButton3, ebiten.MouseButton4,
	}
	for _, btn := range mouseButtons {
		if inpututil.IsMouseButtonJustPressed(btn) {
			g.controls.Set(action, MouseBinding(btn))
			g.awaitingBinding = false
			g.saveControls(fmt.Sprintf("%s bound to %s.", ActionLabel(action), MouseButtonLabel(btn)))
			return
		}
	}

	// Проверяем клавиатуру
	keys := inpututil.AppendJustPressedKeys(nil)
	for _, k := range keys {
		// Игнорируем системные клавиши
		if k == ebiten.KeyEscape || k == ebiten.KeyF1 {
			continue
		}
		g.controls.Set(action, KeyBinding(k))
		g.awaitingBinding = false
		g.saveControls(fmt.Sprintf("%s bound to %s.", ActionLabel(action), KeyLabel(k)))
		return
	}
}

func (g *Game) saveControls(msg string) {
	if err := g.controls.Save(g.settingsPath); err != nil {
		g.settingsStatus = fmt.Sprintf("Save error: %v", err)
	} else {
		g.settingsStatus = msg
	}
}

func (g *Game) drawSettings(screen *ebiten.Image) {
	// Фон
	vector.DrawFilledRect(screen, 0, 0, float32(shared.ScreenWidth), float32(shared.ScreenHeight), color.RGBA{10, 15, 20, 230}, false)

	panelRect := shared.Rect{X: 150, Y: 50, W: 980, H: 620}
	drawPanel(screen, panelRect, color.RGBA{25, 35, 50, 250}, color.RGBA{70, 110, 180, 255}, 2)

	ebitenutil.DebugPrintAt(screen, "CONTROL SETTINGS", 200, 80)
	ebitenutil.DebugPrintAt(screen, "LEFT CLICK: BIND | RIGHT CLICK: RESET | ESC: BACK", 200, 105)

	startX, startY, rowH := 200.0, 140.0, 45.0

	for i, action := range actionOrder {
		y := startY + float64(i)*rowH

		// Название действия
		ebitenutil.DebugPrintAt(screen, ActionLabel(action), int(startX), int(y)+8)

		// Рисуем кнопку
		bindRect := shared.Rect{X: startX + 400, Y: y, W: 250, H: 35}
		isWaiting := g.awaitingBinding && g.settingsSelection == i

		btnClr := color.RGBA{45, 55, 75, 255}
		if isWaiting {
			btnClr = color.RGBA{150, 100, 20, 255} // Подсвечиваем оранжевым
		} else if uiHover(bindRect) {
			btnClr = color.RGBA{60, 75, 100, 255}
		}

		drawPanel(screen, bindRect, btnClr, color.RGBA{120, 150, 200, 255}, 1)

		label := BindingLabel(g.controls, action)
		if isWaiting {
			label = "> PRESS KEY <"
		}
		ebitenutil.DebugPrintAt(screen, label, int(bindRect.X)+20, int(y)+8)
	}

	// Кнопка сброса всего
	resetAllRect := shared.Rect{X: startX, Y: 620, W: 180, H: 35}
	drawButton(screen, resetAllRect, false, uiHover(resetAllRect))
	ebitenutil.DebugPrintAt(screen, "RESET ALL", int(resetAllRect.X)+55, int(resetAllRect.Y)+8)

	// Статус (результат сохранения)
	ebitenutil.DebugPrintAt(screen, g.settingsStatus, int(startX)+250, 630)
}

func anyBindingSourcePressed() bool {
	// Проверяем стандартные кнопки мыши
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButton3) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButton4) {
		return true
	}
	// Проверяем, нажата ли любая клавиша на клавиатуре
	return len(inpututil.AppendPressedKeys(nil)) > 0
}
