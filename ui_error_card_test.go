package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

func testUIFace(t *testing.T, size float64) text.Face {
	t.Helper()
	source, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	return &text.GoTextFace{Source: source, Size: size}
}

func TestPureWrapTextLines(t *testing.T) {
	face := testUIFace(t, 20)
	const maxWidth = 200
	longPath := "/mnt/c/Users/someone/Pictures/" + strings.Repeat("very_long_directory_name/", 4) + "page.png"

	tests := []struct {
		name      string
		input     string
		maxLines  int
		wantLines int
		wantCut   bool
	}{
		{name: "short text", input: "Reason: bad data", maxLines: 5, wantLines: 1},
		{name: "blank line kept", input: "Source: a\n\nReason: b", maxLines: 5, wantLines: 3},
		{name: "path without spaces breaks", input: "Source: " + longPath, maxLines: 20},
		{name: "line limit truncates", input: "Source: " + longPath, maxLines: 2, wantLines: 2, wantCut: true},
		{name: "no room", input: "Reason: b", maxLines: 0, wantLines: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapTextLines(tt.input, maxWidth, tt.maxLines, face)
			if tt.maxLines == 0 {
				if got != "" {
					t.Fatalf("wrapTextLines() = %q, want empty", got)
				}
				return
			}
			lines := strings.Split(got, "\n")
			if tt.wantLines > 0 && len(lines) != tt.wantLines {
				t.Fatalf("wrapTextLines() = %d lines %q, want %d", len(lines), lines, tt.wantLines)
			}
			if len(lines) > tt.maxLines {
				t.Fatalf("wrapTextLines() = %d lines, want at most %d", len(lines), tt.maxLines)
			}
			for _, line := range lines {
				if width, _ := text.Measure(line, face, 0); width > maxWidth {
					t.Fatalf("line %q is %.1fpx wide, want at most %d", line, width, maxWidth)
				}
			}
			if cut := strings.HasSuffix(got, "…"); cut != tt.wantCut {
				t.Fatalf("truncated = %v, want %v: %q", cut, tt.wantCut, got)
			}
			// Line breaks may replace spaces; the visible characters must survive.
			if !tt.wantCut && strings.Join(strings.Fields(got), "") != strings.Join(strings.Fields(tt.input), "") {
				t.Fatalf("wrapping changed the text: %q", got)
			}
		})
	}
}

func TestGUI_ErrorCardUsesLabelsAndRebuildsOnlyWhenKeyChanges(t *testing.T) {
	cfg := defaultConfig()
	g := &Game{config: cfg, pendingConfig: cfg}
	c := NewUIController(g)
	g.uiController = c
	g.displayContent = &DisplayContent{
		LeftImage: newFailedDisplayImage("/images/broken.webp", errors.New("unsupported bitstream")),
		Metadata:  DisplayMetadata{LeftPage: 3, ActualImages: 1},
	}
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	c.Update()
	c.Draw(screen, nil)

	window := c.errorWindows[0].window
	if window == nil {
		t.Fatal("failed page did not get an error card")
	}
	// Anything scrollable would carry screen-sized render buffers.
	labels := 0
	var walk func(*widget.Container)
	walk = func(container *widget.Container) {
		for _, child := range container.Children() {
			switch child := child.(type) {
			case *widget.Container:
				walk(child)
			case *widget.Label:
				labels++
			default:
				t.Fatalf("error card contains %T, want only containers and labels", child)
			}
		}
	}
	root, ok := window.GetContainer().(*widget.Container)
	if !ok {
		t.Fatalf("error card root is %T, want *widget.Container", window.GetContainer())
	}
	walk(root)
	if labels != 2 {
		t.Fatalf("error card has %d labels, want title and details", labels)
	}

	c.Draw(screen, nil)
	if c.errorWindows[0].window != window {
		t.Fatal("redraw with an unchanged failure rebuilt the error card")
	}

	// Cards are capped at 640x360, so only a screen below that cap changes
	// the card size; a larger screen must keep the existing card.
	larger := ebiten.NewImage(1920, 1080)
	t.Cleanup(larger.Deallocate)
	c.Draw(larger, nil)
	if c.errorWindows[0].window != window {
		t.Fatal("a moved card with the same size was rebuilt")
	}
	smaller := ebiten.NewImage(480, 320)
	t.Cleanup(smaller.Deallocate)
	c.Draw(smaller, nil)
	if c.errorWindows[0].window == window {
		t.Fatal("a resized card kept text wrapped for the old size")
	}
}
