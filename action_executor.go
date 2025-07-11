package main

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
	default:
		return false
	}

	return true
}
