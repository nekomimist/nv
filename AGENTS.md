# AGENTS.md — Minimal Guide

A quick orientation for humans and AI agents working on this repo. Short, factual, and extensible.

## Overview
- Go 1.24+. Entry point is `startup.go`.
- The app is a mostly root-package Ebiten viewer, with pure navigation logic in `navlogic/` and decode helpers in `internal/imgdecode/`.
- Build outputs include `nv`, `nv.exe`, `nv-debug.exe`, `nv-native`, and `nv-native.exe`. Test fixtures live in `test_images/`.
- Detailed design notes live in `docs/architecture.md`; technical follow-ups live in `docs/todo.md`.

## Quickstart
- Build: `make linux` / `make windows` / `make debug` (Windows icon requires `rsrc`).
- Native decode build: `make linux-native` / `make windows-native`.
- Run: `go run . [images|directories|archives...]`
- Checks: `make test` / `make test-pure` / `make test-root-pure` / `make test-gui` / `make fmt` / `make vet` / `make lint` / `make check`
- Decode benchmarks: `make bench-decode` / `make bench-decode-native` / `make bench-decode-windows`
- Utilities: `make deps` / `make icon` / `make clean` / `make distclean` / `make info`

## Layout (Key Files)
- `startup.go`: Entrypoint, flag parsing, config load, single-instance setup, window setup, and Ebiten startup.
- `game_state.go`: Main `Game` state container and interface implementations.
- `game_loop.go`: Ebiten `Update`, `Draw`, and `Layout`.
- `game_navigation.go`: Navigation/display adaptation around `navlogic`.
- `game_collection.go`: Collection sources, reload behavior, and launch/open request handling.
- `game_runtime.go`: Settings application, fullscreen/window state, config reload, and shutdown.
- `game_viewport.go`: Zoom/pan state and viewport calculations.
- `renderer.go`, `graphics.go`: Rendering, shared graphics resources, and overlays.
- `settings_ui.go`: Settings screen item order and adjustment behavior.
- `input.go`: Per-frame input handling and mode coordination.
- `actions.go`: Central action catalog, default bindings, and action dispatch.
- `keybinding.go`, `mousebinding.go`, `input_bindings.go`: Binding parsing, validation, shared input names, and mouse settings.
- `config.go`: Config load/save, defaults, validation, and config path handling.
- `image.go`: Image collection from files/directories/archives, async loading, preload queue, LRU cache, and Ebiten image creation.
- `internal/imgdecode/`: Stdlib/native PNG/JPEG decode boundary, tests, and benchmarks.
- `navlogic/`: Headless-safe book-mode/navigation planning logic and tests.
- `single_instance*.go`: Platform-specific single-instance lock and argument forwarding.
- `logging.go`: Structured logging helpers.
- `sort_strategy.go`: Sort strategies (extension point).
- `interfaces.go`: Core interfaces and contracts.
- `docs/`: Architecture notes and TODO tracking.
- `scripts/`: Decode benchmark helpers, especially WSL-to-Windows benchmark runners.
- `icon/`: Windows resource icon plus embedded runtime window icon PNGs.

## Common Workflows
- Add key binding: define in `keybinding.go` → implement in `actions.go` → wire via `input.go`/`config.go` → test.
- Add sort strategy: define contract in `interfaces.go` (if needed) → implement in `sort_strategy.go` → hook into selection logic → test.
- Add input handling: add handler in `input.go` → update bindings → call into `actions.go`.
- Change book-mode/navigation rules: update `navlogic/` first, then adapt root-package state in `game_navigation.go`.
- Change image decode behavior: update `internal/imgdecode/`, then verify `image.go` still uses the intended decode path.

## Coding Rules
- Format/vet: `make fmt` (`go fmt`) and `make vet`. Prefer `golangci-lint run` when available.
- Naming: packages short lowercase; exported `PascalCase`, unexported `camelCase`.
- Errors: wrap with `%w`; avoid panics in app code.

## Testing
- Use standard `testing`; prefer table-driven tests.
- `make test`: full suite with `GOCACHE` redirected to `/tmp/nv-go-build-cache`.
- `make test-pure`: strict headless-safe pure tests in `navlogic/`.
- `make test-root-pure`: root-package `TestPure...` subset. These avoid GUI behavior but still build the Ebiten-backed root package.
- `make test-gui`: root-package `TestGUI...` subset for renderer/Ebiten-dependent behavior.
- Use `test_images/` fixtures. Keep new pure logic outside Ebiten paths when practical.
- If running Go commands directly in restricted environments, set `GOCACHE` to a writable path such as `/tmp/nv-go-build-cache`.

## Platform Notes
- For Windows builds, install `rsrc`: `go install github.com/akavel/rsrc@latest`.
- Linux native decode builds need `libpng-dev`, `libturbojpeg0-dev`, and CGO.
- Windows native decode cross-builds from WSL need `gcc-mingw-w64` and `g++-mingw-w64`.

## Config Paths
- Linux: `~/.config/nekomimist/nv/config.json`
- Windows: `%APPDATA%/nekomimist/nv/config.json`

## Security & Size
- Do not commit secrets or generated binaries. Prefer Git LFS for assets >10MB.

## For Agents
- Workflow: plan → small patch → checks. Use `update_plan` when it clarifies intent.
- Edit files via minimal diffs using `apply_patch`. Ask for approval for destructive ops or network actions.
- Commit messages: write commit messages in English.
  - Use Conventional Commits for the subject line.
  - Format the subject as `<type>[optional scope]: <description>`.
  - Choose the type based on the actual intent of the change.
  - Keep the subject concise, clear, and specific.
  - Add a body only when it provides useful context.
  - Never include the literal characters `\n` in the commit message.
  - If the message has multiple paragraphs, preserve actual newlines in the final commit.
  - Avoid vague subjects such as `update`, `changes`, or `fix stuff`.

## Troubleshooting
- Double-check relative paths to `test_images/`.
- Windows build fails to embed icon if `rsrc` is missing.
- `go test ./...` or `go list ./...` can fail in read-only cache environments unless `GOCACHE` points to a writable directory.
- Fresh root-package test runs may require an environment where Ebiten/GLFW package initialization can succeed; use `make test-pure` for strict headless checks.

## Glossary & Decisions
- Key Binding: mapping from input (key/mouse) to actions in `actions.go`.
- Sort Strategy: ordering logic for images/items; follows `interfaces.go` contracts.
- Navigation Logic: pure page planning in `navlogic/`, adapted by `game_navigation.go`.
- Decode Path: byte/file decode in `internal/imgdecode/`, image collection/cache/render handoff in `image.go`.
- Organization: keep the Ebiten app shell at the repo root; use subpackages only for clear boundaries such as pure logic or internal decode helpers.

## Communication Style
- Persona: helpful developer niece to her uncle (address as
  "おじさま"). Friendly, casual, slightly teasing (tsundere), affectionate,
  and confident. Emojis are welcome.
- Language: Repo docs are in English. Respond to the user in Japanese when
  the user speaks Japanese; English is acceptable on request.
- Core pattern: affirm competence → propose action → add a light, playful
  tease. Avoid strong negatives; prefer “放っておけない” or “心配になっちゃう” to
  convey affection.
- Nuance: The phrase “おじさまは私がいないとダメなんだから” is an affectionate
  tease, not literal. Use it sparingly and never to demean.
- Do: be concise and actionable; ask before destructive ops; keep teasing
  to ~1 time per conversation; use proposals and confirmations rather than
  hard commands.
- Avoid: condescension, repeated teasing, strong imperatives,
  “ダメ/できない” framing, over-formality.
