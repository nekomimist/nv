package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPureApplyConfigResultUpdatesStatus(t *testing.T) {
	g := &Game{
		imageManager: &stubImageManager{},
		zoomState:    NewZoomState(),
		config: Config{
			WindowWidth:  800,
			WindowHeight: 600,
		},
	}
	res := ConfigLoadResult{
		Config: Config{
			WindowWidth:  1024,
			WindowHeight: 768,
		},
		Status:   "Warning",
		Warnings: []string{"normalized value"},
	}

	g.applyConfigResult(res)

	if g.configStatus.Status != "Warning" {
		t.Fatalf("config status = %q, want Warning", g.configStatus.Status)
	}
	if g.config.WindowWidth != 1024 || g.config.WindowHeight != 768 {
		t.Fatalf("config not applied: %+v", g.config)
	}
}

func TestPureIsArchiveExt(t *testing.T) {
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

func TestPureIsSupportedExt(t *testing.T) {
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

func TestPureConfigValidation(t *testing.T) {
	tests := []struct {
		name                  string
		configJSON            string
		expectedWidth         int
		expectedHeight        int
		expectedDefaultWidth  int
		expectedDefaultHeight int
		expectedRatio         float64
		expectedRTL           bool
	}{
		{
			name: "Valid config",
			configJSON: `{
				"window_width": 1000,
				"window_height": 800,
				"default_window_width": 1024,
				"default_window_height": 768,
				"aspect_ratio_threshold": 2.0,
				"right_to_left": true
			}`,
			expectedWidth:         1000,
			expectedHeight:        800,
			expectedDefaultWidth:  1024,
			expectedDefaultHeight: 768,
			expectedRatio:         2.0,
			expectedRTL:           true,
		},
		{
			name: "Width too small",
			configJSON: `{
				"window_width": 200,
				"window_height": 600,
				"aspect_ratio_threshold": 1.5,
				"right_to_left": false
			}`,
			expectedWidth:         defaultWidth,
			expectedHeight:        600,
			expectedDefaultWidth:  defaultWidth,
			expectedDefaultHeight: defaultHeight,
			expectedRatio:         1.5,
			expectedRTL:           false,
		},
		{
			name: "Height too small",
			configJSON: `{
				"window_width": 800,
				"window_height": 100,
				"aspect_ratio_threshold": 1.5,
				"right_to_left": false
			}`,
			expectedWidth:         800,
			expectedHeight:        defaultHeight,
			expectedDefaultWidth:  defaultWidth,
			expectedDefaultHeight: defaultHeight,
			expectedRatio:         1.5,
			expectedRTL:           false,
		},
		{
			name: "Invalid aspect ratio threshold",
			configJSON: `{
				"window_width": 800,
				"window_height": 600,
				"aspect_ratio_threshold": 0.5,
				"right_to_left": false
			}`,
			expectedWidth:         800,
			expectedHeight:        600,
			expectedDefaultWidth:  defaultWidth,
			expectedDefaultHeight: defaultHeight,
			expectedRatio:         1.5,
			expectedRTL:           false,
		},
		{
			name: "Default window size too small",
			configJSON: `{
				"window_width": 800,
				"window_height": 600,
				"default_window_width": 200,
				"default_window_height": 100,
				"aspect_ratio_threshold": 1.5,
				"right_to_left": false
			}`,
			expectedWidth:         800,
			expectedHeight:        600,
			expectedDefaultWidth:  defaultWidth,
			expectedDefaultHeight: defaultHeight,
			expectedRatio:         1.5,
			expectedRTL:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			if config.DefaultWindowWidth != tt.expectedDefaultWidth {
				t.Errorf("Expected default width %d, got %d", tt.expectedDefaultWidth, config.DefaultWindowWidth)
			}
			if config.DefaultWindowHeight != tt.expectedDefaultHeight {
				t.Errorf("Expected default height %d, got %d", tt.expectedDefaultHeight, config.DefaultWindowHeight)
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

func TestPureDefaultWindowSizeValidation(t *testing.T) {
	tests := []struct {
		name           string
		configJSON     string
		expectedWidth  int
		expectedHeight int
	}{
		{
			name: "Valid default window size",
			configJSON: `{
				"window_width": 800,
				"window_height": 600,
				"default_window_width": 1024,
				"default_window_height": 768
			}`,
			expectedWidth:  1024,
			expectedHeight: 768,
		},
		{
			name: "Default width too small",
			configJSON: `{
				"window_width": 800,
				"window_height": 600,
				"default_window_width": 200,
				"default_window_height": 600
			}`,
			expectedWidth:  defaultWidth,
			expectedHeight: 600,
		},
		{
			name: "Default height too small",
			configJSON: `{
				"window_width": 800,
				"window_height": 600,
				"default_window_width": 800,
				"default_window_height": 200
			}`,
			expectedWidth:  800,
			expectedHeight: defaultHeight,
		},
		{
			name: "Both default sizes too small",
			configJSON: `{
				"window_width": 800,
				"window_height": 600,
				"default_window_width": 100,
				"default_window_height": 100
			}`,
			expectedWidth:  defaultWidth,
			expectedHeight: defaultHeight,
		},
		{
			name: "Missing default window size fields",
			configJSON: `{
				"window_width": 800,
				"window_height": 600
			}`,
			expectedWidth:  defaultWidth,
			expectedHeight: defaultHeight,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, ".nv.json")

			err := os.WriteFile(configPath, []byte(tt.configJSON), 0644)
			if err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			configResult := loadConfigFromPath(configPath)
			config := configResult.Config

			if config.DefaultWindowWidth != tt.expectedWidth {
				t.Errorf("Expected default width %d, got %d", tt.expectedWidth, config.DefaultWindowWidth)
			}
			if config.DefaultWindowHeight != tt.expectedHeight {
				t.Errorf("Expected default height %d, got %d", tt.expectedHeight, config.DefaultWindowHeight)
			}
		})
	}
}

func TestPureCollectImages(t *testing.T) {
	tempDir := t.TempDir()

	testFiles := []struct {
		name      string
		shouldAdd bool
	}{
		{"image1.jpg", true},
		{"image2.png", true},
		{"image3.webp", true},
		{"document.txt", false},
		{"Image4.PNG", true},
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

func TestPureExpandToDirectorySwitchesCollectionSource(t *testing.T) {
	tempDir := t.TempDir()
	originalFile := filepath.Join(tempDir, "page2.jpg")
	otherFile := filepath.Join(tempDir, "page10.jpg")

	for _, path := range []string{originalFile, otherFile} {
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}

	initialPaths := []ImagePath{{Path: originalFile}}
	imageManager := &stubImageManager{}
	imageManager.SetPaths(initialPaths)

	g := &Game{
		imageManager:     imageManager,
		config:           Config{SortMethod: SortNatural},
		collectionSource: newArgsCollectionSource([]string{originalFile}),
		launchSingleFile: originalFile,
		zoomState:        NewZoomState(),
		bookMode:         false,
		fullscreen:       true,
		currentLogicalW:  800,
		currentLogicalH:  600,
	}

	g.expandToDirectoryAndJump()

	if g.collectionSource.Mode != CollectionSourceExpandedSingleDirectory {
		t.Fatalf("expected expanded directory source, got %v", g.collectionSource.Mode)
	}

	if got := g.imageManager.GetPathsCount(); got != 2 {
		t.Fatalf("expected 2 paths after expansion, got %d", got)
	}

	currentPath, ok := g.imageManager.GetPath(g.idx)
	if !ok {
		t.Fatal("expected current path after expansion")
	}
	if currentPath.Path != originalFile {
		t.Fatalf("expected current file %s after expansion, got %s", originalFile, currentPath.Path)
	}
}

func TestPureReloadPathsForCurrentSourceKeepsExpandedDirectorySelection(t *testing.T) {
	tempDir := t.TempDir()
	originalFile := filepath.Join(tempDir, "page2.jpg")
	files := []string{
		filepath.Join(tempDir, "page01.jpg"),
		originalFile,
		filepath.Join(tempDir, "page10.jpg"),
	}

	for _, path := range files {
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}

	initialPaths, err := collectImagesFromSameDirectory(originalFile, SortNatural)
	if err != nil {
		t.Fatalf("collectImagesFromSameDirectory failed: %v", err)
	}

	imageManager := &stubImageManager{}
	imageManager.SetPaths(initialPaths)

	originalIdx := findImagePathIndex(initialPaths, originalFile)
	if originalIdx < 0 {
		t.Fatalf("expected to find original file in initial paths")
	}

	g := &Game{
		imageManager:     imageManager,
		idx:              originalIdx,
		config:           Config{SortMethod: SortSimple},
		collectionSource: newExpandedDirectorySource(originalFile),
		launchSingleFile: originalFile,
		zoomState:        NewZoomState(),
		bookMode:         false,
		fullscreen:       true,
		currentLogicalW:  800,
		currentLogicalH:  600,
	}

	if ok := g.reloadPathsForCurrentSource(); !ok {
		t.Fatal("expected reloadPathsForCurrentSource to succeed")
	}

	if got := g.imageManager.GetPathsCount(); got != 3 {
		t.Fatalf("expected 3 paths after reload, got %d", got)
	}

	currentPath, ok := g.imageManager.GetPath(g.idx)
	if !ok {
		t.Fatal("expected current path after reload")
	}
	if currentPath.Path != originalFile {
		t.Fatalf("expected current file %s after sort reload, got %s", originalFile, currentPath.Path)
	}
}

func TestPureApplyNewConfigReloadsFromCurrentSource(t *testing.T) {
	tempDir := t.TempDir()
	originalFile := filepath.Join(tempDir, "page2.jpg")
	files := []string{
		filepath.Join(tempDir, "page01.jpg"),
		originalFile,
		filepath.Join(tempDir, "page10.jpg"),
	}

	for _, path := range files {
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}

	initialPaths, err := collectImagesFromSameDirectory(originalFile, SortNatural)
	if err != nil {
		t.Fatalf("collectImagesFromSameDirectory failed: %v", err)
	}

	imageManager := &stubImageManager{}
	imageManager.SetPaths(initialPaths)

	originalIdx := findImagePathIndex(initialPaths, originalFile)
	if originalIdx < 0 {
		t.Fatalf("expected to find original file in initial paths")
	}

	g := &Game{
		imageManager: imageManager,
		idx:          originalIdx,
		bookMode:     false,
		fullscreen:   true,
		config: Config{
			SortMethod: SortNatural,
			Fullscreen: true,
		},
		collectionSource: newExpandedDirectorySource(originalFile),
		launchSingleFile: originalFile,
		zoomState:        NewZoomState(),
		currentLogicalW:  800,
		currentLogicalH:  600,
	}

	newCfg := g.config
	newCfg.SortMethod = SortSimple

	g.applyNewConfig(newCfg)

	if g.collectionSource.Mode != CollectionSourceExpandedSingleDirectory {
		t.Fatalf("expected expanded directory source after config apply, got %v", g.collectionSource.Mode)
	}

	if got := g.imageManager.GetPathsCount(); got != 3 {
		t.Fatalf("expected 3 paths after config apply reload, got %d", got)
	}

	currentPath, ok := g.imageManager.GetPath(g.idx)
	if !ok {
		t.Fatal("expected current path after config apply")
	}
	if currentPath.Path != originalFile {
		t.Fatalf("expected current file %s after config apply, got %s", originalFile, currentPath.Path)
	}
}

func TestPureCalculateHorizontalPosition(t *testing.T) {
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

func TestPureImagePathCreation(t *testing.T) {
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

func TestPureLoadConfigDefaults(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nonexistent.json")

	configResult := loadConfigFromPath(configPath)
	config := configResult.Config

	if config.WindowWidth != defaultWidth {
		t.Errorf("Expected WindowWidth %d, got %d", defaultWidth, config.WindowWidth)
	}
	if config.WindowHeight != defaultHeight {
		t.Errorf("Expected WindowHeight %d, got %d", defaultHeight, config.WindowHeight)
	}
	if config.DefaultWindowWidth != defaultWidth {
		t.Errorf("Expected DefaultWindowWidth %d, got %d", defaultWidth, config.DefaultWindowWidth)
	}
	if config.DefaultWindowHeight != defaultHeight {
		t.Errorf("Expected DefaultWindowHeight %d, got %d", defaultHeight, config.DefaultWindowHeight)
	}
	if config.AspectRatioThreshold != 1.5 {
		t.Errorf("Expected AspectRatioThreshold 1.5, got %f", config.AspectRatioThreshold)
	}
	if config.RightToLeft != false {
		t.Errorf("Expected RightToLeft false, got %t", config.RightToLeft)
	}
	if config.FontSize != 24.0 {
		t.Errorf("Expected FontSize 24.0, got %f", config.FontSize)
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

func TestPureKeybindingConflictDetection(t *testing.T) {
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
				"help": {"Escape"},
			},
			expectError: true,
		},
		{
			name: "modifier key conflict",
			keybindings: map[string][]string{
				"exit": {"Shift+KeyA"},
				"help": {"Shift+KeyA"},
			},
			expectError: true,
		},
		{
			name: "multiple keys one conflict",
			keybindings: map[string][]string{
				"exit": {"Escape", "KeyQ"},
				"help": {"Shift+Slash", "KeyQ"},
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

func TestPureMousebindingConflictDetection(t *testing.T) {
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
				"help": {"LeftClick"},
			},
			expectError: true,
		},
		{
			name: "wheel action conflict",
			mousebindings: map[string][]string{
				"next":     {"WheelDown"},
				"previous": {"WheelDown"},
			},
			expectError: true,
		},
		{
			name: "modifier mouse conflict",
			mousebindings: map[string][]string{
				"action1": {"Shift+LeftClick"},
				"action2": {"Shift+LeftClick"},
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
				"action2": {"DoubleLeftClick"},
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

func TestPureMouseSettingsValidation(t *testing.T) {
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
		},
		{
			name: "wheel sensitivity too low",
			input: MouseSettings{
				WheelSensitivity: 0.05,
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
		},
		{
			name: "wheel sensitivity too high",
			input: MouseSettings{
				WheelSensitivity: 10.0,
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 5.0,
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
		},
		{
			name: "double click time too low",
			input: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  50,
				DragThreshold:    5,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
		},
		{
			name: "double click time too high",
			input: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  2000,
				DragThreshold:    5,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  1000,
				DragThreshold:    5,
			},
		},
		{
			name: "drag threshold too low",
			input: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    0,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    5,
			},
		},
		{
			name: "drag threshold too high",
			input: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    50,
			},
			expectedOutput: MouseSettings{
				WheelSensitivity: 1.0,
				DoubleClickTime:  300,
				DragThreshold:    20,
			},
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
