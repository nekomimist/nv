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
	"runtime"
	"strings"

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

// ImageManager interface for managing image loading and caching
type ImageManager interface {
	GetImage(idx int) *ebiten.Image
	GetCurrentImage(idx int) *ebiten.Image
	GetBookModeImages(idx int, rightToLeft bool) (*ebiten.Image, *ebiten.Image)
	SetPaths(paths []ImagePath)
	GetPathsCount() int
}

// DefaultImageManager implements ImageManager
type DefaultImageManager struct {
	paths []ImagePath
	cache *lru.Cache[string, *ebiten.Image]
}

// NewImageManager creates a new DefaultImageManager
func NewImageManager(cacheSize int) ImageManager {
	cache, err := lru.New[string, *ebiten.Image](cacheSize)
	if err != nil {
		log.Printf("Error: Failed to create LRU cache: %v", err)
		cache, _ = lru.New[string, *ebiten.Image](16) // fallback to default size
	}
	return &DefaultImageManager{
		paths: []ImagePath{},
		cache: cache,
	}
}

func (m *DefaultImageManager) SetPaths(paths []ImagePath) {
	m.paths = paths
	// No need to clear cache since we use file paths as keys
	debugLog("SetPaths: %d new paths, cache preserved (%d items)", len(paths), m.cache.Len())
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

	imagePath := m.paths[idx]
	cacheKey := imagePath.Path

	// Check if image is already in cache
	if img, ok := m.cache.Get(cacheKey); ok {
		debugLog("Cache HIT: %s (cache: %d items)", cacheKey, m.cache.Len())
		return img
	}

	// Load image on demand
	img, err := loadImage(imagePath)
	if err != nil {
		log.Printf("Error: Failed to load image [%d/%d] %s: %v",
			idx+1, len(m.paths), imagePath.Path, err)
		return nil
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
