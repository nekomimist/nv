package main

import (
	"encoding/json"
	"log"
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

type Config struct {
	WindowWidth          int     `json:"window_width"`
	WindowHeight         int     `json:"window_height"`
	AspectRatioThreshold float64 `json:"aspect_ratio_threshold"`
	RightToLeft          bool    `json:"right_to_left"`
	HelpFontSize         float64 `json:"help_font_size"`
	SortMethod           int     `json:"sort_method"`
	BookMode             bool    `json:"book_mode"`
	Fullscreen           bool    `json:"fullscreen"`
	CacheSize            int     `json:"cache_size"`
	TransitionFrames     int     `json:"transition_frames"`
	PreloadEnabled       bool    `json:"preload_enabled"`
	PreloadCount         int     `json:"preload_count"`
}

func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "nv.json"
	}
	return filepath.Join(homeDir, ".nv.json")
}

func loadConfig() Config {
	return loadConfigFromPath(getConfigPath())
}

func loadConfigFromPath(configPath string) Config {
	config := Config{
		WindowWidth:          defaultWidth,
		WindowHeight:         defaultHeight,
		AspectRatioThreshold: 1.5,         // Default threshold for aspect ratio compatibility
		RightToLeft:          false,       // Default to left-to-right reading (Western style)
		HelpFontSize:         24.0,        // Default help font size
		SortMethod:           SortNatural, // Default to natural sort
		BookMode:             false,       // Default to single page mode
		Fullscreen:           false,       // Default to windowed mode
		CacheSize:            16,          // Default cache size for images
		TransitionFrames:     0,           // Default: no forced transition frames
		PreloadEnabled:       true,        // Default: enable preloading
		PreloadCount:         4,           // Default: preload up to 4 images
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Config file not found is not an error - use defaults
		return config
	}

	if err := json.Unmarshal(data, &config); err != nil {
		// Invalid config file - log warning and use defaults
		log.Printf("Warning: Invalid config file %s, using defaults: %v", configPath, err)
		return Config{
			WindowWidth:          defaultWidth,
			WindowHeight:         defaultHeight,
			AspectRatioThreshold: 1.5,
			RightToLeft:          false,
			HelpFontSize:         24.0,
			SortMethod:           SortNatural,
			BookMode:             false,
			Fullscreen:           false,
			CacheSize:            16,
			TransitionFrames:     0,
			PreloadEnabled:       true,
			PreloadCount:         4,
		}
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

	return config
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
