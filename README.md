# NV - Image Viewer

A simple image viewer built with Go and Ebiten, featuring seamless archive support and intelligent book mode for manga reading.

## Features

- Multiple Format Support: PNG, JPEG, WebP, BMP, GIF
- Archive Integration: Direct ZIP, RAR, and 7Z file viewing
- Book Mode: Side-by-side image display with configurable reading direction
- Fullscreen Support: Toggle between windowed and fullscreen modes
- Page Jump: Direct navigation to specific pages
- Customizable Keybindings: Configure all keyboard shortcuts via JSON settings

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

### Other
- `?` - Show/hide help overlay
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
go run main.go [image_files_or_directories...]
```

## Requirements

- Go 1.19 or later
- Platform support: Windows, Linux (macOS untested)

## Configuration

Settings are automatically saved to `~/.nv.json`:

```json
{
  "window_width": 800,
  "window_height": 600,
  "aspect_ratio_threshold": 1.5,
  "right_to_left": false,
  "help_font_size": 24.0,
  "transition_frames": 0,
  "preload_enabled": true,
  "preload_count": 4,
  "keybindings": {
    "exit": ["Escape", "KeyQ"],
    "help": ["Shift+Slash"],
    "next": ["Space", "KeyN"],
    "previous": ["Backspace", "KeyP"],
    "fullscreen": ["Enter"],
    "page_input": ["KeyG"]
  }
}
```

- `aspect_ratio_threshold` - Controls book mode compatibility (default: 1.5)
- `right_to_left` - Reading direction for book mode (default: false)
- `help_font_size` - Font size for help overlay (default: 24.0)
- `transition_frames` - Force redraw frames after fullscreen transitions (default: 0)
- `preload_enabled` - Enable automatic image preloading (default: true)
- `preload_count` - Number of images to preload ahead (1-16, default: 4)
- `keybindings` - Custom keyboard shortcuts for actions. Each action can have multiple keys. Use format like `"KeyA"`, `"Space"`, `"Shift+KeyB"`. Falls back to defaults if invalid.

## License

MIT License - see LICENSE file for details
