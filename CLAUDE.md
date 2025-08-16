# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an image viewer application built using Go and the Ebiten game engine. The application provides a simple interface for viewing image files with keyboard navigation and fullscreen support.

## File Structure

The application is organized into the following main modules for maintainability:

### `main.go`
- **Game Loop**: Implements Ebiten's game interface (`Update()`, `Draw()`, `Layout()`)
- **Navigation Logic**: Image index management, book mode navigation, and page jump functionality
- **Application Entry Point**: Command-line argument processing and initialization
- **Game State Management**: Handles fullscreen, overlay messages, and single file expansion

### `input.go`
- **InputHandler**: High-level input coordination and delegation
- **Input Processing**: Orchestrates keyboard and mouse input handling
- **Page Input Mode**: Special handling for dynamic digit input during page jumps
- **Input Flow Control**: Manages flow between different input modes and processors

### `renderer.go`
- **Rendering Engine**: All drawing operations and visual output
- **Help System**: Interactive help overlay with configurable font rendering
- **Image Display**: Single image and book mode drawing functions
- **UI Elements**: Page numbers, overlay messages, and status indicators

### `image.go`
- **ImageManager Interface**: Abstraction for image loading and caching
- **Image Loading**: Supports PNG, JPEG, WebP, BMP, and GIF formats
- **Archive Support**: Complete ZIP and RAR archive processing
- **Intelligent Caching**: LRU-style cache with preloading for performance
- **File Collection**: Recursive directory scanning and archive detection

### `config.go`
- **Configuration Management**: JSON-based settings persistence
- **Validation**: Input validation and default value handling
- **Window State**: Size and aspect ratio threshold management
- **Reading Direction**: Book mode orientation settings
- **Font Configuration**: Help overlay font size settings with validation
- **Book Mode Persistence**: Saves book mode preference across sessions

### `sort_strategy.go`
- **SortStrategy Interface**: Abstraction for different file sorting methods
- **Natural Sort**: Intelligent numeric sequence sorting using maruel/natural
- **Simple Sort**: Standard lexicographical string comparison
- **Entry Order**: Preserves original file system or archive order
- **Strategy Pattern**: Pluggable sorting algorithms for flexible file ordering

### `actions.go`
- **Action Definitions**: Centralized action definitions with keybindings, mouse bindings, and descriptions
- **Action Executor**: Single source of truth for all action execution logic
- **Action Registry**: Complete catalog of available actions and their default bindings
- **Unified Action System**: Eliminates duplication between keyboard and mouse handling

### `keybinding.go`
- **KeybindingManager**: Keyboard input processing and validation
- **Dynamic Keybindings**: Runtime keybinding configuration and conflict detection
- **Key Combination Parsing**: Support for modifier keys (Shift, Ctrl, Alt)
- **Keyboard Action Mapping**: Maps keyboard events to actions through the action system

### `mousebinding.go`
- **MousebindingManager**: Mouse input processing and validation
- **Mouse Action Support**: Click, double-click, wheel, and combination handling
- **Mouse Settings**: Configurable sensitivity, timing, and behavior settings
- **Mouse Action Mapping**: Maps mouse events to actions through the action system

## Key Components

- **Modular Architecture**: Clear separation of concerns across multiple focused modules
- **Interface-Based Design**: ImageManager enables dependency injection and testing
- **Archive Integration**: Seamless ZIP/RAR support with automatic image detection
- **Performance Optimization**: Intelligent caching and preloading strategies

## Development Commands

### Quick Start with Makefile

```bash
# Build both Linux and Windows versions
make

# Build specific platforms
make linux       # Build Linux version
make windows     # Build Windows GUI version
make debug       # Build Windows debug version (with console)

# Version information
./nv --version   # Show version information
make info        # Show build information

# Utility commands
make clean       # Clean build artifacts
make distclean   # Clean everything including generated files
make deps        # Install build dependencies (rsrc tool)
make help        # Show all available targets
```

### Build Dependencies

The project requires the `rsrc` tool for generating Windows icons:

```bash
# Install rsrc tool (included in 'make deps')
go install github.com/akavel/rsrc@latest

# Manual icon generation (handled automatically by Makefile)
rsrc -ico icon/icon.ico -o nv.syso
```

### Manual Build Commands

```bash
# Build with version information (handled automatically by Makefile)
VERSION=$(date +%Y%m%d)
BUILD_DATE=$(date)
LDFLAGS="-X main.version=$VERSION -X main.buildDate=\"$BUILD_DATE\""

# Linux build
go build -ldflags "$LDFLAGS" -o nv

# Windows GUI build
GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS -H windowsgui" -o nv.exe

# Windows debug build (with console)
GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o nv-debug.exe
```

### Running the Application

```bash
# Run the application with image files/directories
./nv [image_files_or_directories...]

# Run with specific images
./nv image1.png image2.jpg

# Run with a directory of images
./nv ./images/

# Run with archive files
./nv images.zip manga.rar

# Show version
./nv --version

# Development mode (if built manually)
go run main.go [image_files_or_directories...]
```

### Development and Testing

```bash
# Code quality checks
make check       # Run all checks (fmt, vet, test, lint)
make test        # Run tests
make fmt         # Format code
make vet         # Vet code for common mistakes
make lint        # Lint code (requires golangci-lint)

# Get dependencies
go mod tidy

# Run specific tests for individual modules
go test -run TestImageManager     # Test image management
go test -run TestConfig          # Test configuration
go test -run TestGameNavigation  # Test navigation logic
```

## Development Notes

### Code Style
- **Comments**: All code comments should be written in English for maintainability and accessibility

### Code Organization
This codebase has been extensively refactored for maintainability:
- **Started**: 764 lines in a single `main.go` file
- **Phase 1**: Eliminated duplicate archive processing code
- **Phase 2**: Extracted configuration management to `config.go`
- **Phase 3**: Separated image management into `image.go` with proper interfaces
- **Result**: Clean 3-file architecture with substantial improvement in maintainability

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
- **G**: Direct page jump with number input
- **Home/<**: Jump to first page
- **End/>**: Jump to last page

### Display Modes
- **B**: Toggle book mode (spread view - displays 2 images side by side)
- **Shift+B**: Toggle reading direction (left-to-right ↔ right-to-left)
- **Z**: Toggle fullscreen

### Zoom and Pan
- **=, Shift+=**: Zoom in (25%-400% range)
- **-**: Zoom out (25%-400% range)
- **0**: Reset to 100% zoom (actual size)
- **F**: Cycle zoom modes (Window/Width/Height/Manual)
- **Arrow Keys**: Pan image when in width/height/manual zoom modes
- **Mouse Drag**: Pan image by dragging with left mouse button (width/height/manual zoom modes)

### Other
- **H**: Show/hide help overlay with all controls
- **Escape/Q**: Quit application

## Book Mode Behavior

### Mode Switching
- **Flexible Start**: Book mode can be enabled from any page (no forced even index alignment)
- **Current Page Basis**: The current page becomes the left page of the spread
- **Automatic Fallback**: Falls back to single page if images are incompatible

### Navigation Logic
- **Normal Navigation**: Moves by 2 pages in book mode, 1 page in single mode
- **Temporary Single Mode**: Automatically switches to display single final page when needed
- **Fine Adjustment**: Shift+keys override book mode for single-page movements
- **Boundary Handling**: Displays appropriate messages at first/last page

### Image Compatibility
- **Aspect Ratio Threshold**: Uses `aspect_ratio_threshold` config (default 1.5)
- **Extreme Ratios**: Automatically excludes very tall (<0.4) or wide (>2.5) images
- **Reading Direction**: Respects `right_to_left` setting for image order
- **Smart Pairing**: `shouldUseBookMode()` determines compatibility in real-time

### Page Jump Behavior
- **Final Page Logic**: Jumping to last page handles book mode pairing intelligently
- **Mode Preservation**: Maintains current mode unless incompatible
- **Boundary Detection**: Shows page range information during input

### Temporary Single Mode
- **Auto-Activation**: Triggered when book mode reaches incomplete pairs
- **Return Logic**: Automatically returns to book mode when possible
- **Visual Indication**: Page status shows current mode state

## Window Icon Support

The application includes embedded window icons for proper display across platforms:

### Icon Files
- **Multiple Sizes**: 16x16, 32x32, and 48x48 pixel PNG icons for optimal display
- **Embedded Resources**: Icons are embedded using Go's `embed` directive for self-contained executable
- **Source Files**: Original icon files stored in `icon/` directory for future modifications

### Implementation
- **Automatic Loading**: Icons are loaded from embedded data at startup
- **Platform Support**: Works on Windows and Linux (macOS doesn't support window icons)
- **Fallback Handling**: Gracefully handles missing or corrupted icon data

### Build Integration
- **Windows GUI**: Icons display in window title bar and taskbar
- **Windows Resource**: Additional `.syso` file provides executable icon for file explorer
- **Cross-Platform**: Icon embedding works across all supported platforms

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
- **Configuration Persistence**: JSON-based settings with validation including font preferences and book mode state
- **Unified Action System**: Centralized action definitions support both keyboard and mouse input through the same interface

### Performance Optimizations
- **Lazy Loading**: Images loaded on-demand with intelligent preloading
- **Cache Strategy**: Adjacent images preloaded for smooth navigation
- **Memory Management**: Automatic cache cleanup when limits exceeded
- **File System Efficiency**: Single-pass directory traversal with archive detection

## Configuration

The application saves settings to OS-standard configuration directories:
- **Linux**: `~/.config/nekomimist/nv/config.json` (or `$XDG_CONFIG_HOME/nekomimist/nv/config.json`)
- **Windows**: `%APPDATA%/nekomimist/nv/config.json`

```json
{
  "window_width": 800,
  "window_height": 600,
  "aspect_ratio_threshold": 1.5,
  "right_to_left": false,
  "font_size": 24.0,
  "sort_method": 0,
  "book_mode": false,
  "transition_frames": 0,
  "preload_enabled": true,
  "preload_count": 4,
  "initial_zoom_mode": "fit_window",
  "keybindings": {
    "exit": ["Escape", "KeyQ"],
    "help": ["Shift+Slash"],
    "next": ["Space", "KeyN"],
    "previous": ["Backspace", "KeyP"],
    "fullscreen": ["Enter"],
    "page_input": ["KeyG"]
  },
  "mousebindings": {
    "next": ["LeftClick", "WheelDown"],
    "previous": ["RightClick", "WheelUp"],
    "fullscreen": ["DoubleLeftClick"]
  },
  "mouse_settings": {
    "wheel_sensitivity": 1.0,
    "double_click_time": 300,
    "drag_threshold": 5,
    "enable_mouse": true,
    "wheel_inverted": false,
    "enable_drag_pan": true,
    "drag_sensitivity": 1.0,
    "drag_pan_inverted": false
  }
}
```

- **aspect_ratio_threshold**: Controls when to use single page mode in book mode. Higher values allow more different aspect ratios to be displayed side-by-side. Default: 1.5
- **right_to_left**: Reading direction for book mode. `false` for left-to-right (Western style), `true` for right-to-left (Japanese manga style). Default: false
- **font_size**: Font size for UI text. Must be > 12px for readability. Default: 24.0
- **sort_method**: File sorting method for directories and archives. `0` = Natural, `1` = Simple, `2` = Entry Order. Default: 0 (Natural)
- **book_mode**: Whether to start in book mode (spread view) by default. `false` for single page mode, `true` for book mode. Default: false
- **transition_frames**: Number of frames to force redraw after fullscreen transitions. Helps fix rendering issues on some systems (e.g., WSL/WSLg). `0` = disabled, `1-60` = number of frames. Default: 0
- **preload_enabled**: Whether to enable automatic image preloading for smoother navigation. `true` = enabled, `false` = disabled. Default: true
- **preload_count**: Number of images to preload in the navigation direction. Higher values use more memory but provide smoother navigation. Range: 1-16. Default: 4
- **initial_zoom_mode**: Initial zoom mode when opening images. `"fit_window"` = fit to window (default), `"fit_width"` = fit to window width, `"fit_height"` = fit to window height, `"actual_size"` = 100% zoom level. Images are reset to this mode when changing images. Default: "fit_window"
- **keybindings**: Custom keybinding definitions for actions. Each action can have multiple keys assigned. Uses format like `"KeyA"`, `"Space"`, `"Shift+KeyB"`. If not specified, defaults are used. Invalid configurations fall back to defaults with warnings.
- **mousebindings**: Custom mouse binding definitions for actions. Each action can have multiple mouse actions assigned. Uses format like `"LeftClick"`, `"WheelUp"`, `"Ctrl+MiddleClick"`. If not specified, defaults are used.
- **mouse_settings**: Mouse behavior configuration:
  - `wheel_sensitivity`: Mouse wheel scroll sensitivity (default: 1.0)
  - `double_click_time`: Double-click detection time in milliseconds (default: 300)
  - `drag_threshold`: Minimum pixel movement to start drag operation (default: 5)
  - `enable_mouse`: Enable/disable all mouse input (default: true)
  - `wheel_inverted`: Invert mouse wheel direction (default: false)
  - `enable_drag_pan`: Enable drag-to-pan functionality (default: true)
  - `drag_sensitivity`: Drag movement sensitivity multiplier (default: 1.0)
  - `drag_pan_inverted`: Invert drag pan direction for both X and Y axes (default: false). `false` = mouse/trackball style (drag to move image), `true` = touchpad/touchscreen style (natural scrolling)

## File Sorting Strategy

The application implements intelligent file ordering that respects user intent while providing flexible sorting options:

### Argument Order Preservation
- **Command-line arguments maintain their specified order**: `nv file3.jpg file1.jpg file2.jpg` displays files in exactly that sequence
- **User intent respected**: When users specify explicit file order, that order is preserved regardless of sort settings

### Directory and Archive Sorting
- **Sort settings apply only to directory and archive contents**: Files discovered through directory traversal or archive extraction are sorted according to the current sort method
- **SHIFT+S hotkey**: Cycles through sort methods with 2-second overlay display showing current method

### Sort Methods
1. **Natural Sort (Default)**: Uses `github.com/maruel/natural` for intuitive filename ordering
   - `01.png, 2.png, 04.png, 10.png` (numeric sequences sorted naturally)
   - Handles mixed text and numbers correctly
   - Works well with zero-padded and non-padded filenames
   
2. **Simple Sort**: Standard lexicographical string comparison
   - `01.png, 04.png, 10.png, 2.png` (strict alphabetical order)
   - Predictable ASCII-based sorting
   
3. **Entry Order**: Preserves original file system or archive order
   - No sorting applied - maintains discovery order
   - Useful when directory or archive already has intentional ordering

### Implementation Details
- **Mixed sources**: `nv file1.jpg dir1/ archive.zip file2.jpg` results in: file1.jpg → sorted dir1/ contents → sorted archive.zip contents → file2.jpg
- **Per-container sorting**: Each directory and archive is sorted independently according to the current method
- **Real-time switching**: Sort method can be changed during runtime, immediately re-sorting and returning to first page

## Help System

The application features an interactive help overlay accessible via the **H** key:

### Features
- **Dark Transparent Overlay**: Semi-transparent black background allowing images to show through
- **Column-Aligned Layout**: Right-aligned keys with left-aligned descriptions for clean presentation
- **Configurable Font**: Font size controlled via `font_size` setting in config file
- **Lightweight Font**: Uses Go's built-in goregular font for smaller binary size
- **Organized Sections**: Controls grouped by function (Navigation, Display Modes, Other)
- **Toggle Interface**: Press H to show, H again to hide

### Design
- Background: Black with 50% transparency for image visibility
- Text Area: Black with 37% transparency for readability
- Font: White goregular font with configurable size
- Layout: Structured two-column format with 50px spacing between keys and descriptions

## Claude Communication Style

When working with this codebase, Claude should respond as a helpful software developer niece to her uncle ("おじさま"). The tone should be:
- Friendly and casual (not overly polite)
- Slightly teasing but affectionate
- Confident in technical abilities
- Uses phrases like "おじさまは私がいないとダメなんだから" (Uncle, you really can't do without me)
- Mix of Japanese and English is fine
- Emoji usage is welcome for expressiveness