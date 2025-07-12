package main

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// KeybindingManager handles dynamic keybinding processing
type KeybindingManager struct {
	keybindings map[string][]string
	keyMapping  map[string]ebiten.Key
}

// NewKeybindingManager creates a new KeybindingManager
func NewKeybindingManager(keybindings map[string][]string) *KeybindingManager {
	km := &KeybindingManager{
		keybindings: keybindings,
		keyMapping:  getKeyMapping(),
	}
	return km
}

// getKeyMapping returns a mapping from string keys to Ebiten keys
func getKeyMapping() map[string]ebiten.Key {
	return map[string]ebiten.Key{
		// Letters
		"KeyA": ebiten.KeyA, "KeyB": ebiten.KeyB, "KeyC": ebiten.KeyC, "KeyD": ebiten.KeyD,
		"KeyE": ebiten.KeyE, "KeyF": ebiten.KeyF, "KeyG": ebiten.KeyG, "KeyH": ebiten.KeyH,
		"KeyI": ebiten.KeyI, "KeyJ": ebiten.KeyJ, "KeyK": ebiten.KeyK, "KeyL": ebiten.KeyL,
		"KeyM": ebiten.KeyM, "KeyN": ebiten.KeyN, "KeyO": ebiten.KeyO, "KeyP": ebiten.KeyP,
		"KeyQ": ebiten.KeyQ, "KeyR": ebiten.KeyR, "KeyS": ebiten.KeyS, "KeyT": ebiten.KeyT,
		"KeyU": ebiten.KeyU, "KeyV": ebiten.KeyV, "KeyW": ebiten.KeyW, "KeyX": ebiten.KeyX,
		"KeyY": ebiten.KeyY, "KeyZ": ebiten.KeyZ,

		// Numbers
		"Key0": ebiten.Key0, "Key1": ebiten.Key1, "Key2": ebiten.Key2, "Key3": ebiten.Key3,
		"Key4": ebiten.Key4, "Key5": ebiten.Key5, "Key6": ebiten.Key6, "Key7": ebiten.Key7,
		"Key8": ebiten.Key8, "Key9": ebiten.Key9,

		// Special keys
		"Space":      ebiten.KeySpace,
		"Backspace":  ebiten.KeyBackspace,
		"Enter":      ebiten.KeyEnter,
		"Escape":     ebiten.KeyEscape,
		"Tab":        ebiten.KeyTab,
		"Home":       ebiten.KeyHome,
		"End":        ebiten.KeyEnd,
		"PageUp":     ebiten.KeyPageUp,
		"PageDown":   ebiten.KeyPageDown,
		"ArrowUp":    ebiten.KeyArrowUp,
		"ArrowDown":  ebiten.KeyArrowDown,
		"ArrowLeft":  ebiten.KeyArrowLeft,
		"ArrowRight": ebiten.KeyArrowRight,

		// Punctuation
		"Comma":     ebiten.KeyComma,
		"Period":    ebiten.KeyPeriod,
		"Slash":     ebiten.KeySlash,
		"Semicolon": ebiten.KeySemicolon,
		"Quote":     ebiten.KeyQuote,
		"Minus":     ebiten.KeyMinus,
		"Equal":     ebiten.KeyEqual,

		// Numpad
		"Numpad0":     ebiten.KeyNumpad0,
		"Numpad1":     ebiten.KeyNumpad1,
		"Numpad2":     ebiten.KeyNumpad2,
		"Numpad3":     ebiten.KeyNumpad3,
		"Numpad4":     ebiten.KeyNumpad4,
		"Numpad5":     ebiten.KeyNumpad5,
		"Numpad6":     ebiten.KeyNumpad6,
		"Numpad7":     ebiten.KeyNumpad7,
		"Numpad8":     ebiten.KeyNumpad8,
		"Numpad9":     ebiten.KeyNumpad9,
		"NumpadEnter": ebiten.KeyNumpadEnter,
	}
}

// KeyCombination represents a key with optional modifiers
type KeyCombination struct {
	Key   ebiten.Key
	Shift bool
	Ctrl  bool
	Alt   bool
}

// parseKeyString parses a key string like "Shift+KeyB" into a KeyCombination
func (km *KeybindingManager) parseKeyString(keyStr string) (*KeyCombination, bool) {
	parts := strings.Split(keyStr, "+")
	if len(parts) == 0 {
		return nil, false
	}

	combination := &KeyCombination{}

	// Last part should be the actual key
	keyName := parts[len(parts)-1]
	key, exists := km.keyMapping[keyName]
	if !exists {
		return nil, false
	}
	combination.Key = key

	// Check for modifiers
	for i := 0; i < len(parts)-1; i++ {
		switch strings.ToLower(parts[i]) {
		case "shift":
			combination.Shift = true
		case "ctrl":
			combination.Ctrl = true
		case "alt":
			combination.Alt = true
		}
	}

	return combination, true
}

// isKeyPressed checks if a key combination is currently being pressed
func (km *KeybindingManager) isKeyPressed(combination *KeyCombination) bool {
	// Check if the main key was just pressed
	if !inpututil.IsKeyJustPressed(combination.Key) {
		return false
	}

	// Check modifiers
	if combination.Shift && !ebiten.IsKeyPressed(ebiten.KeyShift) {
		return false
	}
	if combination.Ctrl && !ebiten.IsKeyPressed(ebiten.KeyControl) {
		return false
	}
	if combination.Alt && !ebiten.IsKeyPressed(ebiten.KeyAlt) {
		return false
	}

	// Check that unwanted modifiers aren't pressed
	if !combination.Shift && ebiten.IsKeyPressed(ebiten.KeyShift) {
		return false
	}
	if !combination.Ctrl && ebiten.IsKeyPressed(ebiten.KeyControl) {
		return false
	}
	if !combination.Alt && ebiten.IsKeyPressed(ebiten.KeyAlt) {
		return false
	}

	return true
}

// CheckAction checks if any keybinding for the given action is pressed
func (km *KeybindingManager) CheckAction(action string) bool {
	keyStrings, exists := km.keybindings[action]
	if !exists {
		return false
	}

	for _, keyStr := range keyStrings {
		combination, valid := km.parseKeyString(keyStr)
		if valid && km.isKeyPressed(combination) {
			return true
		}
	}

	return false
}

// ExecuteAction executes the given action using the InputActions interface
func (km *KeybindingManager) ExecuteAction(action string, inputActions InputActions, inputState InputState) bool {
	if !km.CheckAction(action) {
		return false
	}

	return globalActionExecutor.ExecuteAction(action, inputActions, inputState)
}

// GetKeybindings returns the current keybindings map (for display purposes)
func (km *KeybindingManager) GetKeybindings() map[string][]string {
	return km.keybindings
}

// UpdateKeybindings updates the keybindings map
func (km *KeybindingManager) UpdateKeybindings(keybindings map[string][]string) {
	km.keybindings = keybindings
}
