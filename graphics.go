package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// DrawText draws text with specified position and color
func DrawText(screen *ebiten.Image, textString string, font *text.GoTextFace, x, y float64, textColor color.RGBA) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(textColor)
	text.Draw(screen, textString, font, op)
}

// DrawFilledRect draws filled rectangles with float64 coordinates
func DrawFilledRect(screen *ebiten.Image, x, y, w, h float64, bgColor color.RGBA) {
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), bgColor, false)
}
