package main

import "github.com/hajimehoshi/ebiten/v2"

type stubImageManager struct {
	paths             []ImagePath
	images            []*ebiten.Image
	preloadDirections []NavigationDirection
}

func (m *stubImageManager) GetImage(idx int) *ebiten.Image {
	if idx < 0 || idx >= len(m.images) {
		return nil
	}
	return m.images[idx]
}

func (m *stubImageManager) GetBookModeImages(idx int, rightToLeft bool) (*ebiten.Image, *ebiten.Image) {
	if rightToLeft {
		return m.GetImage(idx + 1), m.GetImage(idx)
	}
	return m.GetImage(idx), m.GetImage(idx + 1)
}

func (m *stubImageManager) GetPath(idx int) (ImagePath, bool) {
	if idx < 0 || idx >= len(m.paths) {
		return ImagePath{}, false
	}
	return m.paths[idx], true
}

func (m *stubImageManager) SetPaths(paths []ImagePath) {
	m.paths = make([]ImagePath, len(paths))
	copy(m.paths, paths)
}

func (m *stubImageManager) GetPathsCount() int {
	return len(m.paths)
}

func (m *stubImageManager) StartPreload(currentIdx int, direction NavigationDirection) {
	m.preloadDirections = append(m.preloadDirections, direction)
}

func (m *stubImageManager) StopPreload() {}

func (m *stubImageManager) GetPreloadStats() PreloadStats {
	return PreloadStats{}
}

func (m *stubImageManager) ConsumeAsyncRefresh() bool {
	return false
}
