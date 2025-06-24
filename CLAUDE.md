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

- **Space/N**: Next image
- **Backspace/P**: Previous image  
- **Z**: Toggle fullscreen
- **Escape/Q**: Quit application

## Architecture Notes

- Single `Game` struct implements the Ebiten game interface
- Images are loaded into memory at startup using `loadImage()` function
- `collectImages()` recursively finds supported image files in directories
- Window scaling logic handles both windowed and fullscreen modes
- No external configuration files - all settings are hardcoded defaults