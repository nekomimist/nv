package main

import (
	"encoding/json"
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

type Config struct {
	WindowWidth          int     `json:"window_width"`
	WindowHeight         int     `json:"window_height"`
	AspectRatioThreshold float64 `json:"aspect_ratio_threshold"`
	RightToLeft          bool    `json:"right_to_left"`
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
		AspectRatioThreshold: 1.5,   // Default threshold for aspect ratio compatibility
		RightToLeft:          false, // Default to left-to-right reading (Western style)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return config
	}

	json.Unmarshal(data, &config)

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

	return config
}

func saveConfig(config Config) {
	// Don't save if size is too small
	if config.WindowWidth < minWidth || config.WindowHeight < minHeight {
		return
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(getConfigPath(), data, 0644)
}