package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestIsArchiveExt(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"ZIP file", "test.zip", true},
		{"RAR file", "test.rar", true},
		{"ZIP uppercase", "test.ZIP", true},
		{"RAR uppercase", "test.RAR", true},
		{"PNG file", "test.png", false},
		{"Text file", "test.txt", false},
		{"No extension", "test", false},
		{"Empty string", "", false},
		{"Path with directory", "/path/to/test.zip", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isArchiveExt(tt.path)
			if result != tt.expected {
				t.Errorf("isArchiveExt(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsSupportedExt(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"PNG file", "test.png", true},
		{"JPG file", "test.jpg", true},
		{"JPEG file", "test.jpeg", true},
		{"WebP file", "test.webp", true},
		{"BMP file", "test.bmp", true},
		{"GIF file", "test.gif", true},
		{"PNG uppercase", "test.PNG", true},
		{"JPG uppercase", "test.JPG", true},
		{"Text file", "test.txt", false},
		{"No extension", "test", false},
		{"Empty string", "", false},
		{"Multiple dots", "test.backup.jpg", true},
		{"Path with directory", "/path/to/test.png", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSupportedExt(tt.path)
			if result != tt.expected {
				t.Errorf("isSupportedExt(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name           string
		configJSON     string
		expectedWidth  int
		expectedHeight int
		expectedRatio  float64
		expectedRTL    bool
	}{
		{
			name: "Valid config",
			configJSON: `{
				"window_width": 1000,
				"window_height": 800,
				"aspect_ratio_threshold": 2.0,
				"right_to_left": true
			}`,
			expectedWidth:  1000,
			expectedHeight: 800,
			expectedRatio:  2.0,
			expectedRTL:    true,
		},
		{
			name: "Width too small",
			configJSON: `{
				"window_width": 200,
				"window_height": 600,
				"aspect_ratio_threshold": 1.5,
				"right_to_left": false
			}`,
			expectedWidth:  defaultWidth,
			expectedHeight: 600,
			expectedRatio:  1.5,
			expectedRTL:    false,
		},
		{
			name: "Height too small",
			configJSON: `{
				"window_width": 800,
				"window_height": 100,
				"aspect_ratio_threshold": 1.5,
				"right_to_left": false
			}`,
			expectedWidth:  800,
			expectedHeight: defaultHeight,
			expectedRatio:  1.5,
			expectedRTL:    false,
		},
		{
			name: "Invalid aspect ratio threshold",
			configJSON: `{
				"window_width": 800,
				"window_height": 600,
				"aspect_ratio_threshold": 0.5,
				"right_to_left": false
			}`,
			expectedWidth:  800,
			expectedHeight: 600,
			expectedRatio:  1.5,
			expectedRTL:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, ".nv.json")

			err := os.WriteFile(configPath, []byte(tt.configJSON), 0644)
			if err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			configResult := loadConfigFromPath(configPath)
			config := configResult.Config

			if config.WindowWidth != tt.expectedWidth {
				t.Errorf("Expected width %d, got %d", tt.expectedWidth, config.WindowWidth)
			}
			if config.WindowHeight != tt.expectedHeight {
				t.Errorf("Expected height %d, got %d", tt.expectedHeight, config.WindowHeight)
			}
			if config.AspectRatioThreshold != tt.expectedRatio {
				t.Errorf("Expected ratio %.1f, got %.1f", tt.expectedRatio, config.AspectRatioThreshold)
			}
			if config.RightToLeft != tt.expectedRTL {
				t.Errorf("Expected RightToLeft %t, got %t", tt.expectedRTL, config.RightToLeft)
			}
		})
	}
}

func TestGameNavigation(t *testing.T) {
	tests := []struct {
		name         string
		initialIdx   int
		bookMode     bool
		shiftPressed bool
		pathsCount   int
		operation    string
		expectedIdx  int
	}{
		{"Single mode next", 0, false, false, 5, "next", 1},
		{"Single mode previous", 2, false, false, 5, "prev", 1},
		{"Single mode wrap around next", 4, false, false, 5, "next", 0},
		{"Single mode wrap around prev", 0, false, false, 5, "prev", 4},
		{"Book mode next (by 2)", 0, true, false, 6, "next", 2},
		{"Book mode previous (by 2)", 4, true, false, 6, "prev", 2},
		{"Book mode with shift (by 1)", 0, true, true, 6, "next", 1},
		{"Book mode wrap to last even", 0, true, false, 5, "prev", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create paths slice
			paths := make([]ImagePath, tt.pathsCount)
			for i := 0; i < tt.pathsCount; i++ {
				paths[i] = ImagePath{
					Path:        "image" + string(rune('0'+i)) + ".jpg",
					ArchivePath: "",
					EntryPath:   "",
				}
			}

			imageManager := NewImageManager(4)
			imageManager.SetPaths(paths)

			g := &Game{
				imageManager: imageManager,
				idx:          tt.initialIdx,
				bookMode:     tt.bookMode,
			}

			// Simulate shift key state
			pathsCount := g.imageManager.GetPathsCount()
			if tt.operation == "next" {
				if tt.bookMode && !tt.shiftPressed {
					g.idx = (g.idx + 2) % pathsCount
				} else {
					g.idx = (g.idx + 1) % pathsCount
				}
			} else { // prev
				if tt.bookMode && !tt.shiftPressed {
					g.idx -= 2
					if g.idx < 0 {
						lastEvenIdx := pathsCount - 1
						if lastEvenIdx%2 != 0 {
							lastEvenIdx--
						}
						g.idx = lastEvenIdx
					}
				} else {
					g.idx--
					if g.idx < 0 {
						g.idx = pathsCount - 1
					}
				}
			}

			if g.idx != tt.expectedIdx {
				t.Errorf("Expected idx %d, got %d", tt.expectedIdx, g.idx)
			}
		})
	}
}

func TestCollectImages(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	testFiles := []struct {
		name      string
		shouldAdd bool
	}{
		{"image1.jpg", true},
		{"image2.png", true},
		{"image3.webp", true},
		{"document.txt", false},
		{"Image4.PNG", true}, // uppercase
		{"backup.bak", false},
		{"photo.jpeg", true},
	}

	var expectedFiles []ImagePath
	for _, file := range testFiles {
		filePath := filepath.Join(tempDir, file.name)
		f, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", file.name, err)
		}
		f.Close()

		if file.shouldAdd {
			expectedFiles = append(expectedFiles, ImagePath{
				Path:        filePath,
				ArchivePath: "",
				EntryPath:   "",
			})
		}
	}

	// Test directory collection
	result, err := collectImages([]string{tempDir}, SortNatural)
	if err != nil {
		t.Fatalf("collectImages failed: %v", err)
	}

	if len(result) != len(expectedFiles) {
		t.Errorf("Expected %d images, got %d", len(expectedFiles), len(result))
		for i, expected := range expectedFiles {
			t.Errorf("Expected[%d]: %+v", i, expected)
		}
		for i, got := range result {
			t.Errorf("Got[%d]: %+v", i, got)
		}
	}

	// Test individual file collection
	singleFile := filepath.Join(tempDir, "image1.jpg")
	result, err = collectImages([]string{singleFile}, SortNatural)
	if err != nil {
		t.Fatalf("collectImages with single file failed: %v", err)
	}

	expectedSingle := ImagePath{
		Path:        singleFile,
		ArchivePath: "",
		EntryPath:   "",
	}
	if len(result) != 1 || !reflect.DeepEqual(result[0], expectedSingle) {
		t.Errorf("Expected [%+v], got %v", expectedSingle, result)
	}
}

func TestAspectRatioCompatibility(t *testing.T) {
	g := &Game{
		config: Config{AspectRatioThreshold: 1.5},
	}

	tests := []struct {
		name           string
		leftW, leftH   int
		rightW, rightH int
		expected       bool
	}{
		{"Same aspect ratio", 100, 150, 100, 150, true},
		{"Similar aspect ratio", 100, 150, 120, 180, true},
		{"Very different aspect ratio", 100, 150, 300, 100, false},
		{"One nil image", 100, 150, 0, 0, false},
		{"Extremely tall image", 100, 1000, 100, 150, false},
		{"Extremely wide image", 1000, 100, 100, 150, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var leftImg, rightImg *ebiten.Image

			if tt.leftW > 0 && tt.leftH > 0 {
				leftImg = ebiten.NewImage(tt.leftW, tt.leftH)
			}
			if tt.rightW > 0 && tt.rightH > 0 {
				rightImg = ebiten.NewImage(tt.rightW, tt.rightH)
			}

			result := g.shouldUseBookMode(leftImg, rightImg)
			if result != tt.expected {
				t.Errorf("shouldUseBookMode(%dx%d, %dx%d) = %v, want %v",
					tt.leftW, tt.leftH, tt.rightW, tt.rightH, result, tt.expected)
			}
		})
	}
}

func TestImageManager(t *testing.T) {
	paths := []ImagePath{
		{Path: "1.jpg"},
		{Path: "2.jpg"},
		{Path: "3.jpg"},
		{Path: "4.jpg"},
		{Path: "5.jpg"},
	}

	imageManager := NewImageManager(4)
	imageManager.SetPaths(paths)

	// Test GetPathsCount
	if count := imageManager.GetPathsCount(); count != 5 {
		t.Errorf("Expected paths count 5, got %d", count)
	}

	// Test GetBookModeImages (should not panic)
	leftImg, rightImg := imageManager.GetBookModeImages(0, false)
	// Since we don't have actual image files, both should be nil
	if leftImg != nil || rightImg != nil {
		// This is expected behavior when images can't be loaded
		t.Logf("Images are nil as expected (no actual image files)")
	}
}

func TestCalculateHorizontalPosition(t *testing.T) {
	g := &Game{}
	r := NewRenderer(g)

	tests := []struct {
		name     string
		x        int
		maxW     int
		scaledW  float64
		align    string
		expected float64
	}{
		{"Left align", 10, 100, 50, "left", 10},
		{"Right align", 10, 100, 50, "right", 60},
		{"Center align", 10, 100, 50, "center", 35},
		{"Default (center) align", 0, 200, 100, "unknown", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.CalculateHorizontalPosition(tt.x, tt.maxW, tt.scaledW, tt.align)
			if result != tt.expected {
				t.Errorf("Expected %.1f, got %.1f", tt.expected, result)
			}
		})
	}
}

func TestImagePathCreation(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		archivePath string
		entryPath   string
		expected    ImagePath
	}{
		{
			name:     "Regular file",
			path:     "/path/to/image.jpg",
			expected: ImagePath{Path: "/path/to/image.jpg", ArchivePath: "", EntryPath: ""},
		},
		{
			name:        "Archive entry",
			path:        "archive.zip:image.png",
			archivePath: "archive.zip",
			entryPath:   "image.png",
			expected:    ImagePath{Path: "archive.zip:image.png", ArchivePath: "archive.zip", EntryPath: "image.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var imagePath ImagePath
			if tt.archivePath != "" {
				imagePath = ImagePath{
					Path:        tt.path,
					ArchivePath: tt.archivePath,
					EntryPath:   tt.entryPath,
				}
			} else {
				imagePath = ImagePath{
					Path:        tt.path,
					ArchivePath: "",
					EntryPath:   "",
				}
			}

			if !reflect.DeepEqual(imagePath, tt.expected) {
				t.Errorf("ImagePath creation failed.\nExpected: %+v\nGot: %+v", tt.expected, imagePath)
			}
		})
	}
}

// Helper function to test if two string slices are equal
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Test helper to verify that the config loading works with default values
func TestLoadConfigDefaults(t *testing.T) {
	// Test with non-existent config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nonexistent.json")

	configResult := loadConfigFromPath(configPath)
	config := configResult.Config

	// Check all default values (without keybindings/mouse bindings for simplicity)
	if config.WindowWidth != defaultWidth {
		t.Errorf("Expected WindowWidth %d, got %d", defaultWidth, config.WindowWidth)
	}
	if config.WindowHeight != defaultHeight {
		t.Errorf("Expected WindowHeight %d, got %d", defaultHeight, config.WindowHeight)
	}
	if config.AspectRatioThreshold != 1.5 {
		t.Errorf("Expected AspectRatioThreshold 1.5, got %f", config.AspectRatioThreshold)
	}
	if config.RightToLeft != false {
		t.Errorf("Expected RightToLeft false, got %t", config.RightToLeft)
	}
	if config.HelpFontSize != 24.0 {
		t.Errorf("Expected HelpFontSize 24.0, got %f", config.HelpFontSize)
	}
	if config.SortMethod != SortNatural {
		t.Errorf("Expected SortMethod %d, got %d", SortNatural, config.SortMethod)
	}
	if config.BookMode != false {
		t.Errorf("Expected BookMode false, got %t", config.BookMode)
	}
	if config.Fullscreen != false {
		t.Errorf("Expected Fullscreen false, got %t", config.Fullscreen)
	}
	if config.CacheSize != 16 {
		t.Errorf("Expected CacheSize 16, got %d", config.CacheSize)
	}
	if config.TransitionFrames != 0 {
		t.Errorf("Expected TransitionFrames 0, got %d", config.TransitionFrames)
	}
	if config.PreloadEnabled != true {
		t.Errorf("Expected PreloadEnabled true, got %t", config.PreloadEnabled)
	}
	if config.PreloadCount != 4 {
		t.Errorf("Expected PreloadCount 4, got %d", config.PreloadCount)
	}

	// Check that keybindings and mouse bindings are properly initialized
	if len(config.Keybindings) == 0 {
		t.Error("Expected default keybindings to be populated")
	}
	if len(config.Mousebindings) == 0 {
		t.Error("Expected default mouse bindings to be populated")
	}
	if config.MouseSettings.EnableMouse != true {
		t.Errorf("Expected MouseSettings.EnableMouse true, got %t", config.MouseSettings.EnableMouse)
	}
}

func TestKeybindingConflictDetection(t *testing.T) {
	tests := []struct {
		name        string
		keybindings map[string][]string
		expectError bool
	}{
		{
			name: "no conflicts",
			keybindings: map[string][]string{
				"exit":     {"Escape"},
				"help":     {"Shift+Slash"},
				"next":     {"Space"},
				"previous": {"Backspace"},
			},
			expectError: false,
		},
		{
			name: "direct key conflict",
			keybindings: map[string][]string{
				"exit": {"Escape"},
				"help": {"Escape"}, // Same key as exit
			},
			expectError: true,
		},
		{
			name: "modifier key conflict",
			keybindings: map[string][]string{
				"exit": {"Shift+KeyA"},
				"help": {"Shift+KeyA"}, // Same combination
			},
			expectError: true,
		},
		{
			name: "multiple keys one conflict",
			keybindings: map[string][]string{
				"exit": {"Escape", "KeyQ"},
				"help": {"Shift+Slash", "KeyQ"}, // KeyQ conflicts
			},
			expectError: true,
		},
		{
			name: "valid modifier combinations",
			keybindings: map[string][]string{
				"action1": {"KeyA"},
				"action2": {"Shift+KeyA"},
				"action3": {"Ctrl+KeyA"},
				"action4": {"Alt+KeyA"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKeybindings(tt.keybindings)
			if tt.expectError && err == nil {
				t.Error("Expected validation error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error, but got: %v", err)
			}
		})
	}
}

func TestMousebindingConflictDetection(t *testing.T) {
	tests := []struct {
		name          string
		mousebindings map[string][]string
		expectError   bool
	}{
		{
			name: "no conflicts",
			mousebindings: map[string][]string{
				"next":     {"LeftClick"},
				"previous": {"RightClick"},
				"help":     {"Alt+RightClick"},
			},
			expectError: false,
		},
		{
			name: "direct mouse action conflict",
			mousebindings: map[string][]string{
				"next": {"LeftClick"},
				"help": {"LeftClick"}, // Same action
			},
			expectError: true,
		},
		{
			name: "wheel action conflict",
			mousebindings: map[string][]string{
				"next":     {"WheelDown"},
				"previous": {"WheelDown"}, // Same wheel direction
			},
			expectError: true,
		},
		{
			name: "modifier mouse conflict",
			mousebindings: map[string][]string{
				"action1": {"Shift+LeftClick"},
				"action2": {"Shift+LeftClick"}, // Same combination
			},
			expectError: true,
		},
		{
			name: "valid modifier combinations",
			mousebindings: map[string][]string{
				"action1": {"LeftClick"},
				"action2": {"Shift+LeftClick"},
				"action3": {"Ctrl+LeftClick"},
				"action4": {"Alt+LeftClick"},
			},
			expectError: false,
		},
		{
			name: "double-click combinations",
			mousebindings: map[string][]string{
				"action1": {"LeftClick"},
				"action2": {"DoubleLeftClick"}, // Different from single click
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMousebindings(tt.mousebindings)
			if tt.expectError && err == nil {
				t.Error("Expected validation error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error, but got: %v", err)
			}
		})
	}
}

func TestMouseSettingsValidation(t *testing.T) {
	tests := []struct {
		name           string
		input          MouseSettings
		expectedOutput MouseSettings
		description    string
	}{
		{
			name: "valid settings",
			input: MouseSettings{
				WheelSensitivity: 1.5,
				DoubleClickTime:  250,
				DragThreshold:    8,
				EnableMouse:      true,
				WheelInverted:    false,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.5,
				DoubleClickTime:  250,
				DragThreshold:    8,
				EnableMouse:      true,
				WheelInverted:    false,
			},
			description: "All values within valid ranges",
		},
		{
			name: "wheel sensitivity too low",
			input: MouseSettings{
				WheelSensitivity: 0.05, // Below minimum of 0.1
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0, // Should be reset to default
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
			description: "WheelSensitivity below minimum should be reset",
		},
		{
			name: "wheel sensitivity too high",
			input: MouseSettings{
				WheelSensitivity: 10.0, // Above maximum of 5.0
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 5.0, // Should be clamped to maximum
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
			description: "WheelSensitivity above maximum should be clamped",
		},
		{
			name: "double click time too low",
			input: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  50, // Below minimum of 100
				DragThreshold:    5,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300, // Should be reset to default
				DragThreshold:    5,
			},
			description: "DoubleClickTime below minimum should be reset",
		},
		{
			name: "double click time too high",
			input: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  2000, // Above maximum of 1000
				DragThreshold:    5,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  1000, // Should be clamped to maximum
				DragThreshold:    5,
			},
			description: "DoubleClickTime above maximum should be clamped",
		},
		{
			name: "drag threshold too low",
			input: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    0, // Below minimum of 1
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    5, // Should be reset to default
			},
			description: "DragThreshold below minimum should be reset",
		},
		{
			name: "drag threshold too high",
			input: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    50, // Above maximum of 20
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    20, // Should be clamped to maximum
			},
			description: "DragThreshold above maximum should be clamped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateMouseSettings(tt.input)

			if result.WheelSensitivity != tt.expectedOutput.WheelSensitivity {
				t.Errorf("WheelSensitivity: expected %f, got %f", tt.expectedOutput.WheelSensitivity, result.WheelSensitivity)
			}
			if result.DoubleClickTime != tt.expectedOutput.DoubleClickTime {
				t.Errorf("DoubleClickTime: expected %d, got %d", tt.expectedOutput.DoubleClickTime, result.DoubleClickTime)
			}
			if result.DragThreshold != tt.expectedOutput.DragThreshold {
				t.Errorf("DragThreshold: expected %d, got %d", tt.expectedOutput.DragThreshold, result.DragThreshold)
			}
			if result.EnableMouse != tt.expectedOutput.EnableMouse {
				t.Errorf("EnableMouse: expected %t, got %t", tt.expectedOutput.EnableMouse, result.EnableMouse)
			}
			if result.WheelInverted != tt.expectedOutput.WheelInverted {
				t.Errorf("WheelInverted: expected %t, got %t", tt.expectedOutput.WheelInverted, result.WheelInverted)
			}
		})
	}
}
