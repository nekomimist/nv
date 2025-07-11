package main

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// ActionDefinition defines an action with its default keybindings and description
type ActionDefinition struct {
	Name        string
	Keys        []string
	Description string
}

// actionDefinitions contains all action definitions with default keybindings and descriptions
var actionDefinitions = []ActionDefinition{
	{"exit", []string{"Escape", "KeyQ"}, "Quit application"},
	{"help", []string{"Shift+Slash"}, "Show/hide help"},
	{"info", []string{"KeyI"}, "Show/hide info display"},
	{"next", []string{"Space", "KeyN"}, "Next image (or 2 images in book mode)"},
	{"previous", []string{"Backspace", "KeyP"}, "Previous image (or 2 images in book mode)"},
	{"next_single", []string{"Shift+Space", "Shift+KeyN"}, "Single page forward (fine adjustment)"},
	{"previous_single", []string{"Shift+Backspace", "Shift+KeyP"}, "Single page backward (fine adjustment)"},
	{"toggle_book_mode", []string{"KeyB"}, "Toggle book mode (dual image view)"},
	{"toggle_reading_direction", []string{"Shift+KeyB"}, "Toggle reading direction (LTR ↔ RTL)"},
	{"fullscreen", []string{"Enter"}, "Toggle fullscreen"},
	{"page_input", []string{"KeyG"}, "Go to page (enter page number)"},
	{"jump_first", []string{"Home", "Shift+Comma"}, "Jump to first page"},
	{"jump_last", []string{"End", "Shift+Period"}, "Jump to last page"},
	{"rotate_left", []string{"KeyL"}, "Rotate left 90 degrees"},
	{"rotate_right", []string{"KeyR"}, "Rotate right 90 degrees"},
	{"flip_horizontal", []string{"KeyH"}, "Flip horizontally"},
	{"flip_vertical", []string{"KeyV"}, "Flip vertically"},
	{"cycle_sort", []string{"Shift+KeyS"}, "Cycle sort method (Natural/Simple/Entry)"},
	{"expand_directory", []string{"KeyS"}, "Scan directory images (single file mode)"},
}

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

	switch action {
	case "exit":
		inputActions.Exit()
	case "help":
		inputActions.ToggleHelp()
	case "info":
		inputActions.ToggleInfo()
	case "next":
		inputActions.NavigateNext()
	case "previous":
		inputActions.NavigatePrevious()
	case "next_single":
		// Single page navigation (overrides book mode temporarily)
		inputActions.NavigateNext()
	case "previous_single":
		// Single page navigation (overrides book mode temporarily)
		inputActions.NavigatePrevious()
	case "toggle_book_mode":
		inputActions.ToggleBookMode()
	case "toggle_reading_direction":
		inputActions.ToggleReadingDirection()
	case "fullscreen":
		inputActions.ToggleFullscreen()
	case "page_input":
		if !inputState.IsInPageInputMode() {
			inputActions.EnterPageInputMode()
		}
	case "jump_first":
		inputActions.JumpToPage(1)
	case "jump_last":
		totalPages := inputActions.GetTotalPagesCount()
		if totalPages > 0 {
			inputActions.JumpToPage(totalPages)
		}
	case "rotate_left":
		inputActions.RotateLeft()
	case "rotate_right":
		inputActions.RotateRight()
	case "flip_horizontal":
		inputActions.FlipHorizontal()
	case "flip_vertical":
		inputActions.FlipVertical()
	case "cycle_sort":
		inputActions.CycleSortMethod()
	case "expand_directory":
		inputActions.ExpandToDirectory()
	default:
		return false
	}

	return true
}

// GetKeybindings returns the current keybindings map (for display purposes)
func (km *KeybindingManager) GetKeybindings() map[string][]string {
	return km.keybindings
}

// UpdateKeybindings updates the keybindings map
func (km *KeybindingManager) UpdateKeybindings(keybindings map[string][]string) {
	km.keybindings = keybindings
}

// GetActionDescriptions returns a map of action names to their descriptions
func GetActionDescriptions() map[string]string {
	descriptions := make(map[string]string)
	for _, action := range actionDefinitions {
		descriptions[action.Name] = action.Description
	}
	return descriptions
}

// GetDefaultKeybindings returns a map of action names to their default keybindings
func GetDefaultKeybindings() map[string][]string {
	keybindings := make(map[string][]string)
	for _, action := range actionDefinitions {
		keybindings[action.Name] = action.Keys
	}
	return keybindings
}
