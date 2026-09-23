# Architecture Notes

## Summary

This repository is a single-package Go application built on Ebiten.
The current design centers on a still-large `Game` type in
`game_state.go`, with startup, runtime, navigation, viewport, loop,
rendering, input, image loading, sorting, and configuration split into
separate root-level files.

These notes summarize the design as observed on March 15, 2026, with
updates from the July 4, 2026 performance and refactoring pass.

## Current Layout

- `startup.go`
  - Application entrypoint
  - Flag parsing
  - Config load and launch wiring
- `game_state.go`
  - Main state container (`Game`)
  - Shared display metadata types
  - Interface implementations that expose state/actions
- `game_navigation.go`
  - Navigation/display adaptation around `navlogic`
- `game_runtime.go`
  - Settings application, fullscreen/window state, shutdown
- `game_viewport.go`
  - Zoom/pan state and viewport calculations
- `game_loop.go`
  - Ebiten `Update`, `Draw`, and `Layout`
- `renderer.go`
  - Rendering implementation
  - Image canvas and lightweight info/page-input/transient overlays
  - Draws from `RenderState`
- `ui_controller.go`
  - Retained EbitenUI tree, theme, Help and Settings panels
  - Help uses one scrollable three-column action/binding/description grid
  - Page-local image-load error cards and UI input capture
- `input.go`
  - Frame-by-frame input coordination
  - Keyboard and mouse mode switching
  - Drag/click conflict handling
- `actions.go`
  - Central action catalog
  - Default key and mouse bindings
  - Shared action execution dispatch
- `image.go`
  - Image collection from files, directories, and archives
  - Async loading, preload queue, resolution-tiered LRU cache
- `archive.go`, `archive_cache.go`
  - Shared ZIP/RAR/7z read abstraction (`archiveHandle`)
  - Bounded archive-handle cache owned by the async load worker
- `config.go`
  - JSON config load/save
  - Default filling and validation
- `bounds.go`
  - Shared numeric bounds used by config validation and settings UI
- `sort_strategy.go`
  - Sort strategy abstraction and implementations

## Startup Flow

The application startup path in `startup.go` is:

1. Parse flags.
2. Load config from the default path or `-c`.
3. Collect image paths from files, directories, or archives.
4. Create `ImageManager` with cache and preload settings.
5. Create `Game`.
6. Create keybinding and mousebinding managers.
7. Create `InputHandler`, `Renderer`, and `UIController`.
8. Apply initial display state and Ebiten window settings.
9. Run `ebiten.RunGame`.

## Main Design Boundaries

### `Game` is still the orchestration hub

`Game` owns most runtime state, including:

- current index and display mode
- zoom/pan state
- overlay state
- page input state
- settings UI state
- persisted config state
- references to renderer, input handler, and image manager

`Game` also implements multiple interfaces used internally:

- `InputActions`
- `InputState`
- `RenderState`

This keeps wiring simple, but it also leaves ownership concentrated in
one type even after the `main.go` split.

### Rendering is read-only

`Renderer` depends on `RenderState` and mostly avoids making display
decisions itself. The decision about whether to show one image or two
images is computed in `Game.calculateDisplayContent()`, then rendered from
the resulting `DisplayContent`.

This is one of the cleaner boundaries in the current codebase.

### Input is action-driven with special modes

Most input flows through the action table in `actions.go`:

- key strings are interpreted by `KeybindingManager`
- mouse strings are interpreted by `MousebindingManager`
- `InputHandler` coordinates per-frame processing
- actual behavior dispatch goes through the shared `ActionExecutor`

Three modes bypass or gate the generic action flow for practical reasons:

- help UI mode
- page number input mode
- settings screen input mode

Help and Settings block viewer input while their retained UI is active.
Page-error cards are non-modal: keyboard navigation remains available and
mouse input is captured only while the pointer is over a card.

### Image loading is the main subsystem boundary

`ImageManager` is the main functional abstraction outside `Game`.
`DefaultImageManager` currently provides:

- image path storage
- archive entry discovery
- async cache-miss loading
- preload queue management
- LRU cache eviction
- large-image tiling before Ebiten image creation when dimensions exceed the
  configured threshold
- bounded reuse of open archive handles (via `archive_cache.go`) so
  sequential reads through solid RAR/7z archives do not re-decompress
  preceding entries on every load

Actual file/byte decoding is delegated to `internal/imgdecode` so that the
decode path can be tested and benchmarked without importing Ebiten. All
Makefile application targets enable CGO and the `native_decode` tag for
PNG, JPEG and WebP. Untagged Go builds retain Go's standard and registered
image decoders for testing and benchmark comparisons.

With native decode enabled:

- Linux uses libpng, libdeflate, TurboJPEG and libwebp (all linked as shared
  libraries via `pkg-config`).
- Windows uses libdeflate and libwebp (statically linked from
  `third_party/mingw` for x64 or `third_party/zig-arm64` for ARM64,
  fetched by `make windows-deps` with the matching `WINDOWS_ARCH`; see
  `THIRD_PARTY_NOTICES.md` for their licence terms) for PNG and WebP, and
  the Windows Imaging Component (WIC) for JPEG and as the PNG fallback.
- Windows x64 uses MinGW-w64 GCC; ARM64 uses Zig's C/C++ cross-compiler
  targeting `aarch64-windows-gnu`. The decode implementation is shared.
- JPEG and WebP always try native decode first in native builds.
- PNG only tries native decode for images of at least 1 megapixel; smaller
  PNG files stay on the standard decoder to avoid native setup overhead.
- On both platforms, PNGs that are 8-bit, non-interlaced and free of `tRNS`
  take a shared libdeflate path (`internal/imgdecode/native_png_fastpath.go`)
  that inflates the whole IDAT stream at once and unfilters straight into
  premultiplied RGBA. Anything else, and any failure, falls back to the
  platform's full decoder -- libpng on Linux, WIC on Windows.
- WebP decodes through a shared libwebp path
  (`internal/imgdecode/native_webp.go`) on both platforms; Windows does not
  use WIC for WebP, so there is no dependency on an installed WIC WebP
  codec. Any native decode failure falls back to the pure-Go decoder, same
  as any other unsupported format.
- JPEG XL uses the registered pure-Go decoder in every build and never enters
  the native dispatch path.
- `internal/imgdecode/native_linux.go` and `native_windows.go` keep only
  what is genuinely platform-specific (libpng/TurboJPEG dispatch on Linux,
  WIC dispatch on Windows); `native_common.go` holds the small Go-only
  helpers (buffer allocation, `*image.RGBA` wrapping) both platforms share.

Native decoders write into a caller-supplied Go buffer and return
premultiplied `*image.RGBA`, which is the only shape Ebitengine can upload
without allocating and converting a second full-size copy.

This is the most explicit interface boundary in the repo.

Load failures are cached as metadata-only `DisplayImage` sentinels. They
retain fallback dimensions for stable navigation planning but allocate no
Ebiten texture. `UIController` presents their error message and source in a
card in the affected single/book-mode slot. The card is plain labels wrapped
to its size and truncated with an ellipsis; it deliberately avoids EbitenUI
scroll containers, which allocate screen-sized render buffers. A failed
full-tier refinement does not replace an already usable budget-tier image.

### Images are decoded at display size

`imgdecode.Hint` names the box an image will be displayed in, and
`imgdecode.Info` reports what was actually produced. JPEG (both platforms)
and WebP (both platforms) can decode smaller; PNG and JPEG XL cannot, and
report `Reduced: false`, which means "no higher-resolution version exists".
JPEG XL therefore reaches the tiling stage only after a full-resolution
decode and can have high transient memory use for very large sources.

The cache is keyed by `imgCacheKey{path, tier}` with a budget tier and a
full tier, so both can coexist and a late budget result can never overwrite
a full-resolution one. `GetImage` prefers the full tier and otherwise
returns the budget image rather than a placeholder. `EnsureResolution` is
the separate write-trigger path: when the user zooms past the decoded size
it queues a full-resolution decode on a low-priority queue, which swaps in
through the usual async refresh.

Because a texture can now be smaller than its source, zoom and layout are
measured in source pixels (`DisplayImage.SourceBounds`) while sampling and
vertices stay in texture pixels; the renderer folds each image's
texture-to-source ratio into its own draw transform. That is what keeps the
picture still when a full-resolution decode replaces a budget one.

## Behavior Notes

### Book mode

Book mode compatibility is decided from image aspect ratios.
Pairs are rejected when:

- either image is missing
- either image is extremely tall or wide
- the aspect ratios differ more than `config.AspectRatioThreshold`

When a pair is not suitable, the app falls back to single-image display.

Book mode navigation is now heuristic rather than perfectly symmetric.

- `NavigateNext` still advances by display unit from the current anchor
- `NavigatePrevious` prioritizes the nearest not-yet-shown prior page, then
  tries to pair it with the adjacent page if that pair is compatible
- this intentionally prioritizes "no skipped pages" and "no duplicate pages"
  over perfectly matching forward/backward grouping in odd-length runs
- even-length runs of compatible pages between incompatible boundaries still
  tend to produce the same grouping in both directions

`TempSingleMode` is not just an end-of-book special case anymore. It is also
used as a forced-single marker when book mode needs to show exactly one page
to preserve navigation continuity.

### Config application

Config loading is defensive:

- defaults are populated first
- invalid or missing fields are corrected during load
- missing key/mouse bindings are backfilled from defaults

The EbitenUI settings panel is generated from the shared `settingSpec`
catalog as a scrollable two-column setting/editor grid. Numeric editors
support both step buttons and direct text entry; text is committed on
Enter, focus change, or Save and clamped through the same bounds used by
keyboard adjustment. The panel edits `pendingConfig`, then saves and
reloads config to reuse the same validation path before applying runtime
changes. Existing arrow, Enter, Ctrl+S and Escape controls remain available
alongside mouse and Tab focus.

`FontSize` also drives Help, Settings and image-error text through a sized
variant of the EbitenUI theme. Settings previews its pending value before
Save; Cancel restores the configured size. Retained UI text is clamped to
16–32 points so extreme overlay font sizes cannot make the controls
unusable.

### Rendering optimization

The app skips redraws when no relevant state changed. Retained UI requests
frames for pointer, wheel, keyboard and state changes; numeric text editing
also redraws continuously for caret blinking. Idle Help, Settings and error
cards therefore do not force the underlying image to be rendered every
frame.
`RenderStateSnapshot` is used to detect changes that happen without key
input, such as overlay expiration or window resizing.

## Testing and Validation Status

Observed during repository inspection:

- `gofmt -l *.go`: clean
- `go vet ./...`: passes when `GOCACHE` is redirected to `/tmp`
- `go build -o /tmp/nv-testbuild .`: passes
- `go test ./...`: works when the root package can initialize Ebiten, but
  fresh root-package runs in this headless shell still fail during
  GLFW/X11 startup

The repo now treats tests in three practical buckets:

- strict pure tests
  - currently the `navlogic` package
  - headless-safe because they do not build the Ebiten-backed root package
- logic-oriented root-package tests
  - named `TestPure...` in the root package
  - cover config validation, path collection, sorting, and other
    behavior that avoids constructing Ebiten-backed display images, but they still
    build the root package and therefore still need an environment where
    Ebiten package startup succeeds
- GUI-dependent root-package tests
  - named `TestGUI...` in the root package
  - cover renderer caches, display content using Ebiten images, and
    image-manager behavior that depends on Ebiten image types

Supported commands:

- `make test`: full suite
- `make test-pure`: strictly headless-safe pure tests (`navlogic`)
- `make test-root-pure`: logic-oriented root-package subset
- `make test-gui`: GUI-dependent tests only

## Design Risks

### `Game` remains a broad ownership boundary

The old `main.go` hotspot is gone, but `Game` still owns a wide slice of
runtime state and still serves as the main interface hub between input,
navigation, rendering, and runtime settings.

### Runtime logic is only partially isolated from Ebiten

Some logic is cleanly isolated in `navlogic`, sorting, and config/binding
validation, but the root package still imports Ebiten broadly enough that
even logic-oriented root tests need a graphics-capable package startup
path. That keeps the strict-pure / root-pure / GUI distinction important.

### Some tests mirror logic instead of calling production paths

The higher-risk cases that used to mirror logic have already been reduced,
but test categorization is still important so the repo does not drift back
toward mixed-purpose test files.
