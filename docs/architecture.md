# Architecture Notes

## Summary

This repository is a single-package Go application built on Ebiten.
The current design centers on a still-large `Game` type in
`game_state.go`, with startup, runtime, navigation, viewport, loop,
rendering, input, image loading, sorting, and configuration split into
separate root-level files.

These notes summarize the design as observed on March 15, 2026.

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
  - Help/info/settings overlays
  - Draws from `RenderState`
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
  - Async loading, preload queue, LRU cache
  - Archive support for ZIP, RAR, and 7z
- `config.go`
  - JSON config load/save
  - Default filling and validation
- `sort_strategy.go`
  - Sort strategy abstraction and implementations

## Startup Flow

The application startup path in `startup.go` is:

1. Parse flags.
2. Load config from the default path or `-c`.
3. Initialize graphics resources for error placeholders.
4. Collect image paths from files, directories, or archives.
5. Create `ImageManager` with cache and preload settings.
6. Create `Game`.
7. Create keybinding and mousebinding managers.
8. Create `InputHandler` and `Renderer`.
9. Apply initial display state and Ebiten window settings.
10. Run `ebiten.RunGame`.

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

Two modes bypass the generic action flow for practical reasons:

- page number input mode
- settings screen input mode

### Image loading is the main subsystem boundary

`ImageManager` is the main functional abstraction outside `Game`.
`DefaultImageManager` currently provides:

- image path storage
- archive entry discovery
- async cache-miss loading
- preload queue management
- LRU cache eviction
- large-image downscaling before Ebiten image creation

This is the most explicit interface boundary in the repo.

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

The settings UI edits `pendingConfig`, then saves and reloads config to
reuse the same validation path before applying runtime changes.

### Rendering optimization

The app skips redraws when no relevant state changed.
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
    behavior that avoids constructing `*ebiten.Image`, but they still
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
