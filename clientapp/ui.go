package clientapp

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"warpedrealms/shared"
)

func mousePosition() shared.Vec2 {
	x, y := ebiten.CursorPosition()
	return shared.Vec2{X: float64(x), Y: float64(y)}
}

func uiHover(rect shared.Rect) bool {
	return rect.ContainsPoint(mousePosition())
}

func uiClick(rect shared.Rect) bool {
	return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && uiHover(rect)
}

func uiSecondaryClick(rect shared.Rect) bool {
	return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) && uiHover(rect)
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
