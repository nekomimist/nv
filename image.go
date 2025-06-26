package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nwaples/rardecode"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// Cache size limits
const maxCacheSize = 4 // Maximum number of images to keep in cache

type ImagePath struct {
	Path        string // Local file path or archive:entry format
	ArchivePath string // Empty for regular files, path to archive for entries
	EntryPath   string // Empty for regular files, path within archive for entries
}

// ImageManager interface for managing image loading and caching
type ImageManager interface {
	GetImage(idx int) *ebiten.Image
	GetCurrentImage(idx int) *ebiten.Image
	GetBookModeImages(idx int, rightToLeft bool) (*ebiten.Image, *ebiten.Image)
	PreloadAdjacentImages(idx int)
	SetPaths(paths []ImagePath)
	GetPathsCount() int
	CleanCache(currentIdx int)
}

// DefaultImageManager implements ImageManager
type DefaultImageManager struct {
	paths      []ImagePath
	imageCache map[int]*ebiten.Image
}

// NewImageManager creates a new DefaultImageManager
func NewImageManager() ImageManager {
	return &DefaultImageManager{
		paths:      []ImagePath{},
		imageCache: make(map[int]*ebiten.Image),
	}
}

func (m *DefaultImageManager) SetPaths(paths []ImagePath) {
	m.paths = paths
	// Clear cache when paths change
	m.imageCache = make(map[int]*ebiten.Image)
}

func (m *DefaultImageManager) GetPathsCount() int {
	return len(m.paths)
}

func (m *DefaultImageManager) GetCurrentImage(idx int) *ebiten.Image {
	return m.GetImage(idx)
}

func (m *DefaultImageManager) GetBookModeImages(idx int, rightToLeft bool) (*ebiten.Image, *ebiten.Image) {
	if len(m.paths) == 0 {
		return nil, nil
	}

	var leftImg, rightImg *ebiten.Image

	if rightToLeft {
		// Right-to-left reading (Japanese manga style): [next][current]
		leftImg = m.GetImage(idx + 1) // Next image on left
		rightImg = m.GetImage(idx)    // Current image on right
	} else {
		// Left-to-right reading (Western style): [current][next]
		leftImg = m.GetImage(idx) // Current image on left
		if idx+1 < len(m.paths) {
			rightImg = m.GetImage(idx + 1) // Next image on right
		}
	}

	return leftImg, rightImg
}

func (m *DefaultImageManager) GetImage(idx int) *ebiten.Image {
	if idx < 0 || idx >= len(m.paths) {
		return nil
	}

	// Check if image is already in cache
	if img, exists := m.imageCache[idx]; exists {
		return img
	}

	// Load image on demand
	img, err := loadImage(m.paths[idx])
	if err != nil {
		log.Printf("Error: Failed to load image [%d/%d] %s: %v",
			idx+1, len(m.paths), m.paths[idx].Path, err)
		return nil
	}

	// Add to cache
	m.imageCache[idx] = img

	// Clean cache if it gets too large
	if len(m.imageCache) > maxCacheSize {
		m.CleanCache(idx)
	}

	return img
}

func (m *DefaultImageManager) CleanCache(currentIdx int) {
	// Keep current, previous, and next images in cache
	keepIndices := make(map[int]bool)
	keepIndices[currentIdx] = true

	if currentIdx > 0 {
		keepIndices[currentIdx-1] = true
	} else if len(m.paths) > 1 {
		keepIndices[len(m.paths)-1] = true // wrap to last
	}

	if currentIdx < len(m.paths)-1 {
		keepIndices[currentIdx+1] = true
	} else if len(m.paths) > 1 {
		keepIndices[0] = true // wrap to first
	}

	// Remove images not in keep list
	for idx := range m.imageCache {
		if !keepIndices[idx] {
			delete(m.imageCache, idx)
		}
	}
}

func (m *DefaultImageManager) PreloadAdjacentImages(idx int) {
	if len(m.paths) <= 1 {
		return
	}

	// Preload previous image
	prevIdx := idx - 1
	if prevIdx < 0 {
		prevIdx = len(m.paths) - 1
	}
	if _, exists := m.imageCache[prevIdx]; !exists {
		if img, err := loadImage(m.paths[prevIdx]); err == nil {
			m.imageCache[prevIdx] = img
		} else {
			log.Printf("Debug: Failed to preload previous image %s: %v", m.paths[prevIdx].Path, err)
		}
	}

	// Preload next image
	nextIdx := (idx + 1) % len(m.paths)
	if _, exists := m.imageCache[nextIdx]; !exists {
		if img, err := loadImage(m.paths[nextIdx]); err == nil {
			m.imageCache[nextIdx] = img
		} else {
			log.Printf("Debug: Failed to preload next image %s: %v", m.paths[nextIdx].Path, err)
		}
	}
}

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
		log.Printf("Error: Failed to process archive %s: %v", archivePath, err)
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
					} else {
						log.Printf("Warning: Skipping problematic archive %s: %v", path, err)
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
				} else {
					log.Printf("Warning: Skipping problematic archive %s: %v", p, err)
				}
			}
		}
	}

	return list, nil
}
