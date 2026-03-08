# Architecture Notes

## Summary

This repository is a single-package Go application built on Ebiten.
The current design centers on a large `Game` type in `main.go`, with
supporting components for rendering, input, image loading, sorting, and
configuration.

These notes summarize the design as observed on March 8, 2026.

## Current Layout

- `main.go`
  - Application entrypoint
  - Ebiten game loop (`Update`, `Draw`, `Layout`)
  - Main state container (`Game`)
  - Navigation, zoom/pan, overlays, settings application
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

The application startup path in `main.go` is:

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

### `Game` is the orchestration hub

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

This keeps wiring simple, but it also concentrates behavior in one type.

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

`go test ./...` does not run cleanly in the current headless environment.
The failure is caused by Ebiten/GLFW trying to initialize X11 and failing
to open display `:0`, which prevents package test startup.

## Design Risks

### Large `main.go`

`main.go` is currently the largest file and holds several concerns at once:

- app lifecycle
- runtime state
- navigation rules
- zoom/pan rules
- settings application
- display content calculation

This makes refactors and regression-focused testing harder.

### Runtime logic is only partially isolated from Ebiten

Some logic is testable, but package-level tests still hit Ebiten startup
paths in this environment. That limits reliable automated testing in CI or
headless shells.

### Some tests mirror logic instead of calling production paths

At least some navigation tests reconstruct expected behavior manually
instead of exercising the real navigation methods. That lowers their value
as regression tests.
