package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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

// getDefaultKeybindings returns the default keybinding configuration
func getDefaultKeybindings() map[string][]string {
	return GetDefaultKeybindings()
}

// validateKeybindings validates the keybindings configuration
func validateKeybindings(keybindings map[string][]string) error {
	// Check for valid key formats and detect conflicts
	keyToAction := make(map[string]string)
	validKeys := getValidKeyNames()

	for action, keys := range keybindings {
		for _, keyStr := range keys {
			// Validate key format
			if err := validateKeyString(keyStr, validKeys); err != nil {
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

// validateKeyString validates a single key string format
func validateKeyString(keyStr string, validKeys map[string]bool) error {
	parts := strings.Split(keyStr, "+")
	if len(parts) == 0 {
		return fmt.Errorf("empty key string")
	}

	// Last part should be the actual key
	keyName := parts[len(parts)-1]
	if !validKeys[keyName] {
		return fmt.Errorf("unknown key: %s", keyName)
	}

	// Check modifiers
	for i := 0; i < len(parts)-1; i++ {
		modifier := strings.ToLower(parts[i])
		if modifier != "shift" && modifier != "ctrl" && modifier != "alt" {
			return fmt.Errorf("unknown modifier: %s", parts[i])
		}
	}

	return nil
}

// getValidKeyNames returns a set of valid key names
func getValidKeyNames() map[string]bool {
	// Add all keys from the key mapping
	keyMapping := map[string]bool{
		// Letters
		"KeyA": true, "KeyB": true, "KeyC": true, "KeyD": true,
		"KeyE": true, "KeyF": true, "KeyG": true, "KeyH": true,
		"KeyI": true, "KeyJ": true, "KeyK": true, "KeyL": true,
		"KeyM": true, "KeyN": true, "KeyO": true, "KeyP": true,
		"KeyQ": true, "KeyR": true, "KeyS": true, "KeyT": true,
		"KeyU": true, "KeyV": true, "KeyW": true, "KeyX": true,
		"KeyY": true, "KeyZ": true,

		// Numbers
		"Key0": true, "Key1": true, "Key2": true, "Key3": true,
		"Key4": true, "Key5": true, "Key6": true, "Key7": true,
		"Key8": true, "Key9": true,

		// Special keys
		"Space": true, "Backspace": true, "Enter": true, "Escape": true,
		"Tab": true, "Home": true, "End": true, "PageUp": true, "PageDown": true,
		"ArrowUp": true, "ArrowDown": true, "ArrowLeft": true, "ArrowRight": true,

		// Punctuation
		"Comma": true, "Period": true, "Slash": true, "Semicolon": true,
		"Quote": true, "Minus": true, "Equal": true,

		// Numpad
		"Numpad0": true, "Numpad1": true, "Numpad2": true, "Numpad3": true,
		"Numpad4": true, "Numpad5": true, "Numpad6": true, "Numpad7": true,
		"Numpad8": true, "Numpad9": true, "NumpadEnter": true,
	}

	return keyMapping
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
	AspectRatioThreshold float64             `json:"aspect_ratio_threshold"`
	RightToLeft          bool                `json:"right_to_left"`
	HelpFontSize         float64             `json:"help_font_size"`
	SortMethod           int                 `json:"sort_method"`
	BookMode             bool                `json:"book_mode"`
	Fullscreen           bool                `json:"fullscreen"`
	CacheSize            int                 `json:"cache_size"`
	TransitionFrames     int                 `json:"transition_frames"`
	PreloadEnabled       bool                `json:"preload_enabled"`
	PreloadCount         int                 `json:"preload_count"`
	Keybindings          map[string][]string `json:"keybindings"`
}

func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "nv.json"
	}
	return filepath.Join(homeDir, ".nv.json")
}

func loadConfig() ConfigLoadResult {
	return loadConfigFromPath(getConfigPath())
}

func loadConfigCompat() Config {
	result := loadConfigFromPath(getConfigPath())
	return result.Config
}

func loadConfigFromPath(configPath string) ConfigLoadResult {
	config := Config{
		WindowWidth:          defaultWidth,
		WindowHeight:         defaultHeight,
		AspectRatioThreshold: 1.5,                     // Default threshold for aspect ratio compatibility
		RightToLeft:          false,                   // Default to left-to-right reading (Western style)
		HelpFontSize:         24.0,                    // Default help font size
		SortMethod:           SortNatural,             // Default to natural sort
		BookMode:             false,                   // Default to single page mode
		Fullscreen:           false,                   // Default to windowed mode
		CacheSize:            16,                      // Default cache size for images
		TransitionFrames:     0,                       // Default: no forced transition frames
		PreloadEnabled:       true,                    // Default: enable preloading
		PreloadCount:         4,                       // Default: preload up to 4 images
		Keybindings:          getDefaultKeybindings(), // Default keybindings
	}

	result := ConfigLoadResult{
		Config:   config,
		HasError: false,
		Warnings: []string{},
		Status:   "OK",
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Config file not found is not an error - use defaults
		result.Status = "Default"
		return result
	}

	if err := json.Unmarshal(data, &config); err != nil {
		// Invalid config file - log warning and use defaults
		log.Printf("Warning: Invalid config file %s, using defaults: %v", configPath, err)
		result.HasError = true
		result.Status = "Error"
		result.Warnings = append(result.Warnings, fmt.Sprintf("Invalid config file: %v", err))
		// Keep default config values
		return result
	}

	// Validate minimum size
	if config.WindowWidth < minWidth {
		config.WindowWidth = defaultWidth
	}
	if config.WindowHeight < minHeight {
		config.WindowHeight = defaultHeight
	}

	// Validate aspect ratio threshold
	if config.AspectRatioThreshold <= 1.0 {
		config.AspectRatioThreshold = 1.5
	}

	// Validate help font size (minimum 12px for readability)
	if config.HelpFontSize <= 12.0 {
		config.HelpFontSize = 24.0
	}

	// Validate sort method
	if config.SortMethod < SortNatural || config.SortMethod > SortEntryOrder {
		config.SortMethod = SortNatural
	}

	// Validate cache size (minimum 1, maximum 64)
	if config.CacheSize < 1 {
		config.CacheSize = 16
	} else if config.CacheSize > 64 {
		config.CacheSize = 64
	}

	// Validate transition frames (minimum 0, maximum 60)
	if config.TransitionFrames < 0 {
		config.TransitionFrames = 0
	} else if config.TransitionFrames > 60 {
		config.TransitionFrames = 60
	}

	// Validate preload count (minimum 1, maximum 16)
	if config.PreloadCount < 1 {
		config.PreloadCount = 4
	} else if config.PreloadCount > 16 {
		config.PreloadCount = 16
	}

	// Validate keybindings - ensure defaults exist for missing actions
	if config.Keybindings == nil {
		config.Keybindings = getDefaultKeybindings()
	} else {
		// Fill in missing keybindings with defaults
		defaults := getDefaultKeybindings()
		for action, defaultKeys := range defaults {
			if _, exists := config.Keybindings[action]; !exists {
				config.Keybindings[action] = defaultKeys
			}
		}

		// Validate keybindings and resolve conflicts
		if err := validateKeybindings(config.Keybindings); err != nil {
			log.Printf("Warning: Invalid keybindings detected, using defaults: %v", err)
			config.Keybindings = getDefaultKeybindings()
			result.Status = "Warning"
			result.Warnings = append(result.Warnings, fmt.Sprintf("Keybinding errors: %v", err))
		}
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
		log.Printf("Warning: Not saving config with invalid window size: %dx%d",
			config.WindowWidth, config.WindowHeight)
		return
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		log.Printf("Error: Failed to marshal config: %v", err)
		return
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		log.Printf("Error: Failed to save config to %s: %v", configPath, err)
	}
}
