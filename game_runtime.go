package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) saveCurrentConfig() {
	if g.configPath != "" {
		saveConfigToPath(g.config, g.configPath)
	} else {
		saveConfig(g.config)
	}
}

func (g *Game) saveCurrentWindowSize() {
	if g.fullscreen {
		if g.savedWinW > 0 && g.savedWinH > 0 {
			g.config.WindowWidth = g.savedWinW
			g.config.WindowHeight = g.savedWinH
		}
		return
	}

	w, h := ebiten.WindowSize()
	g.config.WindowWidth = w
	g.config.WindowHeight = h
}

// Settings UI actions
func (g *Game) ToggleSettings() {
	g.showSettings = !g.showSettings
	if g.showSettings {
		g.pendingConfig = g.config
		g.settingsIndex = 0
		return
	}

	g.showOverlayMessage("")
}

func (g *Game) SettingsMoveUp() {
	if g.settingsIndex > 0 {
		g.settingsIndex--
	}
}

func (g *Game) SettingsMoveDown() {
	if g.settingsIndex < len(settingsListOrder())-1 {
		g.settingsIndex++
	}
}

func (g *Game) SettingsLeft()  { g.settingsAdjust(true) }
func (g *Game) SettingsRight() { g.settingsAdjust(false) }
func (g *Game) SettingsEnter() { g.settingsToggleOrEnter() }

func (g *Game) SettingsCancel() {
	g.showSettings = false
	g.showOverlayMessage("Settings canceled")
}

func (g *Game) SettingsSave() {
	if g.configPath != "" {
		saveConfigToPath(g.pendingConfig, g.configPath)
		res := loadConfigFromPath(g.configPath)
		g.applyConfigResult(res)
	} else {
		saveConfig(g.pendingConfig)
		res := loadConfig()
		g.applyConfigResult(res)
	}

	g.showSettings = false
	g.showOverlayMessage("Settings saved")
}

func (g *Game) applyConfigResult(res ConfigLoadResult) {
	g.configStatus = res
	g.applyNewConfig(res.Config)
}

// applyNewConfig applies runtime-affecting changes and updates dependent systems.
func (g *Game) applyNewConfig(newCfg Config) {
	old := g.config
	g.config = newCfg

	if g.fullscreen != g.config.Fullscreen {
		g.toggleFullscreen()
	}
	if !g.fullscreen {
		ebiten.SetWindowSize(g.config.WindowWidth, g.config.WindowHeight)
		g.savedWinW = g.config.WindowWidth
		g.savedWinH = g.config.WindowHeight
	}

	g.bookMode = g.config.BookMode

	if old.SortMethod != g.config.SortMethod {
		g.reloadPathsForCurrentSource()
	}

	g.updatePreloadConfig(g.config.PreloadCount, g.config.PreloadEnabled)
	if dm, ok := g.imageManager.(*DefaultImageManager); ok {
		dm.SetMaxImageDimension(g.config.MaxImageDimension)
	}

	if g.mousebindingManager != nil {
		g.mousebindingManager.UpdateSettings(g.config.MouseSettings)
	}

	g.resetZoomToInitial()
	g.calculateDisplayContent()
}

// updatePreloadConfig updates preload manager (no effect on cache size; restart needed for cache resize).
func (g *Game) updatePreloadConfig(maxPreload int, enabled bool) {
	if dm, ok := g.imageManager.(*DefaultImageManager); ok && dm.preloadManager != nil {
		dm.preloadManager.SetMaxPreload(maxPreload)
		dm.preloadManager.SetEnabled(enabled)
	}
}

func (g *Game) ToggleFullscreen() {
	g.toggleFullscreen()
}

func (g *Game) ResetWindowSize() {
	g.resetToDefaultWindowSize()
}

func (g *Game) shutdown() {
	if g.didShutdown {
		return
	}

	g.didShutdown = true
	g.saveCurrentWindowSize()
	g.saveCurrentConfig()
	g.imageManager.StopPreload()
}

func (g *Game) toggleFullscreen() {
	g.fullscreen = !g.fullscreen
	if g.fullscreen {
		g.savedWinW, g.savedWinH = ebiten.WindowSize()
		ebiten.SetFullscreen(true)
	} else {
		ebiten.SetFullscreen(false)
		if g.savedWinW > 0 && g.savedWinH > 0 {
			ebiten.SetWindowSize(g.savedWinW, g.savedWinH)
		}
	}

	g.config.Fullscreen = g.fullscreen
	if g.config.TransitionFrames > 0 {
		g.forceRedrawFrames = g.config.TransitionFrames
	}
}

func (g *Game) resetToDefaultWindowSize() {
	currentWidth, currentHeight := ebiten.WindowSize()
	defaultWidth := g.config.DefaultWindowWidth
	defaultHeight := g.config.DefaultWindowHeight

	if !g.fullscreen && currentWidth == defaultWidth && currentHeight == defaultHeight {
		g.showOverlayMessage("Already at default window size")
		return
	}

	if g.fullscreen {
		g.fullscreen = false
		ebiten.SetFullscreen(false)
		g.config.Fullscreen = false
		g.showOverlayMessage(fmt.Sprintf("Windowed mode: %dx%d (default)", defaultWidth, defaultHeight))
	} else {
		g.showOverlayMessage(fmt.Sprintf("Window size: %dx%d (default)", defaultWidth, defaultHeight))
	}

	ebiten.SetWindowSize(defaultWidth, defaultHeight)
	g.savedWinW = defaultWidth
	g.savedWinH = defaultHeight

	if g.config.TransitionFrames > 0 {
		g.forceRedrawFrames = g.config.TransitionFrames
	}
}
