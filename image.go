package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"nv/internal/imgdecode"

	"github.com/hajimehoshi/ebiten/v2"
	lru "github.com/hashicorp/golang-lru/v2"
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
	tileGutterSize           = 1
)

type DisplayTile struct {
	Image *ebiten.Image
	X     int
	Y     int
	W     int
	H     int
	SrcX  int // Core-image origin within Image, excluding its sampling gutter.
	SrcY  int
}

type DisplayImage interface {
	Bounds() image.Rectangle
	// SourceBounds reports the full-resolution size of the original
	// image. It equals Bounds() unless this DisplayImage was decoded from
	// a reduced (budget-tier) hint, in which case it is strictly larger.
	SourceBounds() image.Rectangle
	Tiles() []DisplayTile
	TileCount() int
	Deallocate()
}

type tiledDisplayImage struct {
	bounds image.Rectangle
	// sourceW, sourceH hold the original image's full-resolution size.
	// They equal bounds' dimensions unless this image was decoded from a
	// reduced hint (see imgdecode.Info.Reduced).
	sourceW int
	sourceH int
	tiles   []DisplayTile
}

func (i *tiledDisplayImage) Bounds() image.Rectangle {
	if i == nil {
		return image.Rectangle{}
	}
	return i.bounds
}

func (i *tiledDisplayImage) SourceBounds() image.Rectangle {
	if i == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, i.sourceW, i.sourceH)
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
	// Preloading always targets full resolution, matching every other
	// caller in this change (see requestTier's doc comment).
	cacheKey := imgCacheKey{path: imagePath.Path, tier: tierFull}

	// Check if already in cache
	if _, ok := pm.imageManager.cache.Get(cacheKey); ok {
		debugKV("cache", "preload_skip", "reason", "already_cached", "idx", idx, "path", cacheKey.path)
		return // Already cached
	}

	pm.imageManager.requestPreload(imagePath, tierFull)
	pm.updateQueueSize(len(pm.imageManager.preloadRequests))
}

// ImageManager interface for managing image loading and caching
type ImageManager interface {
	GetImage(idx int, hint imgdecode.Hint) DisplayImage
	GetBookModeImages(idx int, rightToLeft bool, leftHint, rightHint imgdecode.Hint) (DisplayImage, DisplayImage)
	// EnsureResolution is the write-trigger counterpart to GetImage's read
	// path: it queues a full-resolution refinement when the cached image
	// for idx falls short of hint and a better decode actually exists. It
	// is deliberately separate from GetImage (which never blocks on or
	// queues a refinement itself beyond its normal cache-miss load) and is
	// safe to call every frame.
	EnsureResolution(idx int, hint imgdecode.Hint)
	GetPath(idx int) (ImagePath, bool)
	SetPaths(paths []ImagePath)
	GetPathsCount() int
	StartPreload(currentIdx int, direction NavigationDirection)
	StopPreload()
	GetPreloadStats() PreloadStats
	ConsumeAsyncRefresh() bool
}

// resTier identifies which resolution tier a cached DisplayImage was
// decoded at.
type resTier int

const (
	tierBudget resTier = iota // decoded to fit a display budget (Hint-constrained)
	tierFull                  // full source resolution
)

func (t resTier) String() string {
	if t == tierFull {
		return "full"
	}
	return "budget"
}

// imgCacheKey identifies one resolution tier of one image path in the
// cache. Keying by tier (rather than path alone) means a late-arriving
// budget-tier result can never clobber an already-cached full-resolution
// one -- they simply occupy different keys.
type imgCacheKey struct {
	path string
	tier resTier
}

// loadSourceKind records why a loadRequest was enqueued, purely for queue
// selection and logging.
type loadSourceKind int

const (
	loadSourceAsync   loadSourceKind = iota // synchronous cache-miss from GetImage
	loadSourcePreload                       // PreloadManager warming nearby pages
	loadSourceRefine                        // EnsureResolution escalating budget -> full
)

func (k loadSourceKind) String() string {
	switch k {
	case loadSourcePreload:
		return "preload"
	case loadSourceRefine:
		return "refine"
	default:
		return "async"
	}
}

// DefaultImageManager implements ImageManager
type DefaultImageManager struct {
	paths             []ImagePath
	cache             *lru.Cache[imgCacheKey, DisplayImage]
	mu                sync.RWMutex
	preloadManager    *PreloadManager
	maxImageDimension atomic.Int64
	loadRequests      chan loadRequest
	preloadRequests   chan loadRequest
	// refineRequests carries EnsureResolution's tierFull escalations. It
	// is deliberately the lowest-priority queue (see asyncLoadWorker) and
	// small, since a refinement must never crowd out real navigation or
	// preload work.
	refineRequests chan loadRequest
	inflight       map[imgCacheKey]struct{}
	inflightMu     sync.Mutex
	// fullRequested guards against refinement request storms: once a
	// path's tierFull load has been requested there is nothing more to
	// escalate to (tierFull is a single maximal target), so further
	// EnsureResolution calls for that path are no-ops until its cache
	// entries are evicted (see the cache's onEvicted callback).
	fullRequested      map[string]struct{}
	fullRequestedMu    sync.Mutex
	loadCtx            context.Context
	loadCancel         context.CancelFunc
	loadWorkerOnce     sync.Once
	loadingPlaceholder DisplayImage
	asyncRefresh       atomic.Bool
	// archiveCache is owned exclusively by asyncLoadWorker; see the
	// ownership note on archiveHandleCache.
	archiveCache *archiveHandleCache
}

type loadRequest struct {
	path     ImagePath
	cacheKey imgCacheKey
	source   loadSourceKind
}

// loadTimings records per-phase durations for a single image load, used to
// emit the "load_timing" debug log line from processLoadRequest. A zero
// value in a field means that phase either didn't run (e.g. read on the
// plain-file path, which os.ReadFile inside imgdecode.DecodeFile does not
// expose separately) or hasn't been measured yet.
type loadTimings struct {
	read   time.Duration
	decode time.Duration
	upload time.Duration
}

// newManagedCache creates the tiered image LRU cache with an eviction
// callback that deallocates the evicted DisplayImage and clears the owning
// manager's "full resolution already requested" guard for that path (see
// DefaultImageManager.fullRequested). The cache must exist before the
// DefaultImageManager does (it's one of the struct's fields), so the
// callback closes over managerRef and reads through it lazily; callers
// fill *managerRef in immediately after construction, before any Add/Get
// can trigger an eviction.
func newManagedCache(cacheSize int, managerRef **DefaultImageManager) *lru.Cache[imgCacheKey, DisplayImage] {
	onEvict := func(key imgCacheKey, img DisplayImage) {
		if img != nil {
			img.Deallocate()
		}
		if m := *managerRef; m != nil {
			m.clearFullRequested(key.path)
		}
	}
	cache, err := lru.NewWithEvict[imgCacheKey, DisplayImage](cacheSize, onEvict)
	if err != nil {
		errorKV("cache", "cache_create_failed", "requested_size", cacheSize, "error", err)
		cache, _ = lru.NewWithEvict[imgCacheKey, DisplayImage](16, onEvict)
	}
	return cache
}

// NewImageManager creates a new DefaultImageManager
func NewImageManager(cacheSize int) ImageManager {
	var manager *DefaultImageManager
	cache := newManagedCache(cacheSize, &manager)
	manager = newDefaultImageManager(cache, 0)
	return manager
}

// NewImageManagerWithPreload creates a new DefaultImageManager with preload configuration
func NewImageManagerWithPreload(cacheSize int, preloadCount int, preloadEnabled bool) ImageManager {
	var manager *DefaultImageManager
	cache := newManagedCache(cacheSize, &manager)
	manager = newDefaultImageManager(cache, preloadCount)

	// Initialize preload manager with configuration
	manager.preloadManager = NewPreloadManager(manager, preloadCount)
	manager.preloadManager.SetEnabled(preloadEnabled)

	return manager
}

// Preload request queue capacity bounds. The floor keeps the plain
// NewImageManager path (no preload configuration) unchanged, while the
// cap matches the maximum legal config PreloadCount.
const (
	minPreloadQueueCapacity = 8
	maxPreloadQueueCapacity = 16
)

// preloadQueueCapacity sizes the preload request channel from the
// configured preload count so PreloadCount values above the previous
// hardcoded capacity of 8 no longer cause preload requests to be
// silently dropped.
func preloadQueueCapacity(preloadCount int) int {
	capacity := preloadCount
	if capacity < minPreloadQueueCapacity {
		capacity = minPreloadQueueCapacity
	}
	if capacity > maxPreloadQueueCapacity {
		capacity = maxPreloadQueueCapacity
	}
	return capacity
}

// refineQueueCapacity is deliberately small: refinements are a background
// nicety, never a substitute for real navigation or preload work, so a
// backlog here should apply backpressure (via enqueueLoadRequest's
// queue-full skip) rather than accumulate.
const refineQueueCapacity = 4

func newDefaultImageManager(cache *lru.Cache[imgCacheKey, DisplayImage], preloadCount int) *DefaultImageManager {
	loadCtx, loadCancel := context.WithCancel(context.Background())
	manager := &DefaultImageManager{
		paths:              []ImagePath{},
		cache:              cache,
		loadRequests:       make(chan loadRequest, 8),
		preloadRequests:    make(chan loadRequest, preloadQueueCapacity(preloadCount)),
		refineRequests:     make(chan loadRequest, refineQueueCapacity),
		inflight:           make(map[imgCacheKey]struct{}),
		fullRequested:      make(map[string]struct{}),
		loadCtx:            loadCtx,
		loadCancel:         loadCancel,
		loadingPlaceholder: createLoadingPlaceholder(),
		archiveCache:       newArchiveHandleCache(),
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

// asyncLoadWorker drains loadRequests, preloadRequests, and refineRequests
// in strict priority order -- a real navigation load always wins over a
// preload, and a preload always wins over a background refinement -- by
// checking each queue non-blockingly before falling back to a blocking
// select across all of them (plus preloadRequests and loadRequests again,
// so a request that arrives while this goroutine was blocked isn't stuck
// behind a still-empty refineRequests wakeup).
func (m *DefaultImageManager) asyncLoadWorker() {
	defer m.archiveCache.closeAll()
	for {
		select {
		case <-m.loadCtx.Done():
			return
		default:
		}

		select {
		case req := <-m.loadRequests:
			m.processLoadRequest(req)
			continue
		default:
		}

		select {
		case req := <-m.loadRequests:
			m.processLoadRequest(req)
			continue
		case req := <-m.preloadRequests:
			m.processLoadRequest(req)
			continue
		default:
			select {
			case <-m.loadCtx.Done():
				return
			case req := <-m.loadRequests:
				m.processLoadRequest(req)
			case req := <-m.preloadRequests:
				m.processLoadRequest(req)
			case req := <-m.refineRequests:
				m.processLoadRequest(req)
			}
		}
	}
}

func (m *DefaultImageManager) processLoadRequest(req loadRequest) {
	start := time.Now()
	defer func() {
		m.inflightMu.Lock()
		delete(m.inflight, req.cacheKey)
		m.inflightMu.Unlock()
	}()

	img, timings, err := m.loadImage(req.path)
	if err != nil {
		errorKV("cache", "cache_load_failed",
			"path", req.path.Path,
			"tier", req.cacheKey.tier,
			"source", req.source,
			"error", err,
			"total_ms", formatMillis(time.Since(start)),
		)
		errorImg := createDisplayImageFromEbitenImage(CreateErrorImage(400, 300, req.path.Path, err.Error()))
		m.cache.Add(req.cacheKey, errorImg)
		m.asyncRefresh.Store(true)
		m.recordPreloadResult(req.source == loadSourcePreload, false)
		return
	}

	m.cache.Add(req.cacheKey, img)
	if req.cacheKey.tier == tierFull {
		// A full-resolution decode landed: drop the now-redundant budget
		// entry (if any) for the same path so steady-state cache usage
		// stays at one entry per path. The evict callback deallocates it.
		// This only runs on success -- a failed refinement must never
		// evict a working budget-tier image out from under the reader.
		m.cache.Remove(imgCacheKey{path: req.cacheKey.path, tier: tierBudget})
	}
	m.asyncRefresh.Store(true)
	m.recordPreloadResult(req.source == loadSourcePreload, true)

	if debugMode {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		debugKV("cache", "cache_load_complete",
			"path", req.cacheKey.path,
			"tier", req.cacheKey.tier,
			"source", req.source,
			"cache_len", m.cache.Len(),
			"mem_mb", mem.Alloc/1024/1024,
		)
	}

	bounds := img.Bounds()
	debugKV("cache", "load_timing",
		"path", req.cacheKey.path,
		"tier", req.cacheKey.tier,
		"source", req.source,
		"read_ms", formatMillis(timings.read),
		"decode_ms", formatMillis(timings.decode),
		"upload_ms", formatMillis(timings.upload),
		"total_ms", formatMillis(time.Since(start)),
		"w", bounds.Dx(),
		"h", bounds.Dy(),
	)
}

// formatMillis formats a duration as milliseconds with a fixed two-decimal
// precision for log output. It's a plain string (not a bare float) so the
// value round-trips through formatLogValue exactly as written here, rather
// than through strconv.FormatFloat's shortest-round-trip formatting.
func formatMillis(d time.Duration) string {
	return strconv.FormatFloat(float64(d.Nanoseconds())/1e6, 'f', 2, 64)
}

func (m *DefaultImageManager) requestAsyncLoad(imagePath ImagePath, tier resTier) {
	m.enqueueLoadRequest(imagePath, tier, loadSourceAsync)
}

func (m *DefaultImageManager) requestPreload(imagePath ImagePath, tier resTier) {
	m.enqueueLoadRequest(imagePath, tier, loadSourcePreload)
}

// queueFor picks the channel (and its debug name) a request of kind should
// be sent on. loadRequests > preloadRequests > refineRequests in priority;
// see asyncLoadWorker.
func (m *DefaultImageManager) queueFor(kind loadSourceKind) (chan loadRequest, string) {
	switch kind {
	case loadSourcePreload:
		return m.preloadRequests, "preload"
	case loadSourceRefine:
		return m.refineRequests, "refine"
	default:
		return m.loadRequests, "async"
	}
}

func (m *DefaultImageManager) enqueueLoadRequest(imagePath ImagePath, tier resTier, kind loadSourceKind) {
	cacheKey := imgCacheKey{path: imagePath.Path, tier: tier}
	if _, ok := m.cache.Get(cacheKey); ok {
		debugKV("cache", "cache_enqueue_skip",
			"path", cacheKey.path,
			"tier", cacheKey.tier,
			"source", kind,
			"reason", "already_cached",
		)
		return
	}

	m.inflightMu.Lock()
	if _, exists := m.inflight[cacheKey]; exists {
		m.inflightMu.Unlock()
		debugKV("cache", "cache_enqueue_skip",
			"path", cacheKey.path,
			"tier", cacheKey.tier,
			"source", kind,
			"reason", "already_inflight",
		)
		return
	}
	m.inflight[cacheKey] = struct{}{}
	m.inflightMu.Unlock()

	req := loadRequest{path: imagePath, cacheKey: cacheKey, source: kind}
	queue, queueName := m.queueFor(kind)

	select {
	case <-m.loadCtx.Done():
		m.clearInflight(cacheKey)
		debugKV("cache", "cache_enqueue_skip",
			"path", cacheKey.path,
			"tier", cacheKey.tier,
			"source", kind,
			"reason", "load_context_closed",
		)
	case queue <- req:
		m.updatePreloadQueueSize()
		debugKV("cache", "cache_enqueue",
			"path", cacheKey.path,
			"tier", cacheKey.tier,
			"source", kind,
			"queue", queueName,
			"queue_len", len(queue),
		)
	default:
		m.clearInflight(cacheKey)
		debugKV("cache", "cache_enqueue_skip",
			"path", cacheKey.path,
			"tier", cacheKey.tier,
			"source", kind,
			"queue", queueName,
			"reason", "queue_full",
		)
	}
}

func (m *DefaultImageManager) clearInflight(cacheKey imgCacheKey) {
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

// clearFullRequested drops path's "full resolution already requested"
// guard. It runs from the cache's onEvicted callback, so once every entry
// for a path has left the cache, a future EnsureResolution call is free to
// request tierFull again from scratch.
func (m *DefaultImageManager) clearFullRequested(path string) {
	m.fullRequestedMu.Lock()
	delete(m.fullRequested, path)
	m.fullRequestedMu.Unlock()
}

func createLoadingPlaceholder() DisplayImage {
	img := ebiten.NewImage(200, 150)
	img.Fill(color.RGBA{45, 45, 45, 255})
	return createDisplayImageFromEbitenImage(img)
}

// createDisplayImageFromEbitenImage wraps img as a single-tile DisplayImage
// whose SourceBounds mirrors Bounds -- appropriate for callers (the loading
// placeholder, the error image) that have no decode Info describing a
// separate source resolution.
func createDisplayImageFromEbitenImage(img *ebiten.Image) DisplayImage {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	return &tiledDisplayImage{
		bounds:  image.Rect(0, 0, w, h),
		sourceW: w,
		sourceH: h,
		tiles: []DisplayTile{{
			Image: img,
			X:     0,
			Y:     0,
			W:     w,
			H:     h,
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

func (m *DefaultImageManager) GetBookModeImages(idx int, rightToLeft bool, leftHint, rightHint imgdecode.Hint) (DisplayImage, DisplayImage) {
	var leftImg, rightImg DisplayImage

	if rightToLeft {
		// Right-to-left reading (Japanese manga style): [next][current]
		leftImg = m.GetImage(idx+1, leftHint) // Next image on left
		rightImg = m.GetImage(idx, rightHint) // Current image on right
	} else {
		// Left-to-right reading (Western style): [current][next]
		leftImg = m.GetImage(idx, leftHint)     // Current image on left
		rightImg = m.GetImage(idx+1, rightHint) // Next image on right (nil if OOB)
	}

	return leftImg, rightImg
}

// requestTier picks the cache tier a fresh (nothing cached yet) load
// should target for hint. An unconstrained hint -- imgdecode.Hint{}, which
// every call site in this change passes -- asks for the image outright, so
// it goes straight to tierFull; a real constraint starts at tierBudget and
// may later be escalated by EnsureResolution. This is the seam a future
// change will use to request genuinely reduced decodes.
func requestTier(hint imgdecode.Hint) resTier {
	if hint.MaxWidth > 0 || hint.MaxHeight > 0 {
		return tierBudget
	}
	return tierFull
}

func (m *DefaultImageManager) GetImage(idx int, hint imgdecode.Hint) DisplayImage {
	m.mu.RLock()
	if idx < 0 || idx >= len(m.paths) {
		m.mu.RUnlock()
		return nil
	}
	imagePath := m.paths[idx]
	m.mu.RUnlock()

	// tierFull is always "good enough": nothing beats full resolution.
	if img, ok := m.cache.Get(imgCacheKey{path: imagePath.Path, tier: tierFull}); ok {
		return img
	}

	// A budget-tier hit still has real pixels, so return it immediately
	// even if it may be too small for hint -- never fall back to the
	// placeholder when real pixels exist. Whether it's actually too small
	// is EnsureResolution's decision (the write-triggered path), queued
	// here as a side effect but not blocking this read.
	if img, ok := m.cache.Get(imgCacheKey{path: imagePath.Path, tier: tierBudget}); ok {
		m.EnsureResolution(idx, hint)
		return img
	}

	debugKV("cache", "cache_lookup_miss", "idx", idx, "path", imagePath.Path)
	m.startLoadWorker()
	m.requestAsyncLoad(imagePath, requestTier(hint))
	return m.loadingPlaceholder
}

// sufficientForHint reports whether an already-decoded image's bounds meet
// hint's requested display box. A zero field in hint (imgdecode.Hint's
// documented "no limit" zero value) is always satisfied on that axis.
func sufficientForHint(bounds image.Rectangle, hint imgdecode.Hint) bool {
	if hint.MaxWidth > 0 && bounds.Dx() < hint.MaxWidth {
		return false
	}
	if hint.MaxHeight > 0 && bounds.Dy() < hint.MaxHeight {
		return false
	}
	return true
}

// EnsureResolution queues a tierFull refinement for idx when the cached
// image falls short of hint and a higher-resolution decode actually
// exists. It never blocks and is safe to call every frame -- every check
// here is a cheap cache Peek or guarded map lookup, and the actual escalate
// happens (if at all) on the background load worker.
func (m *DefaultImageManager) EnsureResolution(idx int, hint imgdecode.Hint) {
	imagePath, ok := m.getPath(idx)
	if !ok {
		return
	}

	if _, ok := m.cache.Peek(imgCacheKey{path: imagePath.Path, tier: tierFull}); ok {
		// Already at the maximal tier; nothing left to escalate to.
		return
	}

	img, ok := m.cache.Peek(imgCacheKey{path: imagePath.Path, tier: tierBudget})
	if !ok {
		// Nothing cached yet for this path; GetImage's own cache-miss
		// path is responsible for the initial load.
		return
	}

	// Reduced == false (imgdecode.Info) means no higher-resolution version
	// of this image exists, which is exactly SourceBounds() == Bounds()
	// here. Treating that as "already maximal" keeps this structurally
	// true even for a stdlib-only build, where every decode always
	// reports Reduced == false and so can never trigger a refinement.
	if img.SourceBounds() == img.Bounds() {
		return
	}

	if sufficientForHint(img.Bounds(), hint) {
		return
	}

	m.requestFullResolution(imagePath)
}

// requestFullResolution escalates path to tierFull, guarded by
// fullRequested so a path already requested (still loading, or resolved
// and still cached) is never re-queued -- tierFull is a single maximal
// target, so there is nothing further to escalate to once it has been
// asked for. The guard is cleared when the path's cache entries are
// evicted (see newManagedCache), allowing a fresh request afterward.
func (m *DefaultImageManager) requestFullResolution(imagePath ImagePath) {
	m.fullRequestedMu.Lock()
	if _, already := m.fullRequested[imagePath.Path]; already {
		m.fullRequestedMu.Unlock()
		return
	}
	m.fullRequested[imagePath.Path] = struct{}{}
	m.fullRequestedMu.Unlock()

	m.enqueueLoadRequest(imagePath, tierFull, loadSourceRefine)
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

func (m *DefaultImageManager) loadImageFromBytes(data []byte, path string) (DisplayImage, loadTimings, error) {
	var timings loadTimings

	// hint is Hint{} (unconstrained) for now: this only switches the
	// decode boundary onto the *Scaled entry points so Info -- and thus
	// DisplayImage.SourceBounds -- is populated. Actually requesting a
	// reduced decode is a later change.
	decodeStart := time.Now()
	decoded, info, err := imgdecode.DecodeBytesScaled(data, path, imgdecode.Hint{})
	timings.decode = time.Since(decodeStart)
	if err != nil {
		return nil, timings, fmt.Errorf("decoding %s: %v", path, err)
	}

	uploadStart := time.Now()
	img, err := m.createEbitenImageFromDecoded(decoded, path, info)
	timings.upload = time.Since(uploadStart)
	return img, timings, err
}

func (m *DefaultImageManager) loadImage(imagePath ImagePath) (DisplayImage, loadTimings, error) {
	if imagePath.ArchivePath == "" {
		// imgdecode.DecodeFileScaled does its own os.ReadFile internally,
		// so read time isn't separable from decode time on this path;
		// read_ms stays zero here (see loadTimings doc comment). hint is
		// Hint{} for the same reason as loadImageFromBytes above.
		decodeStart := time.Now()
		decoded, info, err := imgdecode.DecodeFileScaled(imagePath.Path, imgdecode.Hint{})
		timings := loadTimings{decode: time.Since(decodeStart)}
		if err != nil {
			return nil, timings, fmt.Errorf("decoding %s: %v", imagePath.Path, err)
		}

		uploadStart := time.Now()
		img, err := m.createEbitenImageFromDecoded(decoded, imagePath.Path, info)
		timings.upload = time.Since(uploadStart)
		return img, timings, err
	}

	readStart := time.Now()
	data, err := m.archiveCache.readEntry(imagePath.ArchivePath, imagePath.EntryPath)
	readDur := time.Since(readStart)
	if err != nil {
		return nil, loadTimings{read: readDur}, err
	}

	img, timings, err := m.loadImageFromBytes(data, imagePath.EntryPath)
	timings.read = readDur
	return img, timings, err
}

// applySourceDims records a decode's full source resolution (imgdecode.Info)
// onto img, so DisplayImage.SourceBounds reflects it even though the
// single-texture and tiled construction paths below don't otherwise see
// Info. tiledDisplayImage is the only DisplayImage this package builds, so
// the type assertion always succeeds for images built here.
func applySourceDims(img DisplayImage, info imgdecode.Info) {
	di, ok := img.(*tiledDisplayImage)
	if !ok || di == nil {
		return
	}
	di.sourceW = info.SourceWidth
	di.sourceH = info.SourceHeight
}

func (m *DefaultImageManager) createEbitenImageFromDecoded(src image.Image, origin string, info imgdecode.Info) (DisplayImage, error) {
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
		tiled, err := createTiledDisplayImage(src, defaultTileSize)
		if err != nil {
			return nil, err
		}
		applySourceDims(tiled, info)
		return tiled, nil
	}

	img, err := newDisplayImageFromImage(src)
	if err == nil {
		applySourceDims(img, info)
		return img, nil
	}

	warnKV("cache", "image_single_texture_failed",
		"path", origin,
		"width", width,
		"height", height,
		"error", err,
		"fallback", "tiled",
	)
	tiled, err := createTiledDisplayImage(src, fallbackTileSize)
	if err != nil {
		return nil, err
	}
	applySourceDims(tiled, info)
	return tiled, nil
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
	coreTileSize := tileSize - 2*tileGutterSize
	if coreTileSize <= 0 {
		return nil, fmt.Errorf("tile size %d is too small for %d-pixel gutters", tileSize, tileGutterSize)
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid image bounds: %v", bounds)
	}

	result := &tiledDisplayImage{
		bounds: image.Rect(0, 0, width, height),
		tiles:  make([]DisplayTile, 0, ((width+coreTileSize-1)/coreTileSize)*((height+coreTileSize-1)/coreTileSize)),
	}

	// tileSize is the maximum texture dimension, including a one-pixel
	// sampling gutter on every side. Keeping the texture at or below a
	// power-of-two boundary avoids a 2048-wide tile being rounded up to a
	// 4096-wide GPU texture just to accommodate its gutters.
	//
	// Reuse a scratch buffer for full-size tiles. Edge tiles, whose core is
	// smaller than coreTileSize, allocate a right-sized buffer below.
	// drawTileWithClampedGutter fully overwrites the buffer on every draw, and
	// ebiten.NewImageFromImageWithOptions (via newUnmanagedEbitenImage)
	// copies pixel data synchronously without retaining the source
	// image, so reusing this buffer across tiles is safe.
	tileScratch := image.NewRGBA(image.Rect(0, 0, tileSize, tileSize))

	for y := 0; y < height; y += coreTileSize {
		tileH := min(coreTileSize, height-y)
		for x := 0; x < width; x += coreTileSize {
			tileW := min(coreTileSize, width-x)
			textureW := tileW + 2*tileGutterSize
			textureH := tileH + 2*tileGutterSize
			tileSrc := tileScratch
			if textureW != tileSize || textureH != tileSize {
				tileSrc = image.NewRGBA(image.Rect(0, 0, textureW, textureH))
			}

			sampleRect := image.Rect(
				bounds.Min.X+x-tileGutterSize,
				bounds.Min.Y+y-tileGutterSize,
				bounds.Min.X+x+tileW+tileGutterSize,
				bounds.Min.Y+y+tileH+tileGutterSize,
			)
			drawTileWithClampedGutter(tileSrc, src, sampleRect)

			tileImg, err := newUnmanagedEbitenImageFn(tileSrc)
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
				SrcX:  tileGutterSize,
				SrcY:  tileGutterSize,
			})
		}
	}

	return result, nil
}

// drawTileWithClampedGutter copies srcRect into dst. The part of srcRect
// outside src.Bounds is filled by extending the nearest edge pixel, matching
// clamp-to-edge sampling at the outer boundary of the complete image.
func drawTileWithClampedGutter(dst *image.RGBA, src image.Image, srcRect image.Rectangle) {
	srcBounds := src.Bounds()
	clipped := srcRect.Intersect(srcBounds)
	dstMin := clipped.Min.Sub(srcRect.Min)
	dstRect := image.Rectangle{Min: dstMin, Max: dstMin.Add(clipped.Size())}
	imagedraw.Draw(dst, dstRect, src, clipped.Min, imagedraw.Src)

	for y := dstRect.Min.Y; y < dstRect.Max.Y; y++ {
		row := y * dst.Stride
		leftPixel := row + dstRect.Min.X*4
		for x := 0; x < dstRect.Min.X; x++ {
			copy(dst.Pix[row+x*4:row+x*4+4], dst.Pix[leftPixel:leftPixel+4])
		}

		rightPixel := row + (dstRect.Max.X-1)*4
		for x := dstRect.Max.X; x < dst.Bounds().Dx(); x++ {
			copy(dst.Pix[row+x*4:row+x*4+4], dst.Pix[rightPixel:rightPixel+4])
		}
	}

	rowBytes := dst.Bounds().Dx() * 4
	firstRow := dstRect.Min.Y * dst.Stride
	for y := 0; y < dstRect.Min.Y; y++ {
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+rowBytes], dst.Pix[firstRow:firstRow+rowBytes])
	}

	lastRow := (dstRect.Max.Y - 1) * dst.Stride
	for y := dstRect.Max.Y; y < dst.Bounds().Dy(); y++ {
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+rowBytes], dst.Pix[lastRow:lastRow+rowBytes])
	}
}

// newUnmanagedEbitenImageFn converts a tile's pixel buffer into an
// unmanaged ebiten image. It is a package variable (rather than a direct
// call to newUnmanagedEbitenImage) purely so tests can intercept the
// buffer that would be converted: reading a real ebiten.Image's pixels
// back requires an active Ebiten game loop, which isn't running under
// `go test`, so this is the only way to verify tile pixel content in a
// regression test.
var newUnmanagedEbitenImageFn = newUnmanagedEbitenImage

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

// extractImagesFromArchive enumerates archivePath's supported image
// entries via a one-shot archiveHandle. This runs on the UI goroutine and
// must not touch the worker-owned archiveHandleCache: it opens its own
// handle and closes it immediately after enumeration.
func extractImagesFromArchive(archivePath string) ([]ImagePath, error) {
	handle, err := openArchiveHandle(archivePath)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	var images []ImagePath
	for _, entry := range handle.Entries() {
		if !entry.IsDir && isSupportedExt(entry.Name) {
			images = append(images, ImagePath{
				Path:        archivePath + ":" + entry.Name,
				ArchivePath: archivePath,
				EntryPath:   entry.Name,
			})
		}
	}
	return images, nil
}

func processArchive(archivePath string) ([]ImagePath, error) {
	if !isArchiveExt(archivePath) {
		return []ImagePath{}, nil
	}

	archiveImages, err := extractImagesFromArchive(archivePath)
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
