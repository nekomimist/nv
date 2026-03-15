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
	return true
}

func (g *Game) cycleSortMethod() {
	g.config.SortMethod = (g.config.SortMethod + 1) % 3
	g.showOverlayMessage("Sort: " + getSortMethodName(g.config.SortMethod))
	g.reloadPathsForCurrentSource()
}

func (g *Game) expandToDirectoryAndJump() {
	// Only work when launched with a single regular image and not yet expanded.
	if g.launchSingleFile == "" || g.collectionSource.Mode == CollectionSourceExpandedSingleDirectory {
		return
	}

	originalFilePath := g.launchSingleFile

	newPaths, err := collectImagesFromSameDirectory(originalFilePath, g.config.SortMethod)
	if err != nil {
		g.showOverlayMessage(fmt.Sprintf("Failed to scan directory: %v", err))
		return
	}

	if len(newPaths) == 0 {
		g.showOverlayMessage("No images found in directory")
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
		return
	}

	g.imageManager.SetPaths(newPaths)
	g.collectionSource = newExpandedDirectorySource(originalFilePath)
	g.idx = originalFileIndex
	g.showOverlayMessage(fmt.Sprintf("Loaded %d images from directory", len(newPaths)))
	g.calculateDisplayContent()
}
