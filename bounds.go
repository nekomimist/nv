package main

// bounds.go is the single source of truth for the numeric validation
// bounds shared by config loading (config.go's validateConfig) and the
// in-app settings UI (settings_ui.go's settingsAdjust).
//
// The two consumers apply different policies around these numbers:
//   - Config load treats a value below the floor as corrupt/nonsense
//     and resets it to the default; values above the ceiling are
//     clamped (not reset).
//   - The settings UI continuously clamps to [min, max] as the user
//     adjusts a value with the keyboard.
//
// Only the numbers live here; each consumer keeps its own policy.

type intBound struct {
	min, max, def int
}

type floatBound struct {
	min, max, def float64
}

var (
	// Current window size.
	windowWidthBound  = intBound{min: minWidth, max: 8192, def: defaultWidth}
	windowHeightBound = intBound{min: minHeight, max: 8192, def: defaultHeight}

	// Default (fallback) window size.
	defaultWindowWidthBound  = intBound{min: minWidth, max: 8192, def: defaultWidth}
	defaultWindowHeightBound = intBound{min: minHeight, max: 8192, def: defaultHeight}

	// Image cache size, in images.
	cacheSizeBound = intBound{min: 1, max: 64, def: 16}

	// Preload queue length, in images.
	preloadCountBound = intBound{min: 1, max: 16, def: 4}

	// Forced page-transition animation length, in frames.
	transitionFramesBound = intBound{min: 0, max: 60, def: 0}

	// Maximum image dimension before tiling; 0 means "use the
	// built-in tiling threshold" and is both the floor and the default.
	maxImageDimensionBound = intBound{min: 0, max: 16383, def: 0}

	// Aspect ratio threshold used to decide book-mode page compatibility.
	aspectRatioThresholdBound = floatBound{min: 1.0, max: 3.0, def: 1.5}

	// UI font size, in points. The floor is unified at 12.0 between
	// config load (reset-if-at-or-below) and the settings UI
	// (clamp-to-floor); the UI's previous floor of 10.0 is now 12.0.
	fontSizeBound = floatBound{min: 12.0, max: 72.0, def: 24.0}
)
