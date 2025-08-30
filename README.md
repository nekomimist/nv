# NV - Image Viewer

A simple image viewer built with Go and Ebiten, featuring seamless archive support and intelligent book mode for manga reading.

## Features

- Multiple Format Support: PNG, JPEG, WebP, BMP, GIF
- Archive Integration: Direct ZIP, RAR, and 7Z file viewing
- Book Mode: Side-by-side image display with configurable reading direction
- Manual Zoom & Pan: Zoom in/out with mouse wheel or keyboard, pan with mouse drag or arrow keys
- Fullscreen Support: Toggle between windowed and fullscreen modes
- Page Jump: Direct navigation to specific pages
- Mouse Support: Full mouse navigation with configurable bindings and drag-to-pan
- Customizable Controls: Configure keyboard shortcuts and mouse bindings via JSON settings

## Usage

```bash
# View images in current directory
./nv .

# View specific images
./nv image1.png image2.jpg

# View images from archive
./nv manga.zip photos.rar collection.7z

# View images from multiple sources
./nv ./photos/ manga.zip single_image.png
```

## Controls

### Navigation
- `Space` / `N` - Next image (2 pages in book mode)
- `Backspace` / `P` - Previous image (2 pages in book mode)
- `Shift+Space` / `Shift+N` - Single page forward
- `Shift+Backspace` / `Shift+P` - Single page backward
- `G` - Jump to specific page
- `Home` / `<` - First page
- `End` / `>` - Last page

### Display Modes
- `B` - Toggle book mode (side-by-side view)
- `Shift+B` - Toggle reading direction (LTR ↔ RTL)
- `Enter` - Toggle fullscreen

### Zoom and Pan
- `=` / `Shift+=` - Zoom in (25%-400%)
- `-` - Zoom out (25%-400%)
- `0` - Reset to 100% zoom
- `F` - Cycle zoom modes (Window/Width/Height/Manual)
- `Arrow Keys` - Pan image (width/height/manual zoom modes)

### Mouse Controls
- `Left Click` - Next image (or drag to pan in width/height/manual zoom modes)
- `Right Click` - Previous image
- `Double Left Click` - Toggle fullscreen
- `Mouse Wheel` - Navigate images (or zoom with Ctrl modifier)
- `Mouse Drag` - Pan image (width/height/manual zoom modes)

### Other
- `H` - Show/hide help overlay
- `Escape` / `Q` - Quit

## Book Mode

Book mode displays two images side-by-side, perfect for reading manga or viewing photo spreads:

- Flexible Start: Can be enabled from any page
- Smart Pairing: Automatically handles aspect ratio compatibility
- Reading Direction: Supports both left-to-right and right-to-left modes
- Automatic Fallback: Falls back to single page when needed

## Installation

```bash
# Clone the repository
git clone https://github.com/nekomimist/nv.git
cd nv

# Build the application
go build

# Or run directly
go run . [image_files_or_directories...]
```

## Requirements

- Go 1.24 or later
- Platform support: Windows, Linux (macOS untested)

## Configuration

Settings are automatically saved to OS-standard configuration directories:
- Linux: `~/.config/nekomimist/nv/config.json` (or `$XDG_CONFIG_HOME/nekomimist/nv/config.json`)
- Windows: `%APPDATA%/nekomimist/nv/config.json`

```json
{
  "window_width": 800,
  "window_height": 600,
  "aspect_ratio_threshold": 1.5,
  "right_to_left": false,
  "font_size": 24.0,
  "transition_frames": 0,
  "preload_enabled": true,
  "preload_count": 4,
  "initial_zoom_mode": "fit_window",
  "fit_width_align_top": false,
  "fit_height_align_left": false,
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
    "enable_drag_pan": true,
    "drag_sensitivity": 1.0,
    "drag_threshold": 5,
    "drag_pan_inverted": false
  }
}
```

- `aspect_ratio_threshold`: Controls book mode compatibility (default: 1.5)
- `right_to_left`: Reading direction for book mode (default: false)
- `font_size`: UI/help overlay font size (default: 24.0)
- `initial_zoom_mode`: `"fit_window"` (default), `"fit_width"`, `"fit_height"`, or `"actual_size"`
- `fit_width_align_top`: When `true`, FitWidth shows the image's top edge (align top) instead of center
- `fit_height_align_left`: When `true`, FitHeight shows the image's left edge (align left) instead of center
- `transition_frames`: Force redraw frames after fullscreen transitions (default: 0)
- `preload_enabled`: Enable automatic image preloading (default: true)
- `preload_count`: Number of images to preload ahead (1–16, default: 4)
- `keybindings`: Custom keyboard shortcuts for actions. Use `"KeyA"`, `"Space"`, `"Shift+KeyB"` format
- `mousebindings`: Custom mouse bindings. Use `"LeftClick"`, `"WheelUp"`, `"Ctrl+MiddleClick"`
- `mouse_settings`: Mouse behavior (drag-to-pan, sensitivity, thresholds)
  - `enable_drag_pan`: Enable drag-to-pan (default: true)
  - `drag_sensitivity`: Drag movement sensitivity multiplier (default: 1.0)
  - `drag_threshold`: Minimum pixel movement to start drag (default: 5)
  - `drag_pan_inverted`: Invert drag pan direction (default: false). `false` = mouse/trackball、`true` = natural scrolling

Notes:
- Default config location can be overridden with `-c <path>`.

## License

MIT License - see LICENSE file for details
