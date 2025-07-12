package main

// ActionDefinition defines an action with its default keybindings, mouse bindings, and description
type ActionDefinition struct {
	Name         string
	Keys         []string
	MouseActions []string
	Description  string
}

// actionDefinitions contains all action definitions with default keybindings, mouse bindings, and descriptions
var actionDefinitions = []ActionDefinition{
	{"exit", []string{"Escape", "KeyQ"}, []string{}, "Quit application"},
	{"help", []string{"Shift+Slash"}, []string{"Alt+RightClick"}, "Show/hide help"},
	{"info", []string{"KeyI"}, []string{}, "Show/hide info display"},
	{"next", []string{"Space", "KeyN"}, []string{"LeftClick", "WheelDown"}, "Next image (or 2 images in book mode)"},
	{"previous", []string{"Backspace", "KeyP"}, []string{"RightClick", "WheelUp"}, "Previous image (or 2 images in book mode)"},
	{"next_single", []string{"Shift+Space", "Shift+KeyN"}, []string{"Shift+LeftClick", "Shift+WheelDown"}, "Single page forward (fine adjustment)"},
	{"previous_single", []string{"Shift+Backspace", "Shift+KeyP"}, []string{"Shift+RightClick", "Shift+WheelUp"}, "Single page backward (fine adjustment)"},
	{"toggle_book_mode", []string{"KeyB"}, []string{"MiddleClick"}, "Toggle book mode (dual image view)"},
	{"toggle_reading_direction", []string{"Shift+KeyB"}, []string{"Ctrl+MiddleClick"}, "Toggle reading direction (LTR ↔ RTL)"},
	{"fullscreen", []string{"Enter"}, []string{"DoubleLeftClick"}, "Toggle fullscreen"},
	{"page_input", []string{"KeyG"}, []string{"Ctrl+LeftClick"}, "Go to page (enter page number)"},
	{"jump_first", []string{"Home", "Shift+Comma"}, []string{}, "Jump to first page"},
	{"jump_last", []string{"End", "Shift+Period"}, []string{}, "Jump to last page"},
	{"rotate_left", []string{"KeyL"}, []string{}, "Rotate left 90 degrees"},
	{"rotate_right", []string{"KeyR"}, []string{}, "Rotate right 90 degrees"},
	{"flip_horizontal", []string{"KeyH"}, []string{}, "Flip horizontally"},
	{"flip_vertical", []string{"KeyV"}, []string{}, "Flip vertically"},
	{"cycle_sort", []string{"Shift+KeyS"}, []string{"Alt+MiddleClick"}, "Cycle sort method (Natural/Simple/Entry)"},
	{"expand_directory", []string{"KeyS"}, []string{}, "Scan directory images (single file mode)"},
	
	// Zoom and pan actions
	{"zoom_in", []string{"Equal", "Shift+Equal"}, []string{"Ctrl+WheelUp"}, "Zoom in"},
	{"zoom_out", []string{"Minus"}, []string{"Ctrl+WheelDown"}, "Zoom out"},
	{"zoom_reset", []string{"Key0"}, []string{"Shift+MiddleClick"}, "Reset to 100% zoom"},
	{"zoom_fit", []string{"KeyF"}, []string{"Alt+LeftClick"}, "Toggle fit to window mode"},
	
	// Pan actions (for manual zoom mode)
	{"pan_up", []string{"ArrowUp"}, []string{}, "Pan up"},
	{"pan_down", []string{"ArrowDown"}, []string{}, "Pan down"},
	{"pan_left", []string{"ArrowLeft"}, []string{}, "Pan left"},
	{"pan_right", []string{"ArrowRight"}, []string{}, "Pan right"},
}

// ActionExecutor provides centralized action execution logic
// This eliminates the need for duplicate ExecuteAction implementations
// in both KeybindingManager and MousebindingManager
type ActionExecutor struct{}

// NewActionExecutor creates a new ActionExecutor instance
func NewActionExecutor() *ActionExecutor {
	return &ActionExecutor{}
}

// ExecuteAction executes the given action using the InputActions interface
// This is the single source of truth for all action execution logic
func (ae *ActionExecutor) ExecuteAction(action string, inputActions InputActions, inputState InputState) bool {
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
	
	// Zoom and pan actions
	case "zoom_in":
		inputActions.ZoomIn()
	case "zoom_out":
		inputActions.ZoomOut()
	case "zoom_reset":
		inputActions.ZoomReset()
	case "zoom_fit":
		inputActions.ZoomFit()
	case "pan_up":
		inputActions.PanUp()
	case "pan_down":
		inputActions.PanDown()
	case "pan_left":
		inputActions.PanLeft()
	case "pan_right":
		inputActions.PanRight()
	
	default:
		return false
	}

	return true
}

// globalActionExecutor is the global instance of ActionExecutor used throughout the application
var globalActionExecutor = NewActionExecutor()

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

// GetDefaultMousebindings returns a map of action names to their default mouse bindings
func GetDefaultMousebindings() map[string][]string {
	mousebindings := make(map[string][]string)
	for _, action := range actionDefinitions {
		mousebindings[action.Name] = action.MouseActions
	}
	return mousebindings
}
