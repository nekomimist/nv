# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an image viewer application built using Go and the Ebiten game engine. The application provides a simple interface for viewing image files with keyboard navigation and fullscreen support.

## Key Components

- **Main Application**: Single-file application in `main.go` that implements the entire image viewer
- **Game Loop**: Uses Ebiten's game interface with `Update()`, `Draw()`, and `Layout()` methods
- **Image Loading**: Supports PNG, JPEG, WebP, BMP, and GIF formats
- **Navigation**: Keyboard-based navigation between images with fullscreen toggle

## Development Commands

```bash
# Build the application
go build

# Run the application with image files/directories
go run main.go [image_files_or_directories...]

# Run with specific images
go run main.go image1.png image2.jpg

# Run with a directory of images
go run main.go ./images/

# Test the code
go test ./...

# Format code
go fmt

# Vet code for common mistakes
go vet

# Get dependencies
go mod tidy
```

## Application Controls

- **Space/N**: Next image (2 images in book mode)
- **Backspace/P**: Previous image (2 images in book mode)
- **Shift+Space/Shift+N**: Single page forward (for fine adjustment in book mode)
- **Shift+Backspace/Shift+P**: Single page backward (for fine adjustment in book mode)
- **B**: Toggle book mode (spread view - displays 2 images side by side)
- **Shift+B**: Toggle reading direction (left-to-right ↔ right-to-left)
- **Z**: Toggle fullscreen
- **Escape/Q**: Quit application

## Architecture Notes

- Single `Game` struct implements the Ebiten game interface
- Lazy loading with intelligent image cache (keeps 3-4 images in memory)
- `collectImages()` recursively finds supported image files in directories
- Book mode displays two images side by side for spread viewing
- Configurable reading direction (left-to-right Western style or right-to-left Japanese manga style)
- Intelligent aspect ratio detection in book mode (fallback to single page for mismatched ratios)
- Window scaling logic handles both windowed and fullscreen modes
- Window size persistence using JSON config file at `~/.nv.json`
- Configurable aspect ratio threshold for book mode compatibility

## Configuration

The application saves settings to `~/.nv.json`:

```json
{
  "window_width": 800,
  "window_height": 600,
  "aspect_ratio_threshold": 1.5,
  "right_to_left": false
}
```

- **aspect_ratio_threshold**: Controls when to use single page mode in book mode. Higher values allow more different aspect ratios to be displayed side-by-side. Default: 1.5
- **right_to_left**: Reading direction for book mode. `false` for left-to-right (Western style), `true` for right-to-left (Japanese manga style). Default: false