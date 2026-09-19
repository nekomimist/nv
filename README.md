# NV - Image Viewer

A simple image viewer built with Go and Ebiten, featuring seamless archive support and intelligent book mode for manga reading.

## Features

- Multiple Format Support: PNG, JPEG, JPEG XL, WebP, BMP, GIF
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

# Enable debug logging and also append logs to a file
./nv -d -log-file /tmp/nv-debug.log ./photos/
```

### Command-Line Options

- `-c <path>`: Load and save config using the specified JSON file
- `-d`: Enable debug logging
- `-log-file <path>`: Append logs to the given file as well as the console
- `--version`: Print version information and exit

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
- `J` - Mark current image(s) as already-joined spreads for this session
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
- Session Learning: If two wide images are actually pre-joined spreads, press `J` to teach NV not to pair similar images again during the current session
- Reading Direction: Supports both left-to-right and right-to-left modes
- Automatic Fallback: Falls back to single page when needed

## Installation

```bash
# Clone the repository
git clone https://github.com/nekomimist/nv.git
cd nv

# Build for Linux (nv)
make linux

# Cross-build for Windows x64 from Linux/WSL (nv.exe)
make windows

# Cross-build for Windows ARM64 with Zig (nv-arm64.exe)
make windows-arm64

# Windows build with a console (nv-debug.exe)
make debug

# ARM64 build with a console (nv-debug-arm64.exe)
make debug WINDOWS_ARCH=arm64

# Or run directly
CGO_ENABLED=1 go run -tags native_decode . [image_files_or_directories...]
```

All application targets use CGO-backed PNG/JPEG/WebP decoding. `make` (or
`make all`) builds Linux and Windows x64 GUI binaries. The former
`linux-native` / `windows-native` targets are now `linux` / `windows`, and
the output names are `nv` / `nv.exe` instead of `nv-native` / `nv-native.exe`.

## Requirements

- Go 1.26 or later
- Platform support: Windows, Linux (macOS untested)

Application builds require CGO and the following decode dependencies:

- Linux: `libpng-dev`, `libturbojpeg0-dev`, `libwebp-dev`, `libdeflate-dev`, and a C compiler
- Windows x64 cross-build from Linux/WSL: `gcc-mingw-w64`, `g++-mingw-w64`, `rsrc`, and `cmake`
- Windows ARM64 cross-build from Linux/WSL: Zig (`zig` on `PATH`, tested with Nix-installed Zig 0.16.0), `rsrc`, and `cmake`. Override `WINDOWS_ZIG` to use a specific Zig executable.

Windows targets automatically run `make windows-deps` for their architecture.
It fetches and cross-builds static libdeflate/libwebp libraries into
`third_party/mingw` for x64 and `third_party/zig-arm64` for ARM64, reusing
existing installations. ARM64 uses `zig cc` / `zig c++` with
`-target aarch64-windows-gnu`; `make windows WINDOWS_ARCH=arm64` is equivalent
to `make windows-arm64`. Windows icon resources are also architecture-specific.
Builds remove the legacy generated `nv.syso` to prevent duplicate or mismatched
resources when switching architectures.

ARM64 builds have been cross-compiled and their PE architecture, GUI/console
subsystem, and DLL imports checked. Runtime behavior on Windows ARM64 hardware
still needs verification.

JPEG XL is decoded by the pure-Go `github.com/gen2brain/jxl` decoder in every build. It currently decodes at full resolution because the decoder does not expose reduced-resolution output; very large JXL files can therefore use substantial transient memory before NV tiles the decoded image for display.

The Makefile enables the `native_decode` build tag and CGO for every application build. JPEG and WebP use the native decoder by default; PNG uses the native decoder only for images at least 1 megapixel, because small PNG files are often faster with Go's standard decoder. PNG additionally tries a libdeflate-backed fast path before falling back to libpng (Linux) or WIC (Windows) for the PNG shapes it doesn't cover (16-bit, palette, interlaced, or `tRNS`-bearing). WebP decodes through libwebp on both platforms (WIC is not used for WebP on Windows); on any native decode failure, a registered Go decoder serves as the fallback, like any other unsupported format. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for redistributed dependencies' licence terms.

`make test`, `make test-gui`, `make vet`, and `make lint` also enable native
decode. Untagged Go commands still select the Go decoders for testing and
benchmark comparisons; the `native_decode` build tag remains available for
direct Go commands.

Decode benchmarks:

```bash
make bench-decode
make bench-decode-native

# Full-decode the optional test_images/jpegxltest.jxl large-image fixture
make test-jxl-large

# From WSL, cross-build Windows benchmark executables and run them via Windows
make bench-decode-windows
```

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
- Use `-d` together with `-log-file <path>` when you want verbose debug logs preserved for later analysis.

## License

MIT License - see LICENSE file for details

Third-party licence notices for statically-linked dependencies in the
Windows native-decode build are in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
