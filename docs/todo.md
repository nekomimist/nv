# TODO

## Purpose

This file tracks technical follow-up items discovered during repository
inspection and cross-review comparison on March 12, 2026.

## Completed

- March 14, 2026: Removed the action-semantics mismatch in single-page
  navigation.
  - Result: `next_single` and `previous_single` now select single-page
    movement by action semantics instead of physical `Shift` state.

- March 14, 2026: Eliminated steady-state per-frame image composition and
  transformation allocations in the renderer.
  - Result: book-mode composition and rotation/flip redraws now reuse
    cached intermediate images instead of allocating new textures every
    frame.

- March 14, 2026: Restored bounded ownership in async image loading.
  - Result: cache misses and preload requests now flow through bounded
    manager-owned queues instead of spawning ad-hoc goroutines that can
    bypass backpressure.

- March 14, 2026: Refresh config status consistently after settings
  save/reload.
  - Result: config warnings and status shown in-app now track the most
    recent reload result.

- March 14, 2026: Replaced `os.Exit(0)` gameplay shutdown with an
  Ebiten-native termination path.
  - Result: exit requests now terminate through the game loop after normal
    shutdown work runs.

- March 14, 2026: Removed verified unused helper and compatibility code.
  - Result: unreferenced compatibility and wrapper helpers were deleted so
    the remaining code paths are less noisy ahead of larger refactors.

- March 15, 2026: Decoupled headless-testable navigation/display logic
  from Ebiten runtime image types.
  - Result: navigation/display rules now live in a pure `navlogic`
    package that can be exercised without constructing `*ebiten.Image`
    values or entering Ebiten initialization.

- March 15, 2026: Replaced logic-mirroring navigation tests with
  production-behavior tests.
  - Result: the old arithmetic-only navigation test was removed and
    replaced with table-driven tests that call the extracted production
    logic directly.

- March 15, 2026: Extracted enough of `Game`'s responsibilities to enable
  headless testing of navigation/display decisions.
  - Result: `Game` now adapts image lookup and side effects around the
    pure navigation/display core instead of owning the decision logic
    directly.

- March 15, 2026: Split startup, collection reload, and settings/runtime
  concerns out of `main.go`.
  - Result: process startup now lives in `startup.go`, collection source
    and reload behavior live in `game_collection.go`, and settings/runtime
    application live in `game_runtime.go`, establishing the file split
    that later allowed the old `main.go` shell to be removed entirely.

- March 15, 2026: Split navigation/display, frame lifecycle, and
  viewport concerns out of `main.go`.
  - Result: navigation/display adapters now live in
    `game_navigation.go`, frame update/draw/layout live in
    `game_loop.go`, and zoom/pan state and behavior live in
    `game_viewport.go`, leaving the remaining `Game` state shell ready
    for extraction into `game_state.go`.

- March 15, 2026: Removed duplicated input binding source-of-truth data.
  - Result: key names, mouse action names, wheel directions, and
    double-click/button mappings now come from shared definitions in
    `input_bindings.go`, so runtime parsing and config validation no
    longer drift independently.

- March 15, 2026: Clarified the current test strategy around GUI
  dependencies.
  - Result: root-package tests are now labeled as `TestPure...` or
    `TestGUI...`, `navlogic` remains the strict headless-safe test
    package, and the documented commands explain which subsets require a
    graphics-capable environment.

- March 15, 2026: Refreshed architecture notes for the post-`main.go`
  split layout.
  - Result: `architecture.md` now describes the extracted startup,
    runtime, navigation, loop, viewport, and `Game` state files instead
    of the old monolithic `main.go` design.

## Medium Priority

- Reassess the remaining `Game` shell and ownership boundaries.
  - Why: the `main.go` split is complete, but `Game` still acts as a
    broad state owner and interface hub, so the remaining shape should be
    either documented as intentional or reduced further.
  - Done when: the repo either documents the current `Game` ownership as
    the intended boundary or extracts additional state/forwarding only
    where it clearly improves isolation.

## Lower Priority

- Reassess whether settings UI should stay index-based.
  - Why: the current flat index and string-based dispatch are simple, but
    they require multiple lists and switch statements to stay in sync.
  - Done when: either the current approach is documented as intentional or
    a more structured model is introduced.

- Revisit global runtime helpers that obstruct isolation.
  - Why: globals like the action executor and shared graphics/font state
    make isolated testing and dependency ownership harder to reason about.
  - Done when: remaining globals are either documented as intentional or
    replaced by explicit dependencies.

- Reduce archive-format duplication where it pays off.
  - Why: ZIP, RAR, and 7z handling currently repeat the same control flow
    for entry enumeration and extraction.
  - Done when: shared archive behavior is factored behind a smaller common
    abstraction without changing supported formats.

## Reviewed But Deferred

- Revisit HiDPI scale-factor handling only after a verified repro exists.
  - Why: scale math is spread across multiple code paths, but current
    real-world behavior does not show a clear user-visible bug.
  - Revisit when: an actual HiDPI rendering, zoom, or pan defect is
    reproduced and narrowed to coordinate-space inconsistency.

- Do not track delegation boilerplate as a standalone refactor item.
  - Why: one-line interface forwarding is a symptom of `Game`/`main.go`
    overgrowth, not a separate problem worth prioritizing alone.
  - Revisit when: the larger `Game` split reaches the point where method
    collapse naturally falls out of the design.

- Defer help-overlay measurement caching until profiling justifies it.
  - Why: the help overlay does repeated measurement work, but it is a
    transient path and lower impact than the current texture-allocation
    problems.
  - Revisit when: profiling shows help overlay layout as a meaningful
    performance cost after higher-priority rendering issues are fixed.

- Do not prioritize `SortStrategy` singletonization.
  - Why: the strategies are stateless and tiny, so singletonizing them is
    not a meaningful optimization relative to current defects.
  - Revisit when: sorting becomes hot enough in profiles to justify the
    extra indirection.

- Reject the claim that `getActionDescriptions()` is dead code.
  - Why: the wrapper is thin, but it is currently used by help overlay
    layout and rendering paths.
  - Revisit when: help overlay helpers are refactored and the wrapper
    becomes truly unreferenced.

- Treat worker/channel shutdown details as part of async loading and exit.
  - Why: the important current issue is unbounded fan-out, while channel
    closure and worker lifetime should be redesigned together with bounded
    loading and graceful termination.
  - Revisit when: the async loading and shutdown model is being rewritten
    as one cohesive change.

## Notes

- Keep `architecture.md` factual.
- Keep this file action-oriented.
- Keep deferred items reasoned and explicit; they are not the same as
  accepted TODOs.
- Keep items validated and measurable; avoid speculative wish lists.
- If a TODO becomes active work, move implementation detail into an issue,
  task doc, or pull request description rather than expanding this file
  indefinitely.
