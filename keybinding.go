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
		keyMapping:  keyNameToEbitenKey,
	}
	return km
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
		switch modifier := strings.ToLower(parts[i]); modifier {
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
