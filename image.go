package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bodgit/sevenzip"
	"github.com/hajimehoshi/ebiten/v2"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/nwaples/rardecode"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
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

const defaultMaxImageDimension = 8192

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

// SetMaxPreload updates the max number of images to preload
func (pm *PreloadManager) SetMaxPreload(n int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if n < 0 {
		n = 0
	}
	pm.maxPreload = n
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

func (pm *PreloadManager) updateQueueSize(queueSize int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.stats.QueueSize = queueSize
}

func (pm *PreloadManager) recordResult(success bool, queueSize int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.stats.QueueSize = queueSize
	if success {
		pm.stats.LoadedCount++
		return
	}
	pm.stats.FailedCount++
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

	pm.imageManager.requestPreload(imagePath)
	pm.updateQueueSize(len(pm.imageManager.preloadRequests))
}

// ImageManager interface for managing image loading and caching
type ImageManager interface {
	GetImage(idx int) *ebiten.Image
	GetBookModeImages(idx int, rightToLeft bool) (*ebiten.Image, *ebiten.Image)
	GetPath(idx int) (ImagePath, bool)
	SetPaths(paths []ImagePath)
	GetPathsCount() int
	StartPreload(currentIdx int, direction NavigationDirection)
	StopPreload()
	GetPreloadStats() PreloadStats
	ConsumeAsyncRefresh() bool
}

// DefaultImageManager implements ImageManager
type DefaultImageManager struct {
	paths              []ImagePath
	cache              *lru.Cache[string, *ebiten.Image]
	mu                 sync.RWMutex
	preloadManager     *PreloadManager
	maxImageDimension  atomic.Int64
	loadRequests       chan loadRequest
	preloadRequests    chan loadRequest
	inflight           map[string]struct{}
	inflightMu         sync.Mutex
	loadCtx            context.Context
	loadCancel         context.CancelFunc
	loadWorkerOnce     sync.Once
	loadingPlaceholder *ebiten.Image
	asyncRefresh       atomic.Bool
}

type loadRequest struct {
	path     ImagePath
	cacheKey string
	preload  bool
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

	return newDefaultImageManager(cache)
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

	manager := newDefaultImageManager(cache)

	// Initialize preload manager with configuration
	manager.preloadManager = NewPreloadManager(manager, preloadCount)
	manager.preloadManager.SetEnabled(preloadEnabled)

	return manager
}

func newDefaultImageManager(cache *lru.Cache[string, *ebiten.Image]) *DefaultImageManager {
	loadCtx, loadCancel := context.WithCancel(context.Background())
	manager := &DefaultImageManager{
		paths:              []ImagePath{},
		cache:              cache,
		loadRequests:       make(chan loadRequest, 8),
		preloadRequests:    make(chan loadRequest, 8),
		inflight:           make(map[string]struct{}),
		loadCtx:            loadCtx,
		loadCancel:         loadCancel,
		loadingPlaceholder: createLoadingPlaceholder(),
	}
	manager.startLoadWorker()
	return manager
}

// SetMaxImageDimension updates the maximum permitted decoded image dimension before handing to Ebiten.
// A value of 0 disables pre-scaling and relies on Ebiten's own limits.
func (m *DefaultImageManager) SetMaxImageDimension(limit int) {
	if limit < 0 {
		limit = 0
	}
	m.maxImageDimension.Store(int64(limit))
}

func (m *DefaultImageManager) startLoadWorker() {
	m.loadWorkerOnce.Do(func() {
		go m.asyncLoadWorker()
	})
}

func (m *DefaultImageManager) asyncLoadWorker() {
	for {
		select {
		case <-m.loadCtx.Done():
			return
		default:
		}

		select {
		case req := <-m.loadRequests:
			m.processLoadRequest(req)
		default:
			select {
			case <-m.loadCtx.Done():
				return
			case req := <-m.loadRequests:
				m.processLoadRequest(req)
			case req := <-m.preloadRequests:
				m.processLoadRequest(req)
			}
		}
	}
}

func (m *DefaultImageManager) processLoadRequest(req loadRequest) {
	defer func() {
		m.inflightMu.Lock()
		delete(m.inflight, req.cacheKey)
		m.inflightMu.Unlock()
	}()

	img, err := m.loadImage(req.path)
	if err != nil {
		log.Printf("Error: Failed to load image %s: %v", req.path.Path, err)
		errorImg := CreateErrorImage(400, 300, req.path.Path, err.Error())
		m.cache.Add(req.cacheKey, errorImg)
		m.asyncRefresh.Store(true)
		m.recordPreloadResult(req.preload, false)
		return
	}

	m.cache.Add(req.cacheKey, img)
	m.asyncRefresh.Store(true)
	m.recordPreloadResult(req.preload, true)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	debugLog("Cache MISS (async): %s, loaded and cached (cache: %d items, memory: %dMB)", req.cacheKey, m.cache.Len(), mem.Alloc/1024/1024)
}

func (m *DefaultImageManager) requestAsyncLoad(imagePath ImagePath) {
	m.enqueueLoadRequest(imagePath, false)
}

func (m *DefaultImageManager) requestPreload(imagePath ImagePath) {
	m.enqueueLoadRequest(imagePath, true)
}

func (m *DefaultImageManager) enqueueLoadRequest(imagePath ImagePath, preload bool) {
	cacheKey := imagePath.Path
	if _, ok := m.cache.Get(cacheKey); ok {
		return
	}

	m.inflightMu.Lock()
	if _, exists := m.inflight[cacheKey]; exists {
		m.inflightMu.Unlock()
		return
	}
	m.inflight[cacheKey] = struct{}{}
	m.inflightMu.Unlock()

	req := loadRequest{path: imagePath, cacheKey: cacheKey, preload: preload}
	queue := m.loadRequests
	queueName := "async"
	if preload {
		queue = m.preloadRequests
		queueName = "preload"
	}

	select {
	case <-m.loadCtx.Done():
		m.clearInflight(cacheKey)
	case queue <- req:
		m.updatePreloadQueueSize()
	default:
		m.clearInflight(cacheKey)
		debugLog("%s load queue full, skipping request for %s", queueName, cacheKey)
	}
}

func (m *DefaultImageManager) clearInflight(cacheKey string) {
	m.inflightMu.Lock()
	delete(m.inflight, cacheKey)
	m.inflightMu.Unlock()
}

func (m *DefaultImageManager) updatePreloadQueueSize() {
	if m.preloadManager == nil {
		return
	}
	m.preloadManager.updateQueueSize(len(m.preloadRequests))
}

func (m *DefaultImageManager) recordPreloadResult(preload bool, success bool) {
	if !preload || m.preloadManager == nil {
		return
	}
	m.preloadManager.recordResult(success, len(m.preloadRequests))
}

func createLoadingPlaceholder() *ebiten.Image {
	img := ebiten.NewImage(200, 150)
	img.Fill(color.RGBA{45, 45, 45, 255})
	return img
}

func (m *DefaultImageManager) ConsumeAsyncRefresh() bool {
	return m.asyncRefresh.Swap(false)
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
	m.loadCancel()
}

func (m *DefaultImageManager) GetPreloadStats() PreloadStats {
	if m.preloadManager != nil {
		return m.preloadManager.GetStats()
	}
	return PreloadStats{}
}

func (m *DefaultImageManager) GetPath(idx int) (ImagePath, bool) {
	return m.getPath(idx)
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

	m.startLoadWorker()
	m.requestAsyncLoad(imagePath)
	return m.loadingPlaceholder
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

func (m *DefaultImageManager) loadImageFromBytes(data []byte, path string) (*ebiten.Image, error) {
	reader := bytes.NewReader(data)
	decoded, _, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %v", path, err)
	}
	return m.createEbitenImageFromDecoded(decoded, path)
}

func (m *DefaultImageManager) loadImageFromZip(archivePath, entryPath string) (*ebiten.Image, error) {
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

			return m.loadImageFromBytes(data, entryPath)
		}
	}
	return nil, fmt.Errorf("entry %s not found in %s", entryPath, archivePath)
}

func (m *DefaultImageManager) loadImageFromRar(archivePath, entryPath string) (*ebiten.Image, error) {
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
			return m.loadImageFromBytes(data, entryPath)
		}
	}
	return nil, fmt.Errorf("entry %s not found in %s", entryPath, archivePath)
}

func (m *DefaultImageManager) loadImageFrom7z(archivePath, entryPath string) (*ebiten.Image, error) {
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

			return m.loadImageFromBytes(data, entryPath)
		}
	}
	return nil, fmt.Errorf("entry %s not found in %s", entryPath, archivePath)
}

func (m *DefaultImageManager) loadImage(imagePath ImagePath) (*ebiten.Image, error) {
	if imagePath.ArchivePath == "" {
		f, err := os.Open(imagePath.Path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		decoded, _, err := image.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decoding %s: %v", imagePath.Path, err)
		}
		return m.createEbitenImageFromDecoded(decoded, imagePath.Path)
	}

	ext := strings.ToLower(filepath.Ext(imagePath.ArchivePath))
	switch ext {
	case ".zip":
		return m.loadImageFromZip(imagePath.ArchivePath, imagePath.EntryPath)
	case ".rar":
		return m.loadImageFromRar(imagePath.ArchivePath, imagePath.EntryPath)
	case ".7z":
		return m.loadImageFrom7z(imagePath.ArchivePath, imagePath.EntryPath)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", ext)
	}
}

func (m *DefaultImageManager) createEbitenImageFromDecoded(src image.Image, origin string) (*ebiten.Image, error) {
	if src == nil {
		return nil, fmt.Errorf("decoded image is nil for %s", origin)
	}

	limit := m.preferredMaxDimension()
	if limit > 0 {
		bounds := src.Bounds()
		width := bounds.Dx()
		height := bounds.Dy()
		if width > limit || height > limit {
			resized, changed := resizeImageToFit(src, limit)
			if changed {
				newBounds := resized.Bounds()
				log.Printf("Info: downscaled large image %s from %dx%d to %dx%d (limit %d)", origin, width, height, newBounds.Dx(), newBounds.Dy(), limit)
				src = resized
			}
		}
	}

	return ebiten.NewImageFromImage(src), nil
}

func (m *DefaultImageManager) preferredMaxDimension() int {
	if cfg := int(m.maxImageDimension.Load()); cfg > 0 {
		return cfg
	}
	if size, ok := queryEbitenMaxImageSize(); ok && size > 0 {
		return size
	}
	return defaultMaxImageDimension
}

func resizeImageToFit(src image.Image, limit int) (image.Image, bool) {
	if limit <= 0 {
		return src, false
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= limit && height <= limit {
		return src, false
	}

	scale := float64(limit) / float64(width)
	if height > width {
		scale = float64(limit) / float64(height)
	}
	if scale >= 1.0 {
		return src, false
	}

	newW := int(math.Max(1, math.Round(float64(width)*scale)))
	newH := int(math.Max(1, math.Round(float64(height)*scale)))

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst, true
}

func queryEbitenMaxImageSize() (int, bool) {
	// Current Ebiten stable releases do not expose the texture limit.
	// Return false so that callers fall back to configuration-driven limits.
	return 0, false
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
