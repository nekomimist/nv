package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Window size constants
const (
	defaultWidth  = 800
	defaultHeight = 600
	minWidth      = 400
	minHeight     = 300
)

// Sort method constants
const (
	SortNatural    = 0 // Natural sort order (e.g., file1, file2, file10)
	SortSimple     = 1 // Simple string sort (lexicographical)
	SortEntryOrder = 2 // Maintain original order (no sort)
)

// validateKeybindings validates the keybindings configuration
func validateKeybindings(keybindings map[string][]string) error {
	// Check for valid key formats and detect conflicts
	keyToAction := make(map[string]string)

	for action, keys := range keybindings {
		for _, keyStr := range keys {
			// Validate key format
			if err := validateKeyString(keyStr); err != nil {
				return fmt.Errorf("invalid key '%s' for action '%s': %v", keyStr, action, err)
			}

			// Check for conflicts
			if existingAction, exists := keyToAction[keyStr]; exists {
				return fmt.Errorf("key conflict: '%s' is bound to both '%s' and '%s'", keyStr, existingAction, action)
			}
			keyToAction[keyStr] = action
		}
	}

	return nil
}

// validateMousebindings validates the mouse bindings configuration
func validateMousebindings(mousebindings map[string][]string) error {
	// Check for valid mouse action formats and detect conflicts
	mouseToAction := make(map[string]string)

	for action, mouseActions := range mousebindings {
		for _, mouseStr := range mouseActions {
			// Validate mouse action format
			if err := validateMouseString(mouseStr); err != nil {
				return fmt.Errorf("invalid mouse action '%s' for action '%s': %v", mouseStr, action, err)
			}

			// Check for conflicts
			if existingAction, exists := mouseToAction[mouseStr]; exists {
				return fmt.Errorf("mouse action conflict: '%s' is bound to both '%s' and '%s'", mouseStr, existingAction, action)
			}
			mouseToAction[mouseStr] = action
		}
	}

	return nil
}

// validateMouseString validates a single mouse string format
func validateMouseString(mouseStr string) error {
	name, _, _, _, ok := splitModifiers(mouseStr)

	// The trailing part should be a recognized mouse action
	if _, exists := mouseActionToButton[name]; !exists {
		if _, exists := mouseWheelActionDeltas[name]; !exists {
			if _, exists := mouseDoubleClickToButton[name]; !exists {
				return fmt.Errorf("unknown mouse action: %s", name)
			}
		}
	}

	if !ok {
		return fmt.Errorf("unknown modifier: %s", firstInvalidModifierToken(mouseStr))
	}

	return nil
}

// validateKeyString validates a single key string format
func validateKeyString(keyStr string) error {
	name, _, _, _, ok := splitModifiers(keyStr)

	// The trailing part should be a recognized key
	if _, exists := keyNameToEbitenKey[name]; !exists {
		return fmt.Errorf("unknown key: %s", name)
	}

	if !ok {
		return fmt.Errorf("unknown modifier: %s", firstInvalidModifierToken(keyStr))
	}

	return nil
}

// getValidMouseActionNames returns a set of valid mouse action names
func getValidMouseActionNames() map[string]bool {
	validMouseActions := make(map[string]bool, len(mouseActionToButton)+len(mouseWheelActionDeltas)+len(mouseDoubleClickToButton))
	for actionName := range mouseActionToButton {
		validMouseActions[actionName] = true
	}
	for actionName := range mouseWheelActionDeltas {
		validMouseActions[actionName] = true
	}
	for actionName := range mouseDoubleClickToButton {
		validMouseActions[actionName] = true
	}
	return validMouseActions
}

// getValidKeyNames returns a set of valid key names
func getValidKeyNames() map[string]bool {
	validKeys := make(map[string]bool, len(keyNameToEbitenKey))
	for keyName := range keyNameToEbitenKey {
		validKeys[keyName] = true
	}
	return validKeys
}

// validateMouseSettings validates the mouse settings configuration
func validateMouseSettings(settings MouseSettings) MouseSettings {
	// Validate wheel sensitivity (0.1 to 5.0)
	if settings.WheelSensitivity < 0.1 {
		settings.WheelSensitivity = 1.0
	} else if settings.WheelSensitivity > 5.0 {
		settings.WheelSensitivity = 5.0
	}

	// Validate double-click time (100 to 1000 milliseconds)
	if settings.DoubleClickTime < 100 {
		settings.DoubleClickTime = 300
	} else if settings.DoubleClickTime > 1000 {
		settings.DoubleClickTime = 1000
	}

	// Validate drag threshold (1 to 20 pixels)
	if settings.DragThreshold < 1 {
		settings.DragThreshold = 5
	} else if settings.DragThreshold > 20 {
		settings.DragThreshold = 20
	}

	return settings
}

// ConfigLoadResult contains the result of loading configuration
type ConfigLoadResult struct {
	Config   Config
	HasError bool
	Warnings []string
	Status   string // "OK", "Warning", "Error"
}

type Config struct {
	WindowWidth          int                 `json:"window_width"`
	WindowHeight         int                 `json:"window_height"`
	DefaultWindowWidth   int                 `json:"default_window_width"`
	DefaultWindowHeight  int                 `json:"default_window_height"`
	AspectRatioThreshold float64             `json:"aspect_ratio_threshold"`
	RightToLeft          bool                `json:"right_to_left"`
	FontSize             float64             `json:"font_size"`
	SortMethod           int                 `json:"sort_method"`
	BookMode             bool                `json:"book_mode"`
	Fullscreen           bool                `json:"fullscreen"`
	CacheSize            int                 `json:"cache_size"`
	MaxImageDimension    int                 `json:"max_image_dimension"`
	TransitionFrames     int                 `json:"transition_frames"`
	PreloadEnabled       bool                `json:"preload_enabled"`
	PreloadCount         int                 `json:"preload_count"`
	InitialZoomMode      string              `json:"initial_zoom_mode"`
	FitWidthAlignTop     bool                `json:"fit_width_align_top"`
	FitHeightAlignLeft   bool                `json:"fit_height_align_left"`
	Keybindings          map[string][]string `json:"keybindings"`
	Mousebindings        map[string][]string `json:"mousebindings"`
	MouseSettings        MouseSettings       `json:"mouse_settings"`
}

func getConfigPath() string {
	var configDir string

	// Try to get XDG_CONFIG_HOME on Unix-like systems, APPDATA on Windows
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		configDir = xdgConfig
	} else if appData := os.Getenv("APPDATA"); appData != "" {
		// Windows: use %APPDATA%
		configDir = appData
	} else {
		// Fallback: use ~/.config on Unix-like systems
		homeDir, err := os.UserHomeDir()
		if err != nil {
			warnKV("config", "home_dir_lookup_failed", "error", err, "fallback", "config.json")
			return "config.json" // fallback to current directory
		}
		configDir = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configDir, "nekomimist", "nv", "config.json")
}

func loadConfig() ConfigLoadResult {
	return loadConfigFromPath(getConfigPath())
}

// defaultConfig returns the built-in default configuration used both as
// the starting point for loading (before file/JSON overrides are applied)
// and as the fallback whenever a value must be reset.
func defaultConfig() Config {
	return Config{
		WindowWidth:          defaultWidth,
		WindowHeight:         defaultHeight,
		DefaultWindowWidth:   defaultWidth,  // Default window width
		DefaultWindowHeight:  defaultHeight, // Default window height
		AspectRatioThreshold: 1.5,           // Default threshold for aspect ratio compatibility
		RightToLeft:          false,         // Default to left-to-right reading (Western style)
		FontSize:             24.0,          // Default font size
		SortMethod:           SortNatural,   // Default to natural sort
		BookMode:             false,         // Default to single page mode
		Fullscreen:           false,         // Default to windowed mode
		CacheSize:            16,            // Default cache size for images
		MaxImageDimension:    0,             // Default: use the built-in tiling threshold
		TransitionFrames:     0,             // Default: no forced transition frames
		PreloadEnabled:       true,          // Default: enable preloading
		InitialZoomMode:      "fit_window",  // Default: fit to window
		FitWidthAlignTop:     false,
		FitHeightAlignLeft:   false,
		PreloadCount:         4,                         // Default: preload up to 4 images
		Keybindings:          GetDefaultKeybindings(),   // Default keybindings
		Mousebindings:        GetDefaultMousebindings(), // Default mouse bindings
		MouseSettings:        GetDefaultMouseSettings(), // Default mouse settings
	}
}

// backfillBindings ensures cfg.Keybindings and cfg.Mousebindings are
// non-nil and contain an entry for every action known to the defaults,
// filling in any actions missing from a loaded config.
func backfillBindings(cfg *Config) {
	// Keybindings - ensure defaults exist for missing actions
	if cfg.Keybindings == nil {
		cfg.Keybindings = GetDefaultKeybindings()
	} else {
		// Fill in missing keybindings with defaults
		defaults := GetDefaultKeybindings()
		for action, defaultKeys := range defaults {
			if _, exists := cfg.Keybindings[action]; !exists {
				cfg.Keybindings[action] = defaultKeys
			}
		}
	}

	// Mousebindings - ensure defaults exist for missing actions
	if cfg.Mousebindings == nil {
		cfg.Mousebindings = GetDefaultMousebindings()
	} else {
		// Fill in missing mousebindings with defaults
		mouseDefaults := GetDefaultMousebindings()
		for action, defaultMouseActions := range mouseDefaults {
			if _, exists := cfg.Mousebindings[action]; !exists {
				cfg.Mousebindings[action] = defaultMouseActions
			}
		}
	}
}

// validateConfig clamps/resets numeric and enum fields to valid ranges
// and validates keybinding/mousebinding conflicts (assuming defaults have
// already been backfilled). It mutates cfg in place and returns warning
// messages for any problems that required falling back to defaults.
func validateConfig(cfg *Config) []string {
	var warnings []string

	// Validate window size: reset below the floor (corrupt/nonsense),
	// clamp above the ceiling (excessive).
	if cfg.WindowWidth < windowWidthBound.min {
		cfg.WindowWidth = windowWidthBound.def
	} else if cfg.WindowWidth > windowWidthBound.max {
		cfg.WindowWidth = windowWidthBound.max
	}
	if cfg.WindowHeight < windowHeightBound.min {
		cfg.WindowHeight = windowHeightBound.def
	} else if cfg.WindowHeight > windowHeightBound.max {
		cfg.WindowHeight = windowHeightBound.max
	}

	// Validate default window size: same reset/clamp policy as above.
	if cfg.DefaultWindowWidth < defaultWindowWidthBound.min {
		cfg.DefaultWindowWidth = defaultWindowWidthBound.def
	} else if cfg.DefaultWindowWidth > defaultWindowWidthBound.max {
		cfg.DefaultWindowWidth = defaultWindowWidthBound.max
	}
	if cfg.DefaultWindowHeight < defaultWindowHeightBound.min {
		cfg.DefaultWindowHeight = defaultWindowHeightBound.def
	} else if cfg.DefaultWindowHeight > defaultWindowHeightBound.max {
		cfg.DefaultWindowHeight = defaultWindowHeightBound.max
	}

	// Validate aspect ratio threshold
	if cfg.AspectRatioThreshold <= aspectRatioThresholdBound.min {
		cfg.AspectRatioThreshold = aspectRatioThresholdBound.def
	}

	// Validate font size (minimum 12px for readability)
	if cfg.FontSize <= fontSizeBound.min {
		cfg.FontSize = fontSizeBound.def
	}

	// Validate sort method
	if cfg.SortMethod < SortNatural || cfg.SortMethod > SortEntryOrder {
		cfg.SortMethod = SortNatural
	}

	// Validate cache size
	if cfg.CacheSize < cacheSizeBound.min {
		cfg.CacheSize = cacheSizeBound.def
	} else if cfg.CacheSize > cacheSizeBound.max {
		cfg.CacheSize = cacheSizeBound.max
	}

	// Validate max image dimension (0 disables limit, otherwise positive
	// up to the ceiling).
	if cfg.MaxImageDimension < maxImageDimensionBound.min {
		cfg.MaxImageDimension = maxImageDimensionBound.def
	} else if cfg.MaxImageDimension > maxImageDimensionBound.max {
		cfg.MaxImageDimension = maxImageDimensionBound.max
	}

	// Validate transition frames
	if cfg.TransitionFrames < transitionFramesBound.min {
		cfg.TransitionFrames = transitionFramesBound.def
	} else if cfg.TransitionFrames > transitionFramesBound.max {
		cfg.TransitionFrames = transitionFramesBound.max
	}

	// Validate preload count
	if cfg.PreloadCount < preloadCountBound.min {
		cfg.PreloadCount = preloadCountBound.def
	} else if cfg.PreloadCount > preloadCountBound.max {
		cfg.PreloadCount = preloadCountBound.max
	}

	// Validate initial zoom mode
	validZoomModes := []string{"fit_window", "fit_width", "fit_height", "actual_size"}
	isValid := false
	for _, mode := range validZoomModes {
		if cfg.InitialZoomMode == mode {
			isValid = true
			break
		}
	}
	if !isValid {
		cfg.InitialZoomMode = "fit_window"
	}

	// Validate keybindings and resolve conflicts (defaults already backfilled)
	if err := validateKeybindings(cfg.Keybindings); err != nil {
		warnKV("config", "keybindings_invalid", "error", err, "reason", "use_defaults")
		cfg.Keybindings = GetDefaultKeybindings()
		warnings = append(warnings, fmt.Sprintf("Keybinding errors: %v", err))
	}

	// Validate mousebindings and resolve conflicts (defaults already backfilled)
	if err := validateMousebindings(cfg.Mousebindings); err != nil {
		warnKV("config", "mousebindings_invalid", "error", err, "reason", "use_defaults")
		cfg.Mousebindings = GetDefaultMousebindings()
		warnings = append(warnings, fmt.Sprintf("Mousebinding errors: %v", err))
	}

	// Validate mouse settings
	cfg.MouseSettings = validateMouseSettings(cfg.MouseSettings)

	return warnings
}

func loadConfigFromPath(configPath string) ConfigLoadResult {
	config := defaultConfig()

	result := ConfigLoadResult{
		Config:   config,
		HasError: false,
		Warnings: []string{},
		Status:   "OK",
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Config file not found is not an error - use defaults
		infoKV("config", "config_not_found", "path", configPath, "reason", "use_defaults")
		result.Status = "Default"
		return result
	}

	infoKV("config", "config_loaded", "path", configPath)

	if err := json.Unmarshal(data, &config); err != nil {
		// Invalid config file - log warning and use defaults
		warnKV("config", "config_invalid", "path", configPath, "error", err, "reason", "use_defaults")
		result.HasError = true
		result.Status = "Error"
		result.Warnings = append(result.Warnings, fmt.Sprintf("Invalid config file: %v", err))
		// Keep default config values
		return result
	}

	backfillBindings(&config)
	if warnings := validateConfig(&config); len(warnings) > 0 {
		result.Status = "Warning"
		result.Warnings = append(result.Warnings, warnings...)
	}

	// Update the result with the final config
	result.Config = config
	return result
}

// getSortMethodName returns the human-readable name of a sort method
func getSortMethodName(sortMethod int) string {
	strategy := GetSortStrategy(sortMethod)
	return strategy.Name()
}

func saveConfig(config Config) {
	saveConfigToPath(config, getConfigPath())
}

func saveConfigToPath(config Config, configPath string) {
	// Don't save if size is too small
	if config.WindowWidth < minWidth || config.WindowHeight < minHeight {
		warnKV("config", "config_save_skipped",
			"reason", "invalid_window_size",
			"width", config.WindowWidth,
			"height", config.WindowHeight,
		)
		return
	}

	// Create directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		errorKV("config", "config_dir_create_failed", "path", configDir, "error", err)
		return
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		errorKV("config", "config_marshal_failed", "error", err)
		return
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		errorKV("config", "config_save_failed", "path", configPath, "error", err)
	} else {
		infoKV("config", "config_saved", "path", configPath)
	}
}
