package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/nwaples/rardecode"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const (
	// Book mode layout constants
	imageGap = 10 // Gap between images in book mode

	// Aspect ratio thresholds
	minAspectRatio = 0.4 // Extremely tall images
	maxAspectRatio = 2.5 // Extremely wide images

	// Cache size limits
	maxCacheSize = 4 // Maximum number of images to keep in cache
)


type ImagePath struct {
	Path        string // Local file path or archive:entry format
	ArchivePath string // Empty for regular files, path to archive for entries
	EntryPath   string // Empty for regular files, path within archive for entries
}

func isArchiveExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip", ".rar":
		return true
	default:
		return false
	}
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

func loadImageFromBytes(data []byte, path string) (*ebiten.Image, error) {
	reader := bytes.NewReader(data)
	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %v", path, err)
	}
	return ebiten.NewImageFromImage(img), nil
}

func loadImageFromZip(archivePath, entryPath string) (*ebiten.Image, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == entryPath {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			return loadImageFromBytes(data, entryPath)
		}
	}
	return nil, fmt.Errorf("entry %s not found in %s", entryPath, archivePath)
}

func loadImageFromRar(archivePath, entryPath string) (*ebiten.Image, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r, err := rardecode.NewReader(f, "")
	if err != nil {
		return nil, err
	}

	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == entryPath {
			data, err := io.ReadAll(r)
			if err != nil {
				return nil, err
			}
			return loadImageFromBytes(data, entryPath)
		}
	}
	return nil, fmt.Errorf("entry %s not found in %s", entryPath, archivePath)
}

func loadImage(imagePath ImagePath) (*ebiten.Image, error) {
	if imagePath.ArchivePath == "" {
		// Regular file
		f, err := os.Open(imagePath.Path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		img, _, err := image.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decoding %s: %v", imagePath.Path, err)
		}
		return ebiten.NewImageFromImage(img), nil
	} else {
		// Archive entry
		ext := strings.ToLower(filepath.Ext(imagePath.ArchivePath))
		switch ext {
		case ".zip":
			return loadImageFromZip(imagePath.ArchivePath, imagePath.EntryPath)
		case ".rar":
			return loadImageFromRar(imagePath.ArchivePath, imagePath.EntryPath)
		default:
			return nil, fmt.Errorf("unsupported archive format: %s", ext)
		}
	}
}

type Game struct {
	paths      []ImagePath
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

	var leftImg, rightImg *ebiten.Image

	if g.config.RightToLeft {
		// Right-to-left reading (Japanese manga style): [next][current]
		leftImg = g.getImageAtIndex(g.idx + 1) // Next image on left
		rightImg = g.getImageAtIndex(g.idx)    // Current image on right
	} else {
		// Left-to-right reading (Western style): [current][next]
		leftImg = g.getImageAtIndex(g.idx) // Current image on left
		if g.idx+1 < len(g.paths) {
			rightImg = g.getImageAtIndex(g.idx + 1) // Next image on right
		}
	}

	return leftImg, rightImg
}

func (g *Game) shouldUseBookMode(leftImg, rightImg *ebiten.Image) bool {
	if leftImg == nil || rightImg == nil {
		return false // Can't do book mode with only one image
	}

	// Calculate aspect ratios
	leftAspect := float64(leftImg.Bounds().Dx()) / float64(leftImg.Bounds().Dy())
	rightAspect := float64(rightImg.Bounds().Dx()) / float64(rightImg.Bounds().Dy())

	// Check for extremely tall or wide images (should be single page)
	if leftAspect < minAspectRatio || leftAspect > maxAspectRatio ||
		rightAspect < minAspectRatio || rightAspect > maxAspectRatio {
		return false
	}

	// Calculate the ratio between the two aspect ratios
	aspectRatio := leftAspect / rightAspect
	if aspectRatio < 1.0 {
		aspectRatio = 1.0 / aspectRatio // Always use the larger ratio
	}

	// Use single page if aspect ratios are too different
	return aspectRatio <= g.config.AspectRatioThreshold
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
		log.Printf("failed to load %s: %v", g.paths[idx].Path, err)
		return nil
	}

	// Add to cache
	g.imageCache[idx] = img

	// Clean cache if it gets too large
	if len(g.imageCache) > maxCacheSize {
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

	g.handleExitKeys()
	g.handleModeToggleKeys()
	g.handleNavigationKeys()
	g.handleFullscreenToggle()

	return nil
}

func (g *Game) handleExitKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		g.saveCurrentWindowSize()
		os.Exit(0)
	}
}

func (g *Game) handleModeToggleKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			// SHIFT+B: Toggle reading direction
			g.config.RightToLeft = !g.config.RightToLeft
			saveConfig(g.config)
		} else {
			// B: Toggle book mode
			g.bookMode = !g.bookMode
			// Ensure even index in book mode for proper pairing
			if g.bookMode && g.idx%2 != 0 {
				if g.idx > 0 {
					g.idx--
				}
			}
			g.preloadAdjacentImages()
		}
	}
}

func (g *Game) handleNavigationKeys() {
	// Next page
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyN) {
		g.navigateNext()
		g.preloadAdjacentImages()
	}
	// Previous page
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.navigatePrevious()
		g.preloadAdjacentImages()
	}
}

func (g *Game) navigateNext() {
	if g.bookMode && !ebiten.IsKeyPressed(ebiten.KeyShift) {
		// Move by 2 in book mode (normal spread navigation)
		g.idx = (g.idx + 2) % len(g.paths)
	} else {
		// Move by 1 (single page mode or SHIFT+key for fine adjustment)
		g.idx = (g.idx + 1) % len(g.paths)
	}
}

func (g *Game) navigatePrevious() {
	if g.bookMode && !ebiten.IsKeyPressed(ebiten.KeyShift) {
		// Move by 2 in book mode (normal spread navigation)
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
		// Move by 1 (single page mode or SHIFT+key for fine adjustment)
		g.idx--
		if g.idx < 0 {
			g.idx = len(g.paths) - 1
		}
	}
}

func (g *Game) handleFullscreenToggle() {
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

	// Check if images are compatible for book mode display
	if !g.shouldUseBookMode(leftImg, rightImg) {
		// Fall back to single page display
		g.drawSingleImage(screen)
		return
	}

	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Calculate available width for each image
	availableWidth := (w - imageGap) / 2

	// Draw left image (right-aligned within its region)
	g.drawBookImageInRegion(screen, leftImg, 0, 0, availableWidth, h, "right")

	// Draw right image if exists (left-aligned within its region)
	if rightImg != nil {
		rightX := availableWidth + imageGap
		g.drawBookImageInRegion(screen, rightImg, rightX, 0, availableWidth, h, "left")
	}
}

func (g *Game) drawImageInRegion(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int) {
	g.drawImageInRegionWithAlign(screen, img, x, y, maxW, maxH, "center")
}

func (g *Game) drawBookImageInRegion(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int, align string) {
	g.drawImageInRegionWithAlign(screen, img, x, y, maxW, maxH, align)
}

func (g *Game) drawImageInRegionWithAlign(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int, align string) {
	// Calculate scaling
	scale := g.calculateImageScale(img, maxW, maxH)

	// Create draw options
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)

	// Calculate position based on alignment
	scaledW := float64(img.Bounds().Dx()) * scale
	scaledH := float64(img.Bounds().Dy()) * scale

	xPos := g.calculateHorizontalPosition(x, maxW, scaledW, align)
	yPos := float64(y) + float64(maxH)/2 - scaledH/2 // Always center vertically

	op.GeoM.Translate(xPos, yPos)
	screen.DrawImage(img, op)
}

func (g *Game) calculateImageScale(img *ebiten.Image, maxW, maxH int) float64 {
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()

	if g.fullscreen {
		return math.Min(float64(maxW)/float64(iw), float64(maxH)/float64(ih))
	}

	// In windowed mode, don't scale up small images
	if iw > maxW || ih > maxH {
		return math.Min(float64(maxW)/float64(iw), float64(maxH)/float64(ih))
	}
	return 1
}

func (g *Game) calculateHorizontalPosition(x, maxW int, scaledW float64, align string) float64 {
	switch align {
	case "left":
		return float64(x)
	case "right":
		return float64(x+maxW) - scaledW
	default: // "center"
		return float64(x) + float64(maxW)/2 - scaledW/2
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func extractImagesFromZip(archivePath string) ([]ImagePath, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var images []ImagePath
	for _, f := range r.File {
		if !f.FileInfo().IsDir() && isSupportedExt(f.Name) {
			images = append(images, ImagePath{
				Path:        archivePath + ":" + f.Name,
				ArchivePath: archivePath,
				EntryPath:   f.Name,
			})
		}
	}
	return images, nil
}

func extractImagesFromRar(archivePath string) ([]ImagePath, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r, err := rardecode.NewReader(f, "")
	if err != nil {
		return nil, err
	}

	var images []ImagePath
	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if !header.IsDir && isSupportedExt(header.Name) {
			images = append(images, ImagePath{
				Path:        archivePath + ":" + header.Name,
				ArchivePath: archivePath,
				EntryPath:   header.Name,
			})
		}
	}
	return images, nil
}

func processArchive(archivePath string) ([]ImagePath, error) {
	if !isArchiveExt(archivePath) {
		return []ImagePath{}, nil
	}

	var archiveImages []ImagePath
	var err error

	ext := strings.ToLower(filepath.Ext(archivePath))
	switch ext {
	case ".zip":
		archiveImages, err = extractImagesFromZip(archivePath)
	case ".rar":
		archiveImages, err = extractImagesFromRar(archivePath)
	default:
		return []ImagePath{}, fmt.Errorf("unsupported archive format: %s", ext)
	}

	if err != nil {
		log.Printf("Error reading archive %s: %v", archivePath, err)
		return []ImagePath{}, err
	}

	return archiveImages, nil
}

func collectImages(args []string) ([]ImagePath, error) {
	var list []ImagePath
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
					list = append(list, ImagePath{
						Path:        path,
						ArchivePath: "",
						EntryPath:   "",
					})
				} else if isArchiveExt(path) {
					archiveImages, err := processArchive(path)
					if err == nil {
						list = append(list, archiveImages...)
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		} else {
			if isSupportedExt(p) {
				list = append(list, ImagePath{
					Path:        p,
					ArchivePath: "",
					EntryPath:   "",
				})
			} else if isArchiveExt(p) {
				archiveImages, err := processArchive(p)
				if err == nil {
					list = append(list, archiveImages...)
				}
			}
		}
	}

	// Sort by path for consistent ordering
	sort.Slice(list, func(i, j int) bool {
		return list[i].Path < list[j].Path
	})
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
