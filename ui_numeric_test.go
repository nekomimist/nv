package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGUI_SettingsNumericInputCommitPreservesPrecision(t *testing.T) {
	tests := []struct {
		name    string
		setting string
		initial float64
		first   float64
		final   float64
		set     func(*Config, float64)
		get     func(Config) float64
	}{
		{
			name:    "font size",
			setting: "FontSize",
			initial: 24.0,
			first:   24.05,
			final:   24.10,
			set:     func(c *Config, value float64) { c.FontSize = value },
			get:     func(c Config) float64 { return c.FontSize },
		},
		{
			name:    "wheel sensitivity",
			setting: "Mouse.WheelSensitivity",
			initial: 1.0,
			first:   1.05,
			final:   1.10,
			set:     func(c *Config, value float64) { c.MouseSettings.WheelSensitivity = value },
			get:     func(c Config) float64 { return c.MouseSettings.WheelSensitivity },
		},
		{
			name:    "drag sensitivity",
			setting: "Mouse.DragSensitivity",
			initial: 1.0,
			first:   1.05,
			final:   1.10,
			set:     func(c *Config, value float64) { c.MouseSettings.DragSensitivity = value },
			get:     func(c Config) float64 { return c.MouseSettings.DragSensitivity },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultConfig()
			tt.set(&config, tt.initial)
			configPath := filepath.Join(t.TempDir(), "config.json")
			game := &Game{
				config:        config,
				pendingConfig: config,
				configPath:    configPath,
				showSettings:  true,
			}
			controller := NewUIController(game)
			game.uiController = controller
			controller.Update()

			input := controller.numericInputs[tt.setting]
			if input == nil {
				t.Fatalf("numeric input %q was not created", tt.setting)
			}

			firstConfig := game.pendingConfig
			tt.set(&game.pendingConfig, tt.first)
			if editableSettingsEqual(firstConfig, game.pendingConfig) {
				t.Fatalf("editableSettingsEqual missed %s -> %s", formatSettingFloat(tt.first), formatSettingFloat(tt.initial))
			}
			controller.Update()
			if got, want := input.GetText(), formatSettingFloat(tt.first); got != want {
				t.Fatalf("input after first pending value = %q, want %q", got, want)
			}

			secondConfig := game.pendingConfig
			tt.set(&game.pendingConfig, tt.final)
			if editableSettingsEqual(secondConfig, game.pendingConfig) {
				t.Fatalf("editableSettingsEqual missed rounded display difference %s -> %s", formatSettingFloat(tt.first), formatSettingFloat(tt.final))
			}
			controller.Update()
			if got, want := input.GetText(), formatSettingFloat(tt.final); got != want {
				t.Fatalf("input after final pending value = %q, want %q", got, want)
			}

			controller.CommitSettingsInputs()
			if got := tt.get(game.pendingConfig); got != tt.final {
				t.Fatalf("pending %s after commit = %v, want %v", tt.setting, got, tt.final)
			}
			if got := tt.get(game.config); got != tt.initial {
				t.Fatalf("saved %s changed during input commit = %v, want %v", tt.setting, got, tt.initial)
			}
			if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("input commit wrote config file: stat error = %v", err)
			}
		})
	}
}
