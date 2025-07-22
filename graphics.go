package main

import (
	"bytes"
	"image/color"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

// Global font source for error image generation
var globalFontSource *text.GoTextFaceSource

// InitGraphics initializes the global font source for text rendering
func InitGraphics() error {
	s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		return err
	}
	globalFontSource = s
	return nil
}

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

// CreateErrorImage creates an error placeholder image with filename and error message
func CreateErrorImage(width, height int, filename, errorMsg string) *ebiten.Image {
	// Default size if not specified
	if width <= 0 || height <= 0 {
		width, height = 400, 300
	}

	// Ensure we have a font source
	if globalFontSource == nil {
		// Fallback: create a simple colored rectangle without text
		errorImg := ebiten.NewImage(width, height)
		errorImg.Fill(color.RGBA{120, 30, 30, 255}) // Dark red background

		// Draw white border
		DrawFilledRect(errorImg, 0, 0, float64(width), 3, color.RGBA{255, 255, 255, 255})
		DrawFilledRect(errorImg, 0, float64(height-3), float64(width), 3, color.RGBA{255, 255, 255, 255})
		DrawFilledRect(errorImg, 0, 0, 3, float64(height), color.RGBA{255, 255, 255, 255})
		DrawFilledRect(errorImg, float64(width-3), 0, 3, float64(height), color.RGBA{255, 255, 255, 255})

		return errorImg
	}

	errorImg := ebiten.NewImage(width, height)
	errorImg.Fill(color.RGBA{120, 30, 30, 255}) // Dark red background

	// Create font for error text
	errorFont := &text.GoTextFace{
		Source: globalFontSource,
		Size:   20.0,
	}

	// Draw white border
	DrawFilledRect(errorImg, 0, 0, float64(width), 3, color.RGBA{255, 255, 255, 255})
	DrawFilledRect(errorImg, 0, float64(height-3), float64(width), 3, color.RGBA{255, 255, 255, 255})
	DrawFilledRect(errorImg, 0, 0, 3, float64(height), color.RGBA{255, 255, 255, 255})
	DrawFilledRect(errorImg, float64(width-3), 0, 3, float64(height), color.RGBA{255, 255, 255, 255})

	// Prepare text content
	errorTitle := "ERROR"
	fileText := "File: " + filepath.Base(filename)
	reasonText := "Reason: " + errorMsg

	// Truncate long text to fit within image bounds
	maxChars := (width - 20) / 10 // Rough estimate: 10px per character
	if len(fileText) > maxChars {
		fileText = fileText[:maxChars-3] + "..."
	}
	if len(reasonText) > maxChars {
		reasonText = reasonText[:maxChars-3] + "..."
	}

	// Draw error text
	white := color.RGBA{255, 255, 255, 255}
	DrawText(errorImg, errorTitle, errorFont, 10, 30, white)
	DrawText(errorImg, fileText, errorFont, 10, 60, white)
	DrawText(errorImg, reasonText, errorFont, 10, 90, white)

	return errorImg
}
