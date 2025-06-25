package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const (
	defaultWidth  = 800
	defaultHeight = 600
	minWidth      = 400
	minHeight     = 300
)

type Config struct {
	WindowWidth  int `json:"window_width"`
	WindowHeight int `json:"window_height"`
}

func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "nv.json"
	}
	return filepath.Join(homeDir, ".nv.json")
}

func loadConfig() Config {
	config := Config{
		WindowWidth:  defaultWidth,
		WindowHeight: defaultHeight,
	}

	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		return config
	}

	json.Unmarshal(data, &config)

	// Validate minimum size
	if config.WindowWidth < minWidth {
		config.WindowWidth = defaultWidth
	}
	if config.WindowHeight < minHeight {
		config.WindowHeight = defaultHeight
	}

	return config
}

func saveConfig(config Config) {
	// Don't save if size is too small
	if config.WindowWidth < minWidth || config.WindowHeight < minHeight {
		return
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(getConfigPath(), data, 0644)
}

func isSupportedExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif":
		return true
	default:
		return false
	}
}

func loadImage(path string) (*ebiten.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %v", path, err)
	}
	return ebiten.NewImageFromImage(img), nil
}

type Game struct {
	paths      []string
	idx        int
	fullscreen bool
	bookMode   bool // Book/spread view mode

	savedWinW int
	savedWinH int
	config    Config

	// Image cache - only keep a few images in memory
	imageCache map[int]*ebiten.Image
}

func (g *Game) getCurrentImage() *ebiten.Image {
	return g.getImageAtIndex(g.idx)
}

func (g *Game) getBookModeImages() (*ebiten.Image, *ebiten.Image) {
	if len(g.paths) == 0 {
		return nil, nil
	}

	// Get left image (current index)
	leftImg := g.getImageAtIndex(g.idx)

	// Get right image (next index, or nil if at the end)
	var rightImg *ebiten.Image
	if g.idx+1 < len(g.paths) {
		rightImg = g.getImageAtIndex(g.idx + 1)
	}

	return leftImg, rightImg
}

func (g *Game) getImageAtIndex(idx int) *ebiten.Image {
	if idx < 0 || idx >= len(g.paths) {
		return nil
	}

	// Check if image is already in cache
	if img, exists := g.imageCache[idx]; exists {
		return img
	}

	// Load image on demand
	img, err := loadImage(g.paths[idx])
	if err != nil {
		log.Printf("failed to load %s: %v", g.paths[idx], err)
		return nil
	}

	// Add to cache
	g.imageCache[idx] = img

	// Clean cache if it gets too large (keep only 4 images for book mode)
	if len(g.imageCache) > 4 {
		g.cleanCache()
	}

	return img
}

func (g *Game) cleanCache() {
	// Keep current, previous, and next images in cache
	keepIndices := make(map[int]bool)
	keepIndices[g.idx] = true

	if g.idx > 0 {
		keepIndices[g.idx-1] = true
	} else if len(g.paths) > 1 {
		keepIndices[len(g.paths)-1] = true // wrap to last
	}

	if g.idx < len(g.paths)-1 {
		keepIndices[g.idx+1] = true
	} else if len(g.paths) > 1 {
		keepIndices[0] = true // wrap to first
	}

	// Remove images not in keep list
	for idx := range g.imageCache {
		if !keepIndices[idx] {
			delete(g.imageCache, idx)
		}
	}
}

func (g *Game) preloadAdjacentImages() {
	if len(g.paths) <= 1 {
		return
	}

	// Preload previous image
	prevIdx := g.idx - 1
	if prevIdx < 0 {
		prevIdx = len(g.paths) - 1
	}
	if _, exists := g.imageCache[prevIdx]; !exists {
		if img, err := loadImage(g.paths[prevIdx]); err == nil {
			g.imageCache[prevIdx] = img
		}
	}

	// Preload next image
	nextIdx := (g.idx + 1) % len(g.paths)
	if _, exists := g.imageCache[nextIdx]; !exists {
		if img, err := loadImage(g.paths[nextIdx]); err == nil {
			g.imageCache[nextIdx] = img
		}
	}
}

func (g *Game) saveCurrentWindowSize() {
	if g.fullscreen {
		// Save the size from before fullscreen
		if g.savedWinW > 0 && g.savedWinH > 0 {
			g.config.WindowWidth = g.savedWinW
			g.config.WindowHeight = g.savedWinH
		}
	} else {
		// Save current window size
		w, h := ebiten.WindowSize()
		g.config.WindowWidth = w
		g.config.WindowHeight = h
	}
	saveConfig(g.config)
}

func (g *Game) Update() error {
	if len(g.paths) == 0 {
		return nil
	}

	// Quit keys
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		g.saveCurrentWindowSize()
		os.Exit(0)
	}

	// Book mode toggle
	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		g.bookMode = !g.bookMode
		// Ensure even index in book mode for proper pairing
		if g.bookMode && g.idx%2 != 0 {
			if g.idx > 0 {
				g.idx--
			}
		}
		g.preloadAdjacentImages()
	}

	// Next / Prev keys
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyN) {
		if g.bookMode {
			// Move by 2 in book mode
			g.idx = (g.idx + 2) % len(g.paths)
		} else {
			g.idx = (g.idx + 1) % len(g.paths)
		}
		g.preloadAdjacentImages() // Preload next images for smooth navigation
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
		if g.bookMode {
			// Move by 2 in book mode
			g.idx -= 2
			if g.idx < 0 {
				// Find the last even index
				lastEvenIdx := len(g.paths) - 1
				if lastEvenIdx%2 != 0 {
					lastEvenIdx--
				}
				g.idx = lastEvenIdx
			}
		} else {
			g.idx--
			if g.idx < 0 {
				g.idx = len(g.paths) - 1
			}
		}
		g.preloadAdjacentImages() // Preload next images for smooth navigation
	}

	// Toggle fullscreen / fit
	if inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		g.fullscreen = !g.fullscreen
		if g.fullscreen {
			g.savedWinW, g.savedWinH = ebiten.WindowSize()
			ebiten.SetFullscreen(true)
		} else {
			ebiten.SetFullscreen(false)
			if g.savedWinW > 0 && g.savedWinH > 0 {
				ebiten.SetWindowSize(g.savedWinW, g.savedWinH)
			}
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	if g.bookMode {
		g.drawBookMode(screen)
	} else {
		g.drawSingleImage(screen)
	}
}

func (g *Game) drawSingleImage(screen *ebiten.Image) {
	img := g.getCurrentImage()
	if img == nil {
		return
	}

	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	var scale float64
	if g.fullscreen {
		scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
	} else {
		if iw > w || ih > h {
			scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
		} else {
			scale = 1
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	sw, sh := float64(iw)*scale, float64(ih)*scale
	op.GeoM.Translate(float64(w)/2-sw/2, float64(h)/2-sh/2)

	screen.DrawImage(img, op)
}

func (g *Game) drawBookMode(screen *ebiten.Image) {
	leftImg, rightImg := g.getBookModeImages()
	if leftImg == nil {
		return
	}

	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	const gap = 10 // Gap between images

	// Calculate available width for each image
	availableWidth := (w - gap) / 2

	// Draw left image
	g.drawImageInRegion(screen, leftImg, 0, 0, availableWidth, h)

	// Draw right image if exists
	if rightImg != nil {
		g.drawImageInRegion(screen, rightImg, availableWidth+gap, 0, availableWidth, h)
	}
}

func (g *Game) drawImageInRegion(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int) {
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()

	var scale float64
	if g.fullscreen {
		scale = math.Min(float64(maxW)/float64(iw), float64(maxH)/float64(ih))
	} else {
		if iw > maxW || ih > maxH {
			scale = math.Min(float64(maxW)/float64(iw), float64(maxH)/float64(ih))
		} else {
			scale = 1
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	sw, sh := float64(iw)*scale, float64(ih)*scale
	op.GeoM.Translate(float64(x)+float64(maxW)/2-sw/2, float64(y)+float64(maxH)/2-sh/2)

	screen.DrawImage(img, op)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func collectImages(args []string) ([]string, error) {
	var list []string
	for _, p := range args {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			err := filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if fi.IsDir() {
					return nil
				}
				if isSupportedExt(path) {
					list = append(list, path)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		} else {
			if isSupportedExt(p) {
				list = append(list, p)
			}
		}
	}
	sort.Strings(list)
	return list, nil
}

func main() {
	flag.Parse()
	paths, err := collectImages(flag.Args())
	if err != nil {
		log.Fatal(err)
	}
	if len(paths) == 0 {
		log.Fatal("no image files specified")
	}

	config := loadConfig()

	g := &Game{
		paths:      paths,
		idx:        0,
		config:     config,
		imageCache: make(map[int]*ebiten.Image),
	}

	// Preload the first image and adjacent ones for faster startup
	g.preloadAdjacentImages()

	ebiten.SetWindowTitle("Ebiten Image Viewer")
	ebiten.SetWindowSize(config.WindowWidth, config.WindowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
