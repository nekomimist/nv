package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"nv/navlogic"
)

func isArchiveExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip", ".rar", ".7z":
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

type CollectionSourceMode int

const (
	CollectionSourceArgs CollectionSourceMode = iota
	CollectionSourceExpandedSingleDirectory
)

func (m CollectionSourceMode) String() string {
	switch m {
	case CollectionSourceArgs:
		return "args"
	case CollectionSourceExpandedSingleDirectory:
		return "expanded_single_directory"
	default:
		return "unknown"
	}
}

type CollectionSource struct {
	Mode             CollectionSourceMode
	Args             []string
	ExpandedFilePath string
}

func newArgsCollectionSource(args []string) CollectionSource {
	clonedArgs := make([]string, len(args))
	copy(clonedArgs, args)
	return CollectionSource{
		Mode: CollectionSourceArgs,
		Args: clonedArgs,
	}
}

func newExpandedDirectorySource(filePath string) CollectionSource {
	return CollectionSource{
		Mode:             CollectionSourceExpandedSingleDirectory,
		ExpandedFilePath: filePath,
	}
}

func (s CollectionSource) collect(sortMethod int) ([]ImagePath, error) {
	switch s.Mode {
	case CollectionSourceExpandedSingleDirectory:
		return collectImagesFromSameDirectory(s.ExpandedFilePath, sortMethod)
	default:
		return collectImages(s.Args, sortMethod)
	}
}

func (g *Game) getCurrentImagePath() string {
	imagePath, ok := g.imageManager.GetPath(g.idx)
	if !ok {
		return ""
	}
	return imagePath.Path
}

func findImagePathIndex(paths []ImagePath, targetPath string) int {
	if targetPath == "" {
		return -1
	}

	for i, imagePath := range paths {
		if imagePath.Path == targetPath {
			return i
		}
	}

	return -1
}

func (g *Game) setCurrentIndex(targetIdx int) {
	g.applyNavigationState(navlogic.SetCurrentIndex(g.navigationState(), targetIdx, g.pageMetricsAt))
}

func (g *Game) reloadPathsForCurrentSource() bool {
	currentPath := g.getCurrentImagePath()

	paths, err := g.collectionSource.collect(g.config.SortMethod)
	if err != nil || len(paths) == 0 {
		debugKV("collection", "reload_paths_failed",
			"source_mode", g.collectionSource.Mode,
			"sort_method", g.config.SortMethod,
			"current_path", currentPath,
			"paths_count", len(paths),
			"error", err,
		)
		return false
	}

	g.imageManager.SetPaths(paths)

	targetIdx := findImagePathIndex(paths, currentPath)
	if targetIdx < 0 && g.collectionSource.Mode == CollectionSourceExpandedSingleDirectory {
		targetIdx = findImagePathIndex(paths, g.collectionSource.ExpandedFilePath)
	}
	if targetIdx < 0 {
		targetIdx = 0
	}

	g.setCurrentIndex(targetIdx)
	g.calculateDisplayContent()
	debugKV("collection", "reload_paths_complete",
		"source_mode", g.collectionSource.Mode,
		"sort_method", g.config.SortMethod,
		"current_path", currentPath,
		"target_idx", targetIdx,
		"paths_count", len(paths),
	)
	return true
}

func (g *Game) cycleSortMethod() {
	prevSortMethod := g.config.SortMethod
	g.config.SortMethod = (g.config.SortMethod + 1) % 3
	g.updateSingleInstanceSortMethod()
	g.showOverlayMessage("Sort: " + getSortMethodName(g.config.SortMethod))
	g.reloadPathsForCurrentSource()
	debugKV("collection", "cycle_sort_method",
		"prev_sort_method", prevSortMethod,
		"next_sort_method", g.config.SortMethod,
		"source_mode", g.collectionSource.Mode,
	)
}

func (g *Game) expandToDirectoryAndJump() {
	// Only work when launched with a single regular image and not yet expanded.
	if g.launchSingleFile == "" || g.collectionSource.Mode == CollectionSourceExpandedSingleDirectory {
		debugKV("collection", "expand_directory_skip",
			"launch_single_file", g.launchSingleFile,
			"source_mode", g.collectionSource.Mode,
		)
		return
	}

	originalFilePath := g.launchSingleFile

	newPaths, err := collectImagesFromSameDirectory(originalFilePath, g.config.SortMethod)
	if err != nil {
		g.showOverlayMessage(fmt.Sprintf("Failed to scan directory: %v", err))
		debugKV("collection", "expand_directory_failed",
			"path", originalFilePath,
			"sort_method", g.config.SortMethod,
			"error", err,
		)
		return
	}

	if len(newPaths) == 0 {
		g.showOverlayMessage("No images found in directory")
		debugKV("collection", "expand_directory_failed",
			"path", originalFilePath,
			"sort_method", g.config.SortMethod,
			"reason", "no_images",
		)
		return
	}

	originalFileIndex := -1
	for i, imagePath := range newPaths {
		if imagePath.Path == originalFilePath {
			originalFileIndex = i
			break
		}
	}

	if originalFileIndex == -1 {
		g.showOverlayMessage("Original file not found in directory")
		debugKV("collection", "expand_directory_failed",
			"path", originalFilePath,
			"sort_method", g.config.SortMethod,
			"reason", "original_file_not_found",
		)
		return
	}

	g.imageManager.SetPaths(newPaths)
	g.collectionSource = newExpandedDirectorySource(originalFilePath)
	g.idx = originalFileIndex
	g.showOverlayMessage(fmt.Sprintf("Loaded %d images from directory", len(newPaths)))
	g.calculateDisplayContent()
	debugKV("collection", "expand_directory_complete",
		"path", originalFilePath,
		"sort_method", g.config.SortMethod,
		"paths_count", len(newPaths),
		"target_idx", originalFileIndex,
	)
}

func (g *Game) applyPendingOpenRequests() bool {
	if g.externalOpenRequests == nil {
		return false
	}

	applied := false
	for {
		select {
		case req := <-g.externalOpenRequests:
			g.applyPendingOpenRequest(req)
			applied = true
		default:
			return applied
		}
	}
}

func (g *Game) applyPendingOpenRequest(req pendingLaunchRequest) {
	bestEffortActivateWindow()
	if req.ActivateOnly {
		debugKV("single_instance", "activate_existing_window")
		return
	}

	g.replaceCollectionFromArgs(req.Args, req.Paths)
}

func (g *Game) replaceCollectionFromArgs(args []string, paths []ImagePath) {
	g.imageManager.SetPaths(paths)
	g.collectionSource = newArgsCollectionSource(args)
	g.launchSingleFile = ""
	g.idx = 0
	g.tempSingleMode = false
	g.bookMode = g.config.BookMode
	g.learnedSpreadAspects = nil
	g.rotationAngle = 0
	g.flipH = false
	g.flipV = false
	g.showHelp = false
	g.showInfo = false
	g.showSettings = false
	g.settingsIndex = 0
	g.pendingConfig = Config{}
	g.pageInputMode = false
	g.pageInputBuffer = ""

	g.resetZoomToInitial()
	initializeSingleFileMode(g, args)
	initializeBookModeForLaunch(g, paths)
	g.calculateDisplayContent()
	g.imageManager.StartPreload(g.idx, NavigationJump)
	g.showOverlayMessage(fmt.Sprintf("Loaded %d image(s)", len(paths)))

	debugKV("single_instance", "replace_collection",
		"args_count", len(args),
		"paths_count", len(paths),
		"book_mode", g.bookMode,
		"temp_single", g.tempSingleMode,
	)
}
