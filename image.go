package main

import (
	"archive/zip"
	"context"
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"nv/internal/imgdecode"

	"github.com/bodgit/sevenzip"
	"github.com/hajimehoshi/ebiten/v2"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/nwaples/rardecode"
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

func (d NavigationDirection) String() string {
	switch d {
	case NavigationForward:
		return "forward"
	case NavigationBackward:
		return "backward"
	case NavigationJump:
		return "jump"
	default:
		return "unknown"
	}
}

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

const (
	defaultMaxImageDimension = 8192
	defaultTileSize          = 2048
	fallbackTileSize         = 1024
)

type DisplayTile struct {
	Image *ebiten.Image
	X     int
	Y     int
	W     int
	H     int
}

type DisplayImage interface {
	Bounds() image.Rectangle
	Tiles() []DisplayTile
	TileCount() int
	Deallocate()
}

type tiledDisplayImage struct {
	bounds image.Rectangle
	tiles  []DisplayTile
}

func (i *tiledDisplayImage) Bounds() image.Rectangle {
	if i == nil {
		return image.Rectangle{}
	}
	return i.bounds
}

func (i *tiledDisplayImage) Tiles() []DisplayTile {
	if i == nil {
		return nil
	}
	return i.tiles
}

func (i *tiledDisplayImage) TileCount() int {
	if i == nil {
		return 0
	}
	return len(i.tiles)
}

func (i *tiledDisplayImage) Deallocate() {
	if i == nil {
		return
	}
	for _, tile := range i.tiles {
		if tile.Image != nil {
			tile.Image.Deallocate()
		}
	}
	i.tiles = nil
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
	debugKV("cache", "preload_stop")
}

// StartPreload starts preloading images from the current index in the specified direction
func (pm *PreloadManager) StartPreload(currentIdx int, direction NavigationDirection) {
	if !pm.IsEnabled() {
		debugKV("cache", "preload_skip", "reason", "disabled", "idx", currentIdx, "direction", direction)
		return
	}

	// Clear the request channel to cancel any pending requests
	drained := 0
drain:
	for {
		select {
		case <-pm.requestChan:
			drained++
		default:
			break drain
		}
	}

	// Send new preload request
	select {
	case pm.requestChan <- PreloadRequest{Index: currentIdx, Direction: direction}:
		debugKV("cache", "preload_start",
			"idx", currentIdx,
			"direction", direction,
			"drained", drained,
		)
	default:
		debugKV("cache", "preload_skip",
			"reason", "request_channel_full",
			"idx", currentIdx,
			"direction", direction,
		)
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
	debugKV("cache", "preload_plan",
		"idx", req.Index,
		"direction", req.Direction,
		"paths_count", pathsCount,
		"indices", indices,
	)

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
		debugKV("cache", "preload_skip", "reason", "already_cached", "idx", idx, "path", cacheKey)
		return // Already cached
	}

	pm.imageManager.requestPreload(imagePath)
	pm.updateQueueSize(len(pm.imageManager.preloadRequests))
}

// ImageManager interface for managing image loading and caching
type ImageManager interface {
	GetImage(idx int) DisplayImage
	GetBookModeImages(idx int, rightToLeft bool) (DisplayImage, DisplayImage)
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
	cache              *lru.Cache[string, DisplayImage]
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
	loadingPlaceholder DisplayImage
	asyncRefresh       atomic.Bool
}

type loadRequest struct {
	path     ImagePath
	cacheKey string
	preload  bool
}

// NewImageManager creates a new DefaultImageManager
func NewImageManager(cacheSize int) ImageManager {
	cache, err := lru.NewWithEvict[string, DisplayImage](cacheSize, func(_ string, img DisplayImage) {
		if img != nil {
			img.Deallocate()
		}
	})
	if err != nil {
		errorKV("cache", "cache_create_failed", "requested_size", cacheSize, "error", err)
		cache, _ = lru.NewWithEvict[string, DisplayImage](16, func(_ string, img DisplayImage) {
			if img != nil {
				img.Deallocate()
			}
		})
	}

	return newDefaultImageManager(cache)
}

// NewImageManagerWithPreload creates a new DefaultImageManager with preload configuration
func NewImageManagerWithPreload(cacheSize int, preloadCount int, preloadEnabled bool) ImageManager {
	cache, err := lru.NewWithEvict[string, DisplayImage](cacheSize, func(_ string, img DisplayImage) {
		if img != nil {
			img.Deallocate()
		}
	})
	if err != nil {
		errorKV("cache", "cache_create_failed", "requested_size", cacheSize, "error", err)
		cache, _ = lru.NewWithEvict[string, DisplayImage](16, func(_ string, img DisplayImage) {
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

func newDefaultImageManager(cache *lru.Cache[string, DisplayImage]) *DefaultImageManager {
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

// SetMaxImageDimension updates the dimension threshold that switches decoded images to tiled rendering.
// A value of 0 uses the default threshold.
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
		errorKV("cache", "cache_load_failed",
			"path", req.path.Path,
			"source", loadSource(req.preload),
			"error", err,
		)
		errorImg := createDisplayImageFromEbitenImage(CreateErrorImage(400, 300, req.path.Path, err.Error()))
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
	debugKV("cache", "cache_load_complete",
		"path", req.cacheKey,
		"source", loadSource(req.preload),
		"cache_len", m.cache.Len(),
		"mem_mb", mem.Alloc/1024/1024,
	)
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
		debugKV("cache", "cache_enqueue_skip",
			"path", cacheKey,
			"source", loadSource(preload),
			"reason", "already_cached",
		)
		return
	}

	m.inflightMu.Lock()
	if _, exists := m.inflight[cacheKey]; exists {
		m.inflightMu.Unlock()
		debugKV("cache", "cache_enqueue_skip",
			"path", cacheKey,
			"source", loadSource(preload),
			"reason", "already_inflight",
		)
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
		debugKV("cache", "cache_enqueue_skip",
			"path", cacheKey,
			"source", loadSource(preload),
			"reason", "load_context_closed",
		)
	case queue <- req:
		m.updatePreloadQueueSize()
		debugKV("cache", "cache_enqueue",
			"path", cacheKey,
			"source", loadSource(preload),
			"queue", queueName,
			"queue_len", len(queue),
		)
	default:
		m.clearInflight(cacheKey)
		debugKV("cache", "cache_enqueue_skip",
			"path", cacheKey,
			"source", loadSource(preload),
			"queue", queueName,
			"reason", "queue_full",
		)
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

func createLoadingPlaceholder() DisplayImage {
	img := ebiten.NewImage(200, 150)
	img.Fill(color.RGBA{45, 45, 45, 255})
	return createDisplayImageFromEbitenImage(img)
}

func createDisplayImageFromEbitenImage(img *ebiten.Image) DisplayImage {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	return &tiledDisplayImage{
		bounds: image.Rect(0, 0, bounds.Dx(), bounds.Dy()),
		tiles: []DisplayTile{{
			Image: img,
			X:     0,
			Y:     0,
			W:     bounds.Dx(),
			H:     bounds.Dy(),
		}},
	}
}

func (m *DefaultImageManager) ConsumeAsyncRefresh() bool {
	return m.asyncRefresh.Swap(false)
}

func (m *DefaultImageManager) SetPaths(paths []ImagePath) {
	m.mu.Lock()
	m.paths = paths
	m.mu.Unlock()
	debugKV("cache", "paths_replaced",
		"paths_count", len(paths),
		"cache_len", m.cache.Len(),
	)
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
	debugKV("cache", "load_stop")
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

func (m *DefaultImageManager) GetBookModeImages(idx int, rightToLeft bool) (DisplayImage, DisplayImage) {
	var leftImg, rightImg DisplayImage

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

func (m *DefaultImageManager) GetImage(idx int) DisplayImage {
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
		return img
	}

	debugKV("cache", "cache_lookup_miss", "idx", idx, "path", cacheKey)
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

func (m *DefaultImageManager) loadImageFromBytes(data []byte, path string) (DisplayImage, error) {
	decoded, err := imgdecode.DecodeBytes(data, path)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %v", path, err)
	}
	return m.createEbitenImageFromDecoded(decoded, path)
}

func (m *DefaultImageManager) loadImageFromZip(archivePath, entryPath string) (DisplayImage, error) {
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

func (m *DefaultImageManager) loadImageFromRar(archivePath, entryPath string) (DisplayImage, error) {
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

func (m *DefaultImageManager) loadImageFrom7z(archivePath, entryPath string) (DisplayImage, error) {
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

func (m *DefaultImageManager) loadImage(imagePath ImagePath) (DisplayImage, error) {
	if imagePath.ArchivePath == "" {
		decoded, err := imgdecode.DecodeFile(imagePath.Path)
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

func (m *DefaultImageManager) createEbitenImageFromDecoded(src image.Image, origin string) (DisplayImage, error) {
	if src == nil {
		return nil, fmt.Errorf("decoded image is nil for %s", origin)
	}

	limit := m.preferredMaxDimension()
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if limit > 0 && (width > limit || height > limit) {
		infoKV("cache", "image_tiling",
			"path", origin,
			"width", width,
			"height", height,
			"limit", limit,
			"tile_size", defaultTileSize,
		)
		return createTiledDisplayImage(src, defaultTileSize)
	}

	img, err := newDisplayImageFromImage(src)
	if err == nil {
		return img, nil
	}

	warnKV("cache", "image_single_texture_failed",
		"path", origin,
		"width", width,
		"height", height,
		"error", err,
		"fallback", "tiled",
	)
	return createTiledDisplayImage(src, fallbackTileSize)
}

func newDisplayImageFromImage(src image.Image) (DisplayImage, error) {
	var img *ebiten.Image
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		img = ebiten.NewImageFromImage(src)
	}()
	if recovered != nil {
		return nil, fmt.Errorf("creating ebiten image: %v", recovered)
	}
	return createDisplayImageFromEbitenImage(img), nil
}

func createTiledDisplayImage(src image.Image, tileSize int) (DisplayImage, error) {
	if tileSize <= 0 {
		tileSize = fallbackTileSize
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid image bounds: %v", bounds)
	}

	result := &tiledDisplayImage{
		bounds: image.Rect(0, 0, width, height),
		tiles:  make([]DisplayTile, 0, ((width+tileSize-1)/tileSize)*((height+tileSize-1)/tileSize)),
	}

	for y := 0; y < height; y += tileSize {
		tileH := min(tileSize, height-y)
		for x := 0; x < width; x += tileSize {
			tileW := min(tileSize, width-x)
			tileRect := image.Rect(bounds.Min.X+x, bounds.Min.Y+y, bounds.Min.X+x+tileW, bounds.Min.Y+y+tileH)
			tileSrc := image.NewNRGBA(image.Rect(0, 0, tileW, tileH))
			imagedraw.Draw(tileSrc, tileSrc.Bounds(), src, tileRect.Min, imagedraw.Src)

			tileImg, err := newUnmanagedEbitenImage(tileSrc)
			if err != nil {
				result.Deallocate()
				if tileSize > fallbackTileSize {
					return createTiledDisplayImage(src, fallbackTileSize)
				}
				return nil, err
			}
			result.tiles = append(result.tiles, DisplayTile{
				Image: tileImg,
				X:     x,
				Y:     y,
				W:     tileW,
				H:     tileH,
			})
		}
	}

	return result, nil
}

func newUnmanagedEbitenImage(src image.Image) (*ebiten.Image, error) {
	var img *ebiten.Image
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		img = ebiten.NewImageFromImageWithOptions(src, &ebiten.NewImageFromImageOptions{Unmanaged: true})
	}()
	if recovered != nil {
		return nil, fmt.Errorf("creating tiled ebiten image: %v", recovered)
	}
	return img, nil
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
		errorKV("collection", "archive_process_failed", "archive_path", archivePath, "error", err)
		return []ImagePath{}, err
	}

	debugKV("collection", "archive_processed", "archive_path", archivePath, "entries", len(archiveImages))
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
	debugKV("collection", "collect_same_directory_complete",
		"file_path", filePath,
		"directory", dir,
		"sort_method", sortMethod,
		"paths_count", len(sortedImages),
	)
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
			archiveCount := 0
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
					archiveCount++
					archiveImages, err := processArchive(path)
					if err == nil {
						sortedArchiveImages := sortImagePaths(archiveImages, sortMethod)
						dirImages = append(dirImages, sortedArchiveImages...)
					} else {
						warnKV("collection", "archive_skipped", "path", path, "error", err)
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			sortedDirImages := sortImagePaths(dirImages, sortMethod)
			list = append(list, sortedDirImages...)
			debugKV("collection", "collect_directory_complete",
				"path", p,
				"sort_method", sortMethod,
				"paths_count", len(sortedDirImages),
				"archives_seen", archiveCount,
			)
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
					debugKV("collection", "collect_archive_complete",
						"path", p,
						"sort_method", sortMethod,
						"paths_count", len(sortedArchiveImages),
					)
				} else {
					warnKV("collection", "archive_skipped", "path", p, "error", err)
				}
			}
		}
	}

	debugKV("collection", "collect_complete",
		"args_count", len(args),
		"sort_method", sortMethod,
		"paths_count", len(list),
	)
	return list, nil
}

func loadSource(preload bool) string {
	if preload {
		return "preload"
	}
	return "async"
}
