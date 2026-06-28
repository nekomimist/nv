package main

import "github.com/hajimehoshi/ebiten/v2"

type stubImageManager struct {
	paths             []ImagePath
	images            []DisplayImage
	preloadDirections []NavigationDirection
}

func testDisplayImage(w, h int) DisplayImage {
	return createDisplayImageFromEbitenImage(ebiten.NewImage(w, h))
}

func testDisplayImages(images ...*ebiten.Image) []DisplayImage {
	result := make([]DisplayImage, 0, len(images))
	for _, img := range images {
		result = append(result, createDisplayImageFromEbitenImage(img))
	}
	return result
}

func (m *stubImageManager) GetImage(idx int) DisplayImage {
	if idx < 0 || idx >= len(m.images) {
		return nil
	}
	return m.images[idx]
}

func (m *stubImageManager) GetBookModeImages(idx int, rightToLeft bool) (DisplayImage, DisplayImage) {
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
