# TODO

## Purpose

This file tracks technical follow-up items discovered during repository
inspection and cross-review comparison on March 12, 2026.

## High Priority

- Decouple headless-testable logic from Ebiten initialization and runtime
  image types.
  - Why: `go test ./...` currently fails in a headless environment, and
    display decisions are tightly coupled to `*ebiten.Image`.
  - Done when: navigation/display rules can be tested without requiring
    X11 or constructing Ebiten images.

- Replace logic-mirroring tests with tests that call production behavior.
  - Why: current navigation tests reimplement behavior manually, including
    stale wrap-around expectations that no longer match the app.
  - Done when: regression tests call production navigation/display logic
    directly and assert current boundary behavior.

- Remove the action-semantics mismatch in single-page navigation.
  - Why: `next_single` and `previous_single` exist as actions, but the
    implementation still depends on physical `Shift` state.
  - Done when: single-page navigation is selected by action semantics,
    not by hard-coded modifier checks.

- Eliminate per-frame image composition and transformation allocations in
  the renderer.
  - Why: book-mode composition and rotation/flip currently build
    temporary `ebiten.Image` values during redraws, which adds avoidable
    GPU memory churn and render cost.
  - Done when: redraws reuse cached or persistent intermediate images
    instead of allocating new textures for steady-state rendering.

- Restore real backpressure and ownership in async image loading.
  - Why: cache misses currently spawn ad-hoc goroutines and can bypass the
    intended worker queue under load.
  - Done when: image decode/upload concurrency is bounded and overload
    does not fan out into uncontrolled parallel work.

- Extract enough of `Game`'s responsibilities to enable headless testing.
  - Why: the headless-testing and test-quality items above both require
    navigation and display logic to be separable from `*ebiten.Image` and
    Ebiten runtime state, which currently live together in `Game`.
  - Done when: navigation/display decision logic can be instantiated and
    called without Ebiten initialization.

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

- Refresh config status consistently after settings save/reload.
  - Why: settings reload currently reapplies config values without
    preserving the latest warning/error status for the UI.
  - Done when: config warnings shown in-app always reflect the most recent
    load/validation result.

- Replace `os.Exit(0)` with an Ebiten-native termination path.
  - Why: the current shutdown path bypasses normal return-based game
    termination and makes cleanup ownership harder to reason about.
  - Done when: exit requests are expressed through the game loop and the
    application can shut down without calling `os.Exit` from gameplay code.

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

- Remove verified unused helper and compatibility code.
  - Why: unused helpers and compatibility leftovers make maintenance
    noisier without carrying behavior.
  - Done when: confirmed unreferenced helpers are removed or documented as
    intentional extension points.

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
