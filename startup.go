package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"nv/navlogic"
)

// Build-time variables (set by ldflags)
var (
	version   = "dev"
	buildDate = "unknown"
)

// Global debug mode flag
var debugMode bool

//go:embed icon/icon_16.png
var icon16 []byte

//go:embed icon/icon_32.png
var icon32 []byte

//go:embed icon/icon_48.png
var icon48 []byte

type startupOptions struct {
	configPath string
	logPath    string
	args       []string
}

func parseStartupOptions() startupOptions {
	configFile := flag.String("c", "", "config file path (default: OS config dir)")
	debug := flag.Bool("d", false, "enable debug logging")
	logFile := flag.String("log-file", "", "append logs to file as well as console")
	showVersion := flag.Bool("version", false, "show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("nv version v%s (built on %s)\n", version, buildDate)
		os.Exit(0)
	}

	debugMode = *debug
	return startupOptions{
		configPath: *configFile,
		logPath:    *logFile,
		args:       flag.Args(),
	}
}

func loadStartupConfig(configPath string) ConfigLoadResult {
	if configPath != "" {
		return loadConfigFromPath(configPath)
	}
	return loadConfig()
}

func newGameFromStartup(configResult ConfigLoadResult, configPath string, args []string, paths []ImagePath) *Game {
	config := configResult.Config
	debugKV("startup", "game_create_begin",
		"args_count", len(args),
		"paths_count", len(paths),
		"book_mode", config.BookMode,
		"fullscreen", config.Fullscreen,
		"cache_size", config.CacheSize,
		"preload_enabled", config.PreloadEnabled,
		"preload_count", config.PreloadCount,
	)

	imageManager := NewImageManagerWithPreload(config.CacheSize, config.PreloadCount, config.PreloadEnabled)
	if dm, ok := imageManager.(*DefaultImageManager); ok {
		dm.SetMaxImageDimension(config.MaxImageDimension)
	}
	imageManager.SetPaths(paths)

	g := &Game{
		imageManager:     imageManager,
		idx:              0,
		bookMode:         config.BookMode,
		fullscreen:       config.Fullscreen,
		config:           config,
		configPath:       configPath,
		showInfo:         false,
		collectionSource: newArgsCollectionSource(args),
		configStatus:     configResult,
		zoomState:        NewZoomState(),
	}

	g.resetZoomToInitial()
	imageManager.StartPreload(0, NavigationForward)

	keybindingManager := NewKeybindingManager(config.Keybindings)
	g.keybindingManager = keybindingManager

	mousebindingManager := NewMousebindingManager(config.Mousebindings, config.MouseSettings)
	g.mousebindingManager = mousebindingManager
	g.inputHandler = NewInputHandler(g, g, keybindingManager, mousebindingManager)
	g.renderer = NewRenderer(g)

	applyStartupConfigWarning(g, configResult)
	initializeSingleFileMode(g, args)
	initializeBookModeForLaunch(g, paths)
	g.calculateDisplayContent()
	return g
}

func applyStartupConfigWarning(g *Game, configResult ConfigLoadResult) {
	if configResult.Status != "Warning" && configResult.Status != "Error" {
		return
	}

	if len(configResult.Warnings) > 0 {
		g.showOverlayMessage(fmt.Sprintf("Config %s: %s", configResult.Status, configResult.Warnings[0]))
		return
	}

	g.showOverlayMessage(fmt.Sprintf("Config %s: Using defaults", configResult.Status))
}

func initializeSingleFileMode(g *Game, args []string) {
	if len(args) == 1 && isSupportedExt(args[0]) && !isArchiveExt(args[0]) {
		g.launchSingleFile = args[0]
		debugKV("startup", "single_file_mode_enabled", "path", args[0])
	}
}

func initializeBookModeForLaunch(g *Game, paths []ImagePath) {
	if !g.config.BookMode || len(paths) == 0 {
		return
	}

	if len(paths) == 1 {
		g.tempSingleMode = true
		debugKV("startup", "book_mode_launch_single_fallback",
			"paths_count", len(paths),
			"reason", "single_path",
		)
		return
	}

	plan := navlogic.PlanDisplay(g.navigationState(), g.pageMetricsAt)
	if plan.ActualImages != 2 {
		g.tempSingleMode = true
	}
	debugKV("startup", "book_mode_launch_plan",
		"paths_count", len(paths),
		"actual_images", plan.ActualImages,
		"temp_single", g.tempSingleMode,
	)
}

func configureWindow(g *Game) {
	ebiten.SetWindowTitle(getWindowTitle())
	ebiten.SetWindowSize(g.config.WindowWidth, g.config.WindowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetScreenClearedEveryFrame(false)
	setWindowIcon()

	if g.config.Fullscreen {
		g.savedWinW, g.savedWinH = g.config.WindowWidth, g.config.WindowHeight
		ebiten.SetFullscreen(true)
	}

	debugKV("startup", "window_configured",
		"width", g.config.WindowWidth,
		"height", g.config.WindowHeight,
		"fullscreen", g.config.Fullscreen,
	)
}

// getWindowTitle returns the window title with version information.
func getWindowTitle() string {
	if version == "dev" {
		return "Nekomimist's Image Viewer (dev)"
	}
	return fmt.Sprintf("Nekomimist's Image Viewer v%s", version)
}

func main() {
	opts := parseStartupOptions()
	logFile, err := configureLogOutput(opts.logPath)
	if err != nil {
		fatalKV("startup", "log_output_configure_failed", "path", opts.logPath, "error", err)
	}
	if logFile != nil {
		defer logFile.Close()
		infoKV("startup", "log_file_enabled", "path", opts.logPath)
	}

	configResult := loadStartupConfig(opts.configPath)
	debugKV("startup", "options_parsed",
		"config_path", opts.configPath,
		"log_path", opts.logPath,
		"args", opts.args,
		"debug", debugMode,
	)

	if err := InitGraphics(); err != nil {
		warnKV("startup", "graphics_init_failed", "error", err)
	}

	paths, err := collectImages(opts.args, configResult.Config.SortMethod)
	if err != nil {
		fatalKV("startup", "collect_images_failed", "error", err)
	}
	if len(paths) == 0 {
		fatalKV("startup", "no_images", "args_count", len(opts.args))
	}
	infoKV("startup", "images_collected", "paths_count", len(paths), "sort_method", configResult.Config.SortMethod)

	g := newGameFromStartup(configResult, opts.configPath, opts.args, paths)
	configureWindow(g)

	if err := ebiten.RunGame(g); err != nil && err != ebiten.Termination {
		fatalKV("startup", "run_game_failed", "error", err)
	}
}

func setWindowIcon() {
	iconData := [][]byte{icon16, icon32, icon48}
	var iconImages []image.Image

	for _, data := range iconData {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			continue
		}
		iconImages = append(iconImages, img)
	}

	if len(iconImages) > 0 {
		ebiten.SetWindowIcon(iconImages)
	}
}
