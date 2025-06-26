# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an image viewer application built using Go and the Ebiten game engine. The application provides a simple interface for viewing image files with keyboard navigation and fullscreen support.

## File Structure

The application is organized into three main modules for maintainability:

### `main.go` (470+ lines)
- **Game Loop**: Implements Ebiten's game interface (`Update()`, `Draw()`, `Layout()`)
- **User Interface**: Handles keyboard input and window management
- **Navigation Logic**: Image index management and book mode navigation
- **Rendering**: Single image and book mode drawing functions
- **Help System**: Interactive help overlay with configurable font rendering
- **Application Entry Point**: Command-line argument processing and initialization

### `image.go` (397 lines)
- **ImageManager Interface**: Abstraction for image loading and caching
- **Image Loading**: Supports PNG, JPEG, WebP, BMP, and GIF formats
- **Archive Support**: Complete ZIP and RAR archive processing
- **Intelligent Caching**: LRU-style cache with preloading for performance
- **File Collection**: Recursive directory scanning and archive detection

### `config.go` (96+ lines)
- **Configuration Management**: JSON-based settings persistence
- **Validation**: Input validation and default value handling
- **Window State**: Size and aspect ratio threshold management
- **Reading Direction**: Book mode orientation settings
- **Font Configuration**: Help overlay font size settings with validation

## Key Components

- **Modular Architecture**: Clear separation of concerns across three files
- **Interface-Based Design**: ImageManager enables dependency injection and testing
- **Archive Integration**: Seamless ZIP/RAR support with automatic image detection
- **Performance Optimization**: Intelligent caching and preloading strategies

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

# Run with archive files
go run main.go images.zip manga.rar

# Test the code
go test ./...

# Format code
go fmt

# Vet code for common mistakes
go vet

# Get dependencies
go mod tidy

# Run specific tests for individual modules
go test -run TestImageManager     # Test image management
go test -run TestConfig          # Test configuration
go test -run TestGameNavigation  # Test navigation logic
```

## Development Notes

### Code Organization
This codebase has been extensively refactored for maintainability:
- **Started**: 764 lines in a single `main.go` file
- **Phase 1**: Eliminated duplicate archive processing code
- **Phase 2**: Extracted configuration management to `config.go`
- **Phase 3**: Separated image management into `image.go` with proper interfaces
- **Result**: Clean 3-file architecture with 52% reduction in main.go size

### Testing Strategy
- **Unit Tests**: Each module can be tested independently
- **Interface Mocking**: ImageManager interface enables easy test doubles
- **Integration Tests**: Full application flow tested with real archives
- **Performance Tests**: Cache behavior and memory usage validation

## Application Controls

### Navigation
- **Space/N**: Next image (2 images in book mode)
- **Backspace/P**: Previous image (2 images in book mode)
- **Shift+Space/Shift+N**: Single page forward (for fine adjustment in book mode)
- **Shift+Backspace/Shift+P**: Single page backward (for fine adjustment in book mode)

### Display Modes
- **B**: Toggle book mode (spread view - displays 2 images side by side)
- **Shift+B**: Toggle reading direction (left-to-right ↔ right-to-left)
- **Z**: Toggle fullscreen

### Other
- **H**: Show/hide help overlay with all controls
- **Escape/Q**: Quit application

## Architecture Notes

### Design Principles
- **Separation of Concerns**: Each file handles a distinct responsibility
- **Interface-Driven**: ImageManager interface enables testing and future extensibility
- **Dependency Injection**: Game struct receives ImageManager rather than creating it
- **Clean Architecture**: Business logic separated from UI and infrastructure

### Core Features
- **Game Loop**: `Game` struct implements Ebiten's game interface in main.go
- **Image Management**: `ImageManager` interface abstracts loading, caching, and archive processing
- **Intelligent Caching**: Keeps 3-4 images in memory with LRU-style eviction
- **Archive Support**: Transparent ZIP/RAR handling with automatic image detection
- **Book Mode**: Dual image display with configurable reading direction
- **Aspect Ratio Intelligence**: Automatic fallback to single page for mismatched ratios
- **Help System**: Interactive H key overlay with column-aligned controls display
- **Configuration Persistence**: JSON-based settings with validation including font preferences

### Performance Optimizations
- **Lazy Loading**: Images loaded on-demand with intelligent preloading
- **Cache Strategy**: Adjacent images preloaded for smooth navigation
- **Memory Management**: Automatic cache cleanup when limits exceeded
- **File System Efficiency**: Single-pass directory traversal with archive detection

## Configuration

The application saves settings to `~/.nv.json`:

```json
{
  "window_width": 800,
  "window_height": 600,
  "aspect_ratio_threshold": 1.5,
  "right_to_left": false,
  "help_font_size": 24.0
}
```

- **aspect_ratio_threshold**: Controls when to use single page mode in book mode. Higher values allow more different aspect ratios to be displayed side-by-side. Default: 1.5
- **right_to_left**: Reading direction for book mode. `false` for left-to-right (Western style), `true` for right-to-left (Japanese manga style). Default: false
- **help_font_size**: Font size for help overlay text. Must be > 12px for readability. Default: 24.0

## Help System

The application features an interactive help overlay accessible via the **H** key:

### Features
- **Dark Transparent Overlay**: Semi-transparent black background allowing images to show through
- **Column-Aligned Layout**: Right-aligned keys with left-aligned descriptions for clean presentation
- **Configurable Font**: Font size controlled via `help_font_size` setting in config file
- **Lightweight Font**: Uses Go's built-in goregular font for smaller binary size
- **Organized Sections**: Controls grouped by function (Navigation, Display Modes, Other)
- **Toggle Interface**: Press H to show, H again to hide

### Design
- Background: Black with 50% transparency for image visibility
- Text Area: Black with 37% transparency for readability
- Font: White goregular font with configurable size
- Layout: Structured two-column format with 50px spacing between keys and descriptions