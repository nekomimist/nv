# TODO

## Purpose

This file tracks technical follow-up items discovered during repository
inspection on March 8, 2026.

## High Priority

- Decouple headless-testable logic from Ebiten initialization.
  - Why: `go test ./...` currently fails in a headless environment before
    most tests can run.
  - Done when: core logic tests can run without requiring an X11 display.

- Replace logic-mirroring tests with tests that call production behavior.
  - Why: tests that reimplement navigation rules do not reliably catch
    regressions in the real code.
  - Done when: navigation tests exercise the actual `Game` navigation
    methods or a factored navigation unit.

## Medium Priority

- Reduce the responsibility concentration in `Game`.
  - Why: `Game` currently owns state and behavior across navigation,
    settings, zoom/pan, overlays, and lifecycle.
  - Done when: at least one cohesive domain is extracted behind a smaller
    interface or dedicated type.

- Split `main.go` by runtime concern rather than by growth alone.
  - Why: the current file is the main maintenance hotspot.
  - Done when: startup, navigation/display logic, and settings/runtime
    application are easier to find and test separately.

- Clarify test strategy for GUI-dependent code.
  - Why: current tests mix pure logic with Ebiten-dependent constructs.
  - Done when: the repo documents which tests are pure unit tests, which
    require a display, and how to run both kinds.

## Lower Priority

- Document architectural boundaries for future contributors.
  - Why: the code already has useful boundaries, but they are implicit.
  - Done when: contributors can see where to add behavior without reading
    every core file.

- Reassess whether settings UI should stay index-based.
  - Why: the current flat index dispatch is simple, but it may become
    brittle as the settings list grows.
  - Done when: either the current approach is documented as intentional or
    a more structured model is introduced.

## Notes

- Keep `architecture.md` factual.
- Keep this file action-oriented.
- If a TODO becomes active work, move implementation detail into an issue,
  task doc, or pull request description rather than expanding this file
  indefinitely.
