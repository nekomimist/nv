package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// KeybindingManager handles dynamic keybinding processing
type KeybindingManager struct {
	keybindings map[string][]string
	keyMapping  map[string]ebiten.Key
	parsed      map[string][]KeyCombination
}

// NewKeybindingManager creates a new KeybindingManager
func NewKeybindingManager(keybindings map[string][]string) *KeybindingManager {
	km := &KeybindingManager{
		keybindings: keybindings,
		keyMapping:  keyNameToEbitenKey,
	}
	km.rebuildParsed()
	return km
}

// rebuildParsed parses the current keybindings map into KeyCombinations once,
// so CheckAction doesn't need to re-parse key strings on every frame. Invalid
// strings are skipped here exactly as CheckAction skipped them before
// pre-parsing existed.
func (km *KeybindingManager) rebuildParsed() {
	parsed := make(map[string][]KeyCombination, len(km.keybindings))
	for action, keyStrings := range km.keybindings {
		combos := make([]KeyCombination, 0, len(keyStrings))
		for _, keyStr := range keyStrings {
			if combination, valid := km.parseKeyString(keyStr); valid {
				combos = append(combos, *combination)
			}
		}
		parsed[action] = combos
	}
	km.parsed = parsed
}

// KeyCombination represents a key with optional modifiers
type KeyCombination struct {
	Key   ebiten.Key
	Shift bool
	Ctrl  bool
	Alt   bool
}

// parseKeyString parses a key string like "Shift+KeyB" into a KeyCombination.
// Unknown modifier tokens are silently ignored (not treated as a parse
// failure); only an unrecognized trailing key name fails the parse.
func (km *KeybindingManager) parseKeyString(keyStr string) (*KeyCombination, bool) {
	name, shift, ctrl, alt, _ := splitModifiers(keyStr)

	key, exists := km.keyMapping[name]
	if !exists {
		return nil, false
	}

	return &KeyCombination{Key: key, Shift: shift, Ctrl: ctrl, Alt: alt}, true
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
	combos, exists := km.parsed[action]
	if !exists {
		return false
	}

	for i := range combos {
		if km.isKeyPressed(&combos[i]) {
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
	km.rebuildParsed()
}
