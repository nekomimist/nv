package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/bodgit/sevenzip"
	"github.com/hajimehoshi/ebiten/v2"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/nwaples/rardecode"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

type ImagePath struct {
	Path        string // Local file path or archive:entry format
	ArchivePath string // Empty for regular files, path to archive for entries
	EntryPath   string // Empty for regular files, path within archive for entries
}

// NavigationDirection represents the direction of navigation
type NavigationDirection int

const (
	NavigationForward NavigationDirection = iota
	NavigationBackward
	NavigationJump
)

// PreloadRequest represents a request to preload an image
type PreloadRequest struct {
	Index     int
	Direction NavigationDirection
}

// PreloadStats provides statistics about preloading
type PreloadStats struct {
	QueueSize     int
	LoadedCount   int
	FailedCount   int
	LastDirection NavigationDirection
}

// PreloadManager manages asynchronous image preloading
type PreloadManager struct {
	requestChan  chan PreloadRequest
	ctx          context.Context
	cancel       context.CancelFunc
	imageManager *DefaultImageManager
	mu           sync.RWMutex
	stats        PreloadStats
	maxPreload   int
	enabled      bool
}

// NewPreloadManager creates a new PreloadManager
func NewPreloadManager(imageManager *DefaultImageManager, maxPreload int) *PreloadManager {
	ctx, cancel := context.WithCancel(context.Background())
	pm := &PreloadManager{
		requestChan:  make(chan PreloadRequest, 100),
		ctx:          ctx,
		cancel:       cancel,
		imageManager: imageManager,
		maxPreload:   maxPreload,
		enabled:      true,
	}

	// Start worker goroutine
	go pm.worker()

	return pm
}

// SetEnabled enables or disables preloading
func (pm *PreloadManager) SetEnabled(enabled bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.enabled = enabled
}

// IsEnabled returns whether preloading is enabled
func (pm *PreloadManager) IsEnabled() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.enabled
}

// GetStats returns current preload statistics
func (pm *PreloadManager) GetStats() PreloadStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.stats
}

// Stop stops the preload manager
func (pm *PreloadManager) Stop() {
	pm.cancel()
}

// StartPreload starts preloading images from the current index in the specified direction
func (pm *PreloadManager) StartPreload(currentIdx int, direction NavigationDirection) {
	if !pm.IsEnabled() {
		return
	}

	// Clear the request channel to cancel any pending requests
drain:
	for {
		select {
		case <-pm.requestChan:
			// discard pending requests
		default:
			break drain
		}
	}

	// Send new preload request
	select {
	case pm.requestChan <- PreloadRequest{Index: currentIdx, Direction: direction}:
	default:
		// Channel is full, skip this request
		debugLog("Preload request channel full, skipping preload request")
	}
}

// worker runs the preload worker goroutine
func (pm *PreloadManager) worker() {
	for {
		select {
		case <-pm.ctx.Done():
			return
		case req := <-pm.requestChan:
			if pm.IsEnabled() {
				pm.processPreloadRequest(req)
			}
		}
	}
}

// processPreloadRequest processes a single preload request
func (pm *PreloadManager) processPreloadRequest(req PreloadRequest) {
	pm.mu.Lock()
	pm.stats.LastDirection = req.Direction
	pm.mu.Unlock()

	pathsCount := pm.imageManager.GetPathsCount()
	if pathsCount == 0 {
		return
	}

	indices := pm.calculatePreloadIndices(req.Index, req.Direction, pathsCount)

	for _, idx := range indices {
		select {
		case <-pm.ctx.Done():
			return
		default:
			pm.preloadImage(idx)
		}
	}
}

// calculatePreloadIndices calculates which image indices to preload
func (pm *PreloadManager) calculatePreloadIndices(currentIdx int, direction NavigationDirection, pathsCount int) []int {
	var indices []int

	switch direction {
	case NavigationForward:
		// Preload forward
		for i := 1; i <= pm.maxPreload; i++ {
			idx := currentIdx + i
			if idx < pathsCount {
				indices = append(indices, idx)
			}
		}
	case NavigationBackward:
		// Preload backward
		for i := 1; i <= pm.maxPreload; i++ {
			idx := currentIdx - i
			if idx >= 0 {
				indices = append(indices, idx)
			}
		}
	case NavigationJump:
		// Preload both directions from jump point
		half := pm.maxPreload / 2

		// Forward
		for i := 1; i <= half; i++ {
			idx := currentIdx + i
			if idx < pathsCount {
				indices = append(indices, idx)
			}
		}

		// Backward
		for i := 1; i <= half; i++ {
			idx := currentIdx - i
			if idx >= 0 {
				indices = append(indices, idx)
			}
		}
	}

	return indices
}

// preloadImage loads a single image into cache if not already cached
func (pm *PreloadManager) preloadImage(idx int) {
	if idx < 0 || idx >= pm.imageManager.GetPathsCount() {
		return
	}

	imagePath, ok := pm.imageManager.getPath(idx)
	if !ok {
		return
	}
	cacheKey := imagePath.Path

	// Check if already in cache
	if _, ok := pm.imageManager.cache.Get(cacheKey); ok {
		return // Already cached
	}

	// Load image
	img, err := loadImage(imagePath)
	if err != nil {
		pm.mu.Lock()
		pm.stats.FailedCount++
		pm.mu.Unlock()
		debugLog("Preload failed for [%d] %s: %v", idx+1, imagePath.Path, err)

		// Create error image for cache instead of skipping
		img = CreateErrorImage(400, 300, imagePath.Path, err.Error())
	}

	// Add to cache
	pm.imageManager.cache.Add(cacheKey, img)

	pm.mu.Lock()
	pm.stats.LoadedCount++
	pm.mu.Unlock()

	debugLog("Preloaded [%d] %s (cache: %d items)", idx+1, imagePath.Path, pm.imageManager.cache.Len())
}

// ImageManager interface for managing image loading and caching
type ImageManager interface {
	GetImage(idx int) *ebiten.Image
	GetCurrentImage(idx int) *ebiten.Image
	GetBookModeImages(idx int, rightToLeft bool) (*ebiten.Image, *ebiten.Image)
	SetPaths(paths []ImagePath)
	GetPathsCount() int
	StartPreload(currentIdx int, direction NavigationDirection)
	StopPreload()
	GetPreloadStats() PreloadStats
}

// DefaultImageManager implements ImageManager
type DefaultImageManager struct {
	paths          []ImagePath
	cache          *lru.Cache[string, *ebiten.Image]
	mu             sync.RWMutex
	preloadManager *PreloadManager
}

// NewImageManager creates a new DefaultImageManager
func NewImageManager(cacheSize int) ImageManager {
	cache, err := lru.NewWithEvict[string, *ebiten.Image](cacheSize, func(_ string, img *ebiten.Image) {
		if img != nil {
			img.Deallocate()
		}
	})
	if err != nil {
		log.Printf("Error: Failed to create LRU cache: %v", err)
		cache, _ = lru.NewWithEvict[string, *ebiten.Image](16, func(_ string, img *ebiten.Image) {
			if img != nil {
				img.Deallocate()
			}
		})
	}

	manager := &DefaultImageManager{
		paths: []ImagePath{},
		cache: cache,
	}

	return manager
}

// NewImageManagerWithPreload creates a new DefaultImageManager with preload configuration
func NewImageManagerWithPreload(cacheSize int, preloadCount int, preloadEnabled bool) ImageManager {
	cache, err := lru.NewWithEvict[string, *ebiten.Image](cacheSize, func(_ string, img *ebiten.Image) {
		if img != nil {
			img.Deallocate()
		}
	})
	if err != nil {
		log.Printf("Error: Failed to create LRU cache: %v", err)
		cache, _ = lru.NewWithEvict[string, *ebiten.Image](16, func(_ string, img *ebiten.Image) {
			if img != nil {
				img.Deallocate()
			}
		})
	}

	manager := &DefaultImageManager{
		paths: []ImagePath{},
		cache: cache,
	}

	// Initialize preload manager with configuration
	manager.preloadManager = NewPreloadManager(manager, preloadCount)
	manager.preloadManager.SetEnabled(preloadEnabled)

	return manager
}

func (m *DefaultImageManager) SetPaths(paths []ImagePath) {
	m.mu.Lock()
	m.paths = paths
	m.mu.Unlock()
	// No need to clear cache since we use file paths as keys
	debugLog("SetPaths: %d new paths, cache preserved (%d items)", len(paths), m.cache.Len())
}

func (m *DefaultImageManager) GetPathsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.paths)
}

func (m *DefaultImageManager) StartPreload(currentIdx int, direction NavigationDirection) {
	if m.preloadManager != nil {
		m.preloadManager.StartPreload(currentIdx, direction)
	}
}

func (m *DefaultImageManager) StopPreload() {
	if m.preloadManager != nil {
		m.preloadManager.Stop()
	}
}

func (m *DefaultImageManager) GetPreloadStats() PreloadStats {
	if m.preloadManager != nil {
		return m.preloadManager.GetStats()
	}
	return PreloadStats{}
}

func (m *DefaultImageManager) GetCurrentImage(idx int) *ebiten.Image {
	return m.GetImage(idx)
}

func (m *DefaultImageManager) GetBookModeImages(idx int, rightToLeft bool) (*ebiten.Image, *ebiten.Image) {
	var leftImg, rightImg *ebiten.Image

	if rightToLeft {
		// Right-to-left reading (Japanese manga style): [next][current]
		leftImg = m.GetImage(idx + 1) // Next image on left
		rightImg = m.GetImage(idx)    // Current image on right
	} else {
		// Left-to-right reading (Western style): [current][next]
		leftImg = m.GetImage(idx)      // Current image on left
		rightImg = m.GetImage(idx + 1) // Next image on right (nil if OOB)
	}

	return leftImg, rightImg
}

func (m *DefaultImageManager) GetImage(idx int) *ebiten.Image {
	m.mu.RLock()
	if idx < 0 || idx >= len(m.paths) {
		m.mu.RUnlock()
		return nil
	}
	imagePath := m.paths[idx]
	m.mu.RUnlock()
	cacheKey := imagePath.Path

	// Check if image is already in cache
	img, ok := m.cache.Get(cacheKey)
	if ok {
		debugLog("Cache HIT: %s (cache: %d items)", cacheKey, m.cache.Len())
		return img
	}

	// Load image on demand
	img, err := loadImage(imagePath)
	if err != nil {
		log.Printf("Error: Failed to load image [%d/%d] %s: %v",
			idx+1, len(m.paths), imagePath.Path, err)

		// Create error image instead of returning nil
		return CreateErrorImage(400, 300, imagePath.Path, err.Error())
	}

	// Add to cache
	m.cache.Add(cacheKey, img)

	// Log cache miss with memory info
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	debugLog("Cache MISS: %s, loaded and cached (cache: %d items, memory: %dMB)",
		cacheKey, m.cache.Len(), mem.Alloc/1024/1024)

	return img
}

// getPath safely returns the ImagePath at index if available
func (m *DefaultImageManager) getPath(idx int) (ImagePath, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if idx < 0 || idx >= len(m.paths) {
		return ImagePath{}, false
	}
	return m.paths[idx], true
}

// cache operations are goroutine-safe via golang-lru; no extra locking needed

// Image loading functions

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

func loadImageFrom7z(archivePath, entryPath string) (*ebiten.Image, error) {
	r, err := sevenzip.OpenReader(archivePath)
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
		case ".7z":
			return loadImageFrom7z(imagePath.ArchivePath, imagePath.EntryPath)
		default:
			return nil, fmt.Errorf("unsupported archive format: %s", ext)
		}
	}
}

// File collection functions

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

func extractImagesFrom7z(archivePath string) ([]ImagePath, error) {
	r, err := sevenzip.OpenReader(archivePath)
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
	case ".7z":
		archiveImages, err = extractImagesFrom7z(archivePath)
	default:
		return []ImagePath{}, fmt.Errorf("unsupported archive format: %s", ext)
	}

	if err != nil {
		log.Printf("Error: Failed to process archive %s: %v", archivePath, err)
		return []ImagePath{}, err
	}

	return archiveImages, nil
}

// sortImagePaths sorts the given image paths using the specified sort strategy.
// Returns a new sorted slice without modifying the original.
func sortImagePaths(images []ImagePath, sortMethod int) []ImagePath {
	strategy := GetSortStrategy(sortMethod)
	return strategy.Sort(images)
}

// collectImagesFromSameDirectory collects image files from the same directory as the given file
// Does not include archives or subdirectories - only image files in the same directory
func collectImagesFromSameDirectory(filePath string, sortMethod int) ([]ImagePath, error) {
	// Get the directory of the file
	dir := filepath.Dir(filePath)

	// Read directory contents
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %v", dir, err)
	}

	var images []ImagePath
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories
		}

		fullPath := filepath.Join(dir, entry.Name())

		// Only collect image files, not archives
		if isSupportedExt(fullPath) {
			images = append(images, ImagePath{
				Path:        fullPath,
				ArchivePath: "",
				EntryPath:   "",
			})
		}
	}

	// Sort the images
	sortedImages := sortImagePaths(images, sortMethod)
	return sortedImages, nil
}

func collectImages(args []string, sortMethod int) ([]ImagePath, error) {
	var list []ImagePath
	for _, p := range args {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			var dirImages []ImagePath
			err := filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if fi.IsDir() {
					return nil
				}
				if isSupportedExt(path) {
					dirImages = append(dirImages, ImagePath{
						Path:        path,
						ArchivePath: "",
						EntryPath:   "",
					})
				} else if isArchiveExt(path) {
					archiveImages, err := processArchive(path)
					if err == nil {
						sortedArchiveImages := sortImagePaths(archiveImages, sortMethod)
						dirImages = append(dirImages, sortedArchiveImages...)
					} else {
						log.Printf("Warning: Skipping problematic archive %s: %v", path, err)
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			sortedDirImages := sortImagePaths(dirImages, sortMethod)
			list = append(list, sortedDirImages...)
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
					sortedArchiveImages := sortImagePaths(archiveImages, sortMethod)
					list = append(list, sortedArchiveImages...)
				} else {
					log.Printf("Warning: Skipping problematic archive %s: %v", p, err)
				}
			}
		}
	}

	return list, nil
}
