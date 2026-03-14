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

## Medium Priority

- Split `main.go` by runtime concern rather than by growth alone.
  - Why: the current file is the main maintenance hotspot.
  - Done when: startup, navigation/display logic, and settings/runtime
    application are easier to find and test separately.

- Remove duplicated input source-of-truth data.
  - Why: valid key names and binding resolution are maintained in separate
    places, which makes drift easy when new bindings are added.
  - Done when: input validation and runtime parsing derive from one
    shared definition.

- Clarify test strategy for GUI-dependent code.
  - Why: current tests mix pure logic with Ebiten-dependent constructs.
  - Done when: the repo documents which tests are pure unit tests, which
    require a display, and how to run both kinds.

## Lower Priority

- Document architectural boundaries for future contributors.
  - Why: the code already has useful subsystem boundaries, but ownership
    and runtime responsibilities are still mostly implicit.
  - Done when: contributors can see where to add behavior without reading
    every core file.

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
