package main

import "github.com/hajimehoshi/ebiten/v2"

type mouseWheelDelta struct {
	x float64
	y float64
}

var keyNameToEbitenKey = map[string]ebiten.Key{
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

var mouseActionToButton = map[string]ebiten.MouseButton{
	"LeftClick":   ebiten.MouseButtonLeft,
	"RightClick":  ebiten.MouseButtonRight,
	"MiddleClick": ebiten.MouseButtonMiddle,
	"Back":        ebiten.MouseButton3,
	"Forward":     ebiten.MouseButton4,
}

var mouseWheelActionDeltas = map[string]mouseWheelDelta{
	"WheelUp":    {y: 1.0},
	"WheelDown":  {y: -1.0},
	"WheelLeft":  {x: -1.0},
	"WheelRight": {x: 1.0},
}

var mouseDoubleClickToButton = map[string]ebiten.MouseButton{
	"DoubleLeftClick":   ebiten.MouseButtonLeft,
	"DoubleRightClick":  ebiten.MouseButtonRight,
	"DoubleMiddleClick": ebiten.MouseButtonMiddle,
}

func isValidBindingModifier(modifier string) bool {
	switch modifier {
	case "shift", "ctrl", "alt":
		return true
	default:
		return false
	}
}
