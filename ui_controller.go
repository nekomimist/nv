package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/ebitenui/ebitenui"
	uiimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/themes"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

const errorWindowDrawLayer = -10

// Numeric input only needs periodic redraws while its caret is idle; input and
// other UI changes set redrawRequested immediately.
const numericInputIdleRedrawInterval = 100 * time.Millisecond

const (
	helpBindingColumnBaseMaxWidth = 600
	helpTableHorizontalChrome     = 144
	minEbitenUIFontSize           = 16.0
	maxEbitenUIFontSize           = 32.0
)

type uiErrorWindow struct {
	key    string
	window *widget.Window
	remove widget.RemoveWindowFunc
	rect   image.Rectangle
}

type uiSettingRow struct {
	label  *widget.Container
	editor *widget.Container
}

// UIController owns the application's single retained EbitenUI tree. The
// image canvas remains in Renderer; this controller supplies page-local
// errors and modal application panels above it.
type UIController struct {
	game *Game
	ui   *ebitenui.UI
	// baseTheme owns the standard dark-theme images and metrics. Sized
	// variants shallow-copy its parameter structs and replace only fonts.
	baseTheme *widget.Theme

	helpOverlay              *widget.Container
	helpTable                *widget.Container
	helpStatus               *widget.Container
	settingsOverlay          *widget.Container
	settingsPanel            *widget.Container
	settingsScroller         *widget.ScrollContainer
	settingsSlider           *widget.Slider
	settingsHorizontalSlider *widget.Slider
	settingsSelectionPending bool
	lastSettingsDrawSize     image.Point
	settingsRows             []uiSettingRow
	numericInputs            map[string]*widget.TextInput
	settingCheckboxes        map[string]*widget.Checkbox
	settingEnums             map[string]*widget.ListComboButton
	settingRowIdle           *uiimage.NineSlice
	settingRowActive         *uiimage.NineSlice

	lastHelpVisible       bool
	lastHelpLayoutWidth   int
	lastSettingsVisible   bool
	lastSettingsConfig    Config
	lastSettingsIndex     int
	suppressNumericCommit bool
	lastUIFontSize        float64
	redrawRequested       bool
	lastPointerPosition   image.Point
	hasPointerPosition    bool
	uiPressedKeysScratch  []ebiten.Key
	lastUIDrawAt          time.Time

	errorWindows [2]uiErrorWindow
}

func NewUIController(game *Game) *UIController {
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewStackedLayout()))
	baseTheme := themes.GetBasicDarkTheme()
	controller := &UIController{
		game:            game,
		baseTheme:       baseTheme,
		lastUIFontSize:  -1,
		redrawRequested: true,
		settingRowIdle:  uiimage.NewNineSliceColor(color.NRGBA{R: 30, G: 30, B: 30, A: 255}),
		settingRowActive: uiimage.NewNineSliceColor(
			color.NRGBA{R: 58, G: 58, B: 68, A: 255},
		),
	}
	controller.helpOverlay, controller.helpTable, controller.helpStatus = controller.buildHelpOverlay()
	controller.settingsOverlay, controller.settingsPanel = controller.buildSettingsOverlay()
	controller.helpOverlay.GetWidget().SetVisibility(widget.Visibility_Hide)
	controller.settingsOverlay.GetWidget().SetVisibility(widget.Visibility_Hide)
	root.AddChild(controller.helpOverlay, controller.settingsOverlay)

	controller.ui = &ebitenui.UI{
		Container:    root,
		PrimaryTheme: baseTheme,
	}
	controller.syncUIFontSize()
	return controller
}

func (c *UIController) ClearFocus() {
	if c != nil && c.ui != nil {
		c.ui.ClearFocus()
	}
}

func (c *UIController) HasFocus() bool {
	return c != nil && c.ui != nil && c.ui.GetFocusedWidget() != nil
}

func (c *UIController) IsEditingNumericInput() bool {
	if c == nil || c.ui == nil {
		return false
	}
	_, ok := c.ui.GetFocusedWidget().(*widget.TextInput)
	return ok
}

func (c *UIController) CancelFocusedNumericInput() bool {
	if c == nil || c.ui == nil || c.game == nil {
		return false
	}
	input, ok := c.ui.GetFocusedWidget().(*widget.TextInput)
	if !ok {
		return false
	}
	for name, candidate := range c.numericInputs {
		if candidate != input {
			continue
		}
		if value, found := getSettingNumericInputText(c.game.pendingConfig, name); found {
			input.SetText(value)
		}
		c.suppressNumericCommit = true
		c.ui.ClearFocus()
		c.suppressNumericCommit = false
		return true
	}
	return false
}

func (c *UIController) CommitSettingsInputs() {
	if c == nil {
		return
	}
	for name, input := range c.numericInputs {
		c.commitNumericInput(name, input)
	}
}

func (c *UIController) DiscardSettingsInputs() {
	if c == nil || c.ui == nil {
		return
	}
	c.suppressNumericCommit = true
	c.ui.ClearFocus()
	c.suppressNumericCommit = false
}

func (c *UIController) Update() {
	if c == nil || c.ui == nil {
		return
	}
	c.syncModalState()
	c.captureUIInputActivity()
	c.ui.Update()
}

func (c *UIController) Draw(screen *ebiten.Image, drawNativeOverlays func(*ebiten.Image)) {
	if c == nil || c.ui == nil {
		if drawNativeOverlays != nil {
			drawNativeOverlays(screen)
		}
		return
	}
	c.syncModalState()
	if size := screen.Bounds().Size(); c.game.showSettings && size != c.lastSettingsDrawSize {
		c.lastSettingsDrawSize = size
		c.settingsSelectionPending = true
	}
	if width := screen.Bounds().Dx(); c.game.showHelp && width != c.lastHelpLayoutWidth {
		c.rebuildHelpContents(width)
		c.lastHelpLayoutWidth = width
	}
	c.syncErrorWindows(screen.Bounds().Dx(), screen.Bounds().Dy())
	c.ui.PreRenderHook = drawNativeOverlays
	c.ui.Draw(screen)
	c.lastUIDrawAt = time.Now()
	// Row geometry is available after EbitenUI lays out the scroll content.
	// If selection moved offscreen, present the adjusted scroll on the next frame.
	c.redrawRequested = c.scrollToSettingsSelection()
}

func (c *UIController) NeedsRedraw() bool {
	if c == nil || c.game == nil {
		return false
	}
	if c.redrawRequested {
		return true
	}
	if c.IsEditingNumericInput() && numericInputRedrawDue(c.lastUIDrawAt, time.Now()) {
		return true
	}
	return c.errorWindowsOutOfSync()
}

func numericInputRedrawDue(lastDraw, now time.Time) bool {
	return lastDraw.IsZero() || now.Sub(lastDraw) >= numericInputIdleRedrawInterval
}

func (c *UIController) hasVisibleUI() bool {
	if c == nil || c.game == nil {
		return false
	}
	if c.game.showHelp || c.game.showSettings || c.displayHasFailure() {
		return true
	}
	for _, window := range c.errorWindows {
		if window.window != nil {
			return true
		}
	}
	return false
}

func (c *UIController) captureUIInputActivity() {
	if !c.hasVisibleUI() {
		c.hasPointerPosition = false
		return
	}

	x, y := ebiten.CursorPosition()
	position := image.Pt(x, y)
	if !c.hasPointerPosition || position != c.lastPointerPosition {
		c.redrawRequested = true
		c.lastPointerPosition = position
		c.hasPointerPosition = true
	}
	if wheelX, wheelY := ebiten.Wheel(); wheelX != 0 || wheelY != 0 {
		c.redrawRequested = true
	}
	for _, button := range []ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonRight, ebiten.MouseButtonMiddle} {
		if ebiten.IsMouseButtonPressed(button) || inpututil.IsMouseButtonJustReleased(button) {
			c.redrawRequested = true
			break
		}
	}
	c.uiPressedKeysScratch = inpututil.AppendPressedKeys(c.uiPressedKeysScratch[:0])
	if len(c.uiPressedKeysScratch) > 0 {
		c.redrawRequested = true
	}
}

func (c *UIController) errorWindowsOutOfSync() bool {
	if c.game.showHelp || c.game.showSettings {
		return false
	}
	var images [2]DisplayImage
	if content := c.game.displayContent; content != nil {
		images = [2]DisplayImage{content.LeftImage, content.RightImage}
	}
	for slot, img := range images {
		_, failed := displayImageFailure(img)
		if failed != (c.errorWindows[slot].window != nil) {
			return true
		}
	}
	return false
}

// PointerCaptured lets the existing viewer input layer avoid reacting to a
// click or wheel event that belongs to an error card. Modal panels are
// handled by InputHandler's explicit help/settings modes.
func (c *UIController) PointerCaptured() bool {
	if c == nil {
		return false
	}
	x, y := ebiten.CursorPosition()
	p := image.Pt(x, y)
	for _, ew := range c.errorWindows {
		if ew.window != nil && p.In(ew.rect) {
			return true
		}
	}
	return false
}

func (c *UIController) syncModalState() {
	if c.game == nil {
		return
	}
	c.syncUIFontSize()
	if c.game.showHelp != c.lastHelpVisible {
		c.redrawRequested = true
		if c.game.showHelp {
			width := c.game.currentLogicalW
			if width <= 0 {
				width = defaultWidth
			}
			c.rebuildHelpContents(width)
			c.lastHelpLayoutWidth = width
			c.helpOverlay.GetWidget().SetVisibility(widget.Visibility_Show)
		} else {
			c.helpOverlay.GetWidget().SetVisibility(widget.Visibility_Hide)
			c.ui.ClearFocus()
		}
		c.lastHelpVisible = c.game.showHelp
	}

	if c.game.showSettings != c.lastSettingsVisible {
		c.redrawRequested = true
		if c.game.showSettings {
			// Retain the widgets and their screen-sized render buffers across
			// opens. Recreating them causes expensive GPU allocations at 4K.
			if c.settingsScroller == nil {
				c.buildSettingsPanel()
			} else {
				c.syncSettingsControls()
				c.updateSettingsSelection(c.lastSettingsIndex, c.game.settingsIndex)
			}
			c.settingsScroller.ScrollTop = 0
			c.settingsScroller.ScrollLeft = 0
			c.settingsSlider.Current = 0
			c.settingsHorizontalSlider.Current = 0
			c.settingsOverlay.GetWidget().SetVisibility(widget.Visibility_Show)
			c.lastSettingsConfig = c.game.pendingConfig
			c.lastSettingsIndex = c.game.settingsIndex
			c.settingsSelectionPending = true
		} else {
			c.settingsOverlay.GetWidget().SetVisibility(widget.Visibility_Hide)
			c.ui.ClearFocus()
			for _, combo := range c.settingEnums {
				combo.SetContentVisible(false)
			}
			c.lastSettingsConfig = Config{}
		}
		c.lastSettingsVisible = c.game.showSettings
	}
	if c.game.showSettings {
		if c.game.settingsIndex != c.lastSettingsIndex {
			c.updateSettingsSelection(c.lastSettingsIndex, c.game.settingsIndex)
			c.lastSettingsIndex = c.game.settingsIndex
			c.redrawRequested = true
			c.settingsSelectionPending = true
		}
		if !editableSettingsEqual(c.game.pendingConfig, c.lastSettingsConfig) {
			c.syncSettingsControls()
			c.lastSettingsConfig = c.game.pendingConfig
			c.redrawRequested = true
		}
	}
}

func effectiveEbitenUIFontSize(configured float64) float64 {
	return min(maxEbitenUIFontSize, max(minEbitenUIFontSize, configured))
}

// syncUIFontSize previews pending FontSize changes inside Settings. Outside
// Settings, only the saved config drives the theme, so Cancel naturally
// restores the previous size and Save keeps the previewed size.
func (c *UIController) syncUIFontSize() {
	if c == nil || c.ui == nil || c.game == nil || c.baseTheme == nil {
		return
	}
	configured := c.game.config.FontSize
	if c.game.showSettings {
		configured = c.game.pendingConfig.FontSize
	}
	size := effectiveEbitenUIFontSize(configured)
	if size == c.lastUIFontSize {
		return
	}

	theme := ebitenUIThemeWithFontSize(c.baseTheme, size)
	c.ui.PrimaryTheme = theme
	c.redrawRequested = true
	c.settingsSelectionPending = c.game.showSettings
	for slot := range c.errorWindows {
		window := c.errorWindows[slot].window
		if window == nil || window.GetContainer() == nil {
			continue
		}
		window.GetContainer().GetWidget().SetTheme(theme)
		window.RequestRelayout()
	}
	c.lastUIFontSize = size
}

// ebitenUIThemeWithFontSize preserves the basic dark theme's colors, images,
// and spacing while replacing every text face used by this application.
// Parameter structs are copied before modification so base remains reusable.
func ebitenUIThemeWithFontSize(base *widget.Theme, size float64) *widget.Theme {
	if base == nil || base.DefaultFace == nil {
		return base
	}
	goFace, ok := (*base.DefaultFace).(*text.GoTextFace)
	if !ok || goFace.Source == nil {
		return base
	}

	var face text.Face = &text.GoTextFace{Source: goFace.Source, Size: size}
	facePtr := &face
	theme := *base
	theme.DefaultFace = facePtr

	if base.ButtonTheme != nil {
		params := *base.ButtonTheme
		params.TextFace = facePtr
		theme.ButtonTheme = &params
	}
	if base.LabelTheme != nil {
		params := *base.LabelTheme
		params.Face = facePtr
		theme.LabelTheme = &params
	}
	if base.TextTheme != nil {
		params := *base.TextTheme
		params.Face = facePtr
		theme.TextTheme = &params
	}
	if base.TextInputTheme != nil {
		params := *base.TextInputTheme
		params.Face = facePtr
		theme.TextInputTheme = &params
	}
	if base.TextAreaTheme != nil {
		params := *base.TextAreaTheme
		params.Face = facePtr
		theme.TextAreaTheme = &params
	}
	if base.ListTheme != nil {
		params := *base.ListTheme
		params.EntryFace = facePtr
		theme.ListTheme = &params
	}
	if base.ListComboButtonTheme != nil {
		params := *base.ListComboButtonTheme
		if base.ListComboButtonTheme.List != nil {
			list := *base.ListComboButtonTheme.List
			list.EntryFace = facePtr
			params.List = &list
		}
		if base.ListComboButtonTheme.Button != nil {
			button := *base.ListComboButtonTheme.Button
			button.TextFace = facePtr
			params.Button = &button
		}
		theme.ListComboButtonTheme = &params
	}
	if base.CheckboxTheme != nil {
		params := *base.CheckboxTheme
		if base.CheckboxTheme.Label != nil {
			label := *base.CheckboxTheme.Label
			label.Face = facePtr
			params.Label = &label
		}
		theme.CheckboxTheme = &params
	}
	if base.TabbookTheme != nil {
		params := *base.TabbookTheme
		if base.TabbookTheme.TabButton != nil {
			button := *base.TabbookTheme.TabButton
			button.TextFace = facePtr
			params.TabButton = &button
		}
		theme.TabbookTheme = &params
	}
	return &theme
}

func (c *UIController) buildHelpOverlay() (*widget.Container, *widget.Container, *widget.Container) {
	overlay := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(uiimage.NewNineSliceColor(color.NRGBA{A: 96})),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	panel := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(uiimage.NewBorderedNineSliceColor(
			color.NRGBA{R: 18, G: 18, B: 18, A: 185},
			color.NRGBA{R: 95, G: 95, B: 105, A: 255}, 2,
		)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(0, 8),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false, false}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			StretchHorizontal: true,
			StretchVertical:   true,
			Padding:           widget.NewInsetsSimple(36),
		})),
	)
	title := widget.NewLabel(widget.LabelOpts.LabelText("Help"), widget.LabelOpts.LabelPadding(widget.NewInsetsSimple(12)))
	helpTable := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(3),
		widget.GridLayoutOpts.Spacing(18, 5),
		widget.GridLayoutOpts.Padding(&widget.Insets{Left: 14, Right: 22, Top: 10, Bottom: 10}),
		widget.GridLayoutOpts.Stretch([]bool{false, false, true}, nil),
	)))
	helpScrollArea, _, _, _ := c.newScrollArea(
		helpTable,
		&widget.ScrollContainerImage{
			Idle: uiimage.NewNineSliceColor(color.NRGBA{R: 26, G: 26, B: 28, A: 72}),
			Mask: uiimage.NewNineSliceColor(color.NRGBA{A: 255}),
		},
		240,
		180,
		false,
	)
	helpStatus := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		widget.RowLayoutOpts.Spacing(2),
		widget.RowLayoutOpts.Padding(&widget.Insets{Left: 12, Right: 12, Top: 4, Bottom: 4}),
	)))
	hint := widget.NewLabel(
		widget.LabelOpts.LabelText("Shift+/ or Esc: close   Mouse wheel: scroll"),
		widget.LabelOpts.LabelPadding(&widget.Insets{Left: 12, Right: 12, Top: 4, Bottom: 10}),
	)
	panel.AddChild(title, helpScrollArea, helpStatus, hint)
	overlay.AddChild(panel)
	return overlay, helpTable, helpStatus
}

func (c *UIController) rebuildHelpContents(windowWidth int) {
	if c.game == nil || c.helpTable == nil || c.helpStatus == nil {
		return
	}
	c.helpTable.RemoveChildren()
	c.helpStatus.RemoveChildren()
	face := c.currentUIFontFace()
	preferredActionWidth := 60
	for _, action := range actionDefinitions {
		width, _ := text.Measure(action.Name, face, 0)
		preferredActionWidth = max(preferredActionWidth, int(math.Ceil(width))+8)
	}
	fontScale := effectiveEbitenUIFontSize(c.game.config.FontSize) / 20
	bindingMaxWidth := int(math.Round(helpBindingColumnBaseMaxWidth * fontScale))
	actionWidth, bindingWidth, descriptionWidth := helpColumnWidths(
		windowWidth, preferredActionWidth, bindingMaxWidth,
	)

	c.helpTable.AddChild(
		newHelpCell("Action", actionWidth, color.NRGBA{R: 225, G: 225, B: 235, A: 255}, false),
		newHelpCell("Keyboard / Mouse", bindingWidth, color.NRGBA{R: 225, G: 225, B: 235, A: 255}, false),
		newHelpCell("Description", descriptionWidth, color.NRGBA{R: 225, G: 225, B: 235, A: 255}, false),
	)

	keys := c.game.GetKeybindings()
	mouse := c.game.GetMousebindings()
	for _, action := range actionDefinitions {
		bindings := make([]string, 0, 2)
		if values := keys[action.Name]; len(values) > 0 {
			bindings = append(bindings, "[color=#FFFF64]"+strings.Join(values, ", ")+"[/color]")
		}
		if values := mouse[action.Name]; len(values) > 0 {
			bindings = append(bindings, "[color=#64FFFF]"+strings.Join(values, ", ")+"[/color]")
		}
		if len(bindings) == 0 {
			continue
		}
		c.helpTable.AddChild(
			newHelpCell(wrapHelpActionName(action.Name, actionWidth, face), actionWidth, color.NRGBA{R: 205, G: 205, B: 255, A: 255}, false),
			newHelpCell(strings.Join(bindings, " | "), bindingWidth, color.White, true),
			newHelpCell(action.Description, descriptionWidth, color.NRGBA{R: 205, G: 205, B: 205, A: 255}, false),
		)
	}

	status := c.game.configStatus
	statusColor := color.NRGBA{R: 110, G: 255, B: 130, A: 255}
	if status.Status == "Warning" || status.Status == "Error" {
		statusColor = color.NRGBA{R: 255, G: 195, B: 105, A: 255}
	}
	statusWidth := actionWidth + bindingWidth + descriptionWidth + 36
	c.helpStatus.AddChild(newHelpStatusLabel("System — Config status: "+status.Status, statusColor, statusWidth))
	for i, warning := range status.Warnings {
		if i == 2 {
			c.helpStatus.AddChild(newHelpStatusLabel(fmt.Sprintf("… and %d more warning(s)", len(status.Warnings)-i), color.NRGBA{R: 205, G: 205, B: 205, A: 255}, statusWidth))
			break
		}
		c.helpStatus.AddChild(newHelpStatusLabel("• "+warning, color.NRGBA{R: 255, G: 165, B: 165, A: 255}, statusWidth))
	}
}

func (c *UIController) currentUIFontFace() text.Face {
	if c != nil && c.ui != nil && c.ui.PrimaryTheme != nil && c.ui.PrimaryTheme.DefaultFace != nil {
		return *c.ui.PrimaryTheme.DefaultFace
	}
	return *c.baseTheme.DefaultFace
}

func helpColumnWidths(windowWidth, preferredActionWidth, bindingMaxWidth int) (action, binding, description int) {
	available := max(180, windowWidth-helpTableHorizontalChrome)
	action = min(max(60, preferredActionWidth), max(60, available*35/100))
	remaining := max(1, available-action)
	binding = min(max(80, bindingMaxWidth), max(80, remaining*55/100))
	if action+binding > available-36 {
		binding = max(1, available-action-36)
	}
	description = max(1, available-action-binding)
	return action, binding, description
}

func wrapHelpActionName(name string, maxWidth int, face text.Face) string {
	parts := strings.SplitAfter(name, "_")
	if len(parts) < 2 {
		return wrapHelpActionPart(name, maxWidth, face)
	}
	lines := make([]string, 0, len(parts))
	line := ""
	for _, part := range parts {
		candidate := line + part
		width, _ := text.Measure(candidate, face, 0)
		if line != "" && width > float64(maxWidth) {
			lines = append(lines, line)
			line = ""
		}
		wrappedPart := strings.Split(wrapHelpActionPart(part, maxWidth, face), "\n")
		for i, piece := range wrappedPart {
			if i == 0 && line != "" {
				line += piece
				continue
			}
			if line != "" {
				lines = append(lines, line)
			}
			line = piece
			if i != len(wrappedPart)-1 {
				lines = append(lines, line)
				line = ""
			}
		}
		if len(wrappedPart) > 1 {
			continue
		}
		if line == "" {
			line = part
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func wrapHelpActionPart(part string, maxWidth int, face text.Face) string {
	width, _ := text.Measure(part, face, 0)
	if width <= float64(maxWidth) {
		return part
	}

	runes := []rune(part)
	lines := make([]string, 0, 2)
	start := 0
	for start < len(runes) {
		end := start + 1
		for end <= len(runes) {
			candidateWidth, _ := text.Measure(string(runes[start:end]), face, 0)
			if candidateWidth > float64(maxWidth) {
				if end == start+1 {
					end++
				}
				break
			}
			end++
		}
		end = min(end-1, len(runes))
		lines = append(lines, string(runes[start:end]))
		start = end
	}
	return strings.Join(lines, "\n")
}

func (c *UIController) newScrollArea(content *widget.Container, scrollImage *widget.ScrollContainerImage, minWidth, minHeight int, horizontal bool) (*widget.Container, *widget.ScrollContainer, *widget.Slider, *widget.Slider) {
	scroller := widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(content),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(scrollImage),
		widget.ScrollContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{})),
	)

	pageSize := func() int {
		contentHeight := content.GetWidget().Rect.Dy()
		viewHeight := scroller.ViewRect().Dy()
		if contentHeight <= 0 || viewHeight <= 0 || contentHeight <= viewHeight {
			return 1000
		}
		return max(1, min(1000, int(math.Round(float64(viewHeight)/float64(contentHeight)*1000))))
	}
	slider := widget.NewSlider(
		widget.SliderOpts.Direction(widget.DirectionVertical),
		widget.SliderOpts.MinMax(0, 1000),
		widget.SliderOpts.InitialCurrent(0),
		widget.SliderOpts.PageSizeFunc(pageSize),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			scroller.ScrollTop = float64(args.Slider.Current) / 1000
			c.redrawRequested = true
		}),
		widget.SliderOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{}),
			widget.WidgetOpts.MinSize(14, 0),
		),
	)
	scroller.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		scrolled, ok := args.(*widget.WidgetScrolledEventArgs)
		if !ok {
			return
		}
		overflow := scroller.ContentRect().Dy() - scroller.ViewRect().Dy()
		if overflow <= 0 {
			scroller.ScrollTop = 0
			slider.Current = 0
			return
		}
		scroller.ScrollTop = min(1, max(0, scroller.ScrollTop-scrolled.Y*48/float64(overflow)))
		slider.Current = int(math.Round(scroller.ScrollTop * 1000))
	})

	area := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(4, 0),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{}),
			widget.WidgetOpts.MinSize(minWidth, minHeight),
		),
	)
	area.AddChild(scroller, slider)
	var horizontalSlider *widget.Slider
	if horizontal {
		horizontalSlider = widget.NewSlider(
			widget.SliderOpts.Direction(widget.DirectionHorizontal),
			widget.SliderOpts.MinMax(0, 1000),
			widget.SliderOpts.InitialCurrent(0),
			widget.SliderOpts.PageSizeFunc(func() int {
				width := scroller.ContentRect().Dx()
				if width <= scroller.ViewRect().Dx() || width <= 0 {
					return 1000
				}
				return max(1, int(math.Round(float64(scroller.ViewRect().Dx())/float64(width)*1000)))
			}),
			widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
				scroller.ScrollLeft = float64(args.Slider.Current) / 1000
				c.redrawRequested = true
			}),
			widget.SliderOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, 14)),
		)
		scroller.GetWidget().ScrolledEvent.AddHandler(func(args any) {
			scrolled, ok := args.(*widget.WidgetScrolledEventArgs)
			if !ok {
				return
			}
			overflow := scroller.ContentRect().Dx() - scroller.ViewRect().Dx()
			if overflow > 0 {
				scroller.ScrollLeft = min(1, max(0, scroller.ScrollLeft-scrolled.X*48/float64(overflow)))
				horizontalSlider.Current = int(math.Round(scroller.ScrollLeft * 1000))
			}
		})
		area.AddChild(horizontalSlider, widget.NewContainer())
	}
	return area, scroller, slider, horizontalSlider
}

func newHelpCell(label string, maxWidth int, textColor color.Color, processBBCode bool) *widget.Label {
	return widget.NewLabel(
		widget.LabelOpts.LabelText(label),
		widget.LabelOpts.LabelColor(&widget.LabelColor{Idle: textColor, Disabled: textColor}),
		widget.LabelOpts.LabelPadding(&widget.Insets{Left: 4, Right: 4, Top: 2, Bottom: 2}),
		widget.LabelOpts.TextOpts(
			widget.TextOpts.MaxWidth(float64(maxWidth)),
			widget.TextOpts.ProcessBBCode(processBBCode),
			widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionStart),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				MaxWidth:         maxWidth,
				VerticalPosition: widget.GridLayoutPositionStart,
			})),
		),
	)
}

func newHelpStatusLabel(label string, textColor color.Color, maxWidth int) *widget.Label {
	return widget.NewLabel(
		widget.LabelOpts.LabelText(label),
		widget.LabelOpts.LabelColor(&widget.LabelColor{Idle: textColor, Disabled: textColor}),
		widget.LabelOpts.TextOpts(widget.TextOpts.MaxWidth(float64(maxWidth))),
	)
}

func (c *UIController) buildSettingsOverlay() (*widget.Container, *widget.Container) {
	overlay := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(uiimage.NewNineSliceColor(color.NRGBA{A: 255})),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	panel := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(uiimage.NewBorderedNineSliceColor(
			color.NRGBA{R: 18, G: 18, B: 18, A: 255},
			color.NRGBA{R: 95, G: 95, B: 105, A: 255}, 2,
		)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(0, 10),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			StretchHorizontal: true,
			StretchVertical:   true,
			Padding:           widget.NewInsetsSimple(28),
		})),
	)
	overlay.AddChild(panel)
	return overlay, panel
}

func (c *UIController) buildSettingsPanel() {
	if c.settingsPanel == nil || c.game == nil {
		return
	}
	c.numericInputs = make(map[string]*widget.TextInput)
	c.settingCheckboxes = make(map[string]*widget.Checkbox)
	c.settingEnums = make(map[string]*widget.ListComboButton)
	c.settingsPanel.AddChild(widget.NewLabel(
		widget.LabelOpts.LabelText("Settings"),
		widget.LabelOpts.LabelPadding(widget.NewInsetsSimple(10)),
	))

	content := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(2),
		widget.GridLayoutOpts.Spacing(0, 6),
		widget.GridLayoutOpts.Padding(&widget.Insets{Left: 8, Right: 8, Top: 4, Bottom: 4}),
		widget.GridLayoutOpts.Stretch([]bool{false, true}, nil),
	)))
	content.AddChild(
		newSettingCell("Setting", color.NRGBA{R: 42, G: 42, B: 46, A: 255}),
		newSettingCell("Value / Editor", color.NRGBA{R: 42, G: 42, B: 46, A: 255}),
	)
	for index, spec := range editableSettingSpecs {
		label, editor := c.buildSettingCells(index, spec)
		c.settingsRows = append(c.settingsRows, uiSettingRow{label: label, editor: editor})
		content.AddChild(label, editor)
	}
	scrollArea, scroller, slider, horizontalSlider := c.newScrollArea(
		content,
		&widget.ScrollContainerImage{
			Idle: uiimage.NewNineSliceColor(color.NRGBA{R: 28, G: 28, B: 28, A: 255}),
			Mask: uiimage.NewNineSliceColor(color.NRGBA{R: 28, G: 28, B: 28, A: 255}),
		},
		320,
		200,
		true,
	)
	c.settingsScroller = scroller
	c.settingsSlider = slider
	c.settingsHorizontalSlider = horizontalSlider
	c.settingsPanel.AddChild(scrollArea)

	buttons := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		widget.RowLayoutOpts.Spacing(12),
		widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(8)),
	)))
	for _, action := range []struct {
		label string
		run   func()
	}{{"Save", c.game.SettingsSave}, {"Cancel", c.game.SettingsCancel}} {
		index := len(c.settingsRows)
		cell := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(c.settingRowIdle),
			widget.ContainerOpts.Layout(widget.NewRowLayout(widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(4)))),
		)
		cell.AddChild(widget.NewButton(widget.ButtonOpts.TextLabel(action.label), widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
			action.run()
		})))
		c.selectSettingOnFocus(cell, index)
		c.settingsRows = append(c.settingsRows, uiSettingRow{label: cell, editor: cell})
		buttons.AddChild(cell)
	}
	c.settingsPanel.AddChild(buttons)
}

func (c *UIController) buildSettingCells(index int, spec settingSpec) (*widget.Container, *widget.Container) {
	name := spec.ID
	selected := index == c.game.settingsIndex
	background := c.settingRowIdle
	if selected {
		background = c.settingRowActive
	}
	labelCell := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(background),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Padding(&widget.Insets{Left: 10, Right: 10, Top: 4, Bottom: 4}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{})),
	)
	editorCell := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(background),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(8),
			widget.RowLayoutOpts.Padding(&widget.Insets{Left: 8, Right: 10, Top: 4, Bottom: 4}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{})),
	)
	c.selectSettingOnFocus(editorCell, index)
	label := spec.Label
	if spec.Restart {
		label += " (restart required)"
	}
	labelCell.AddChild(widget.NewLabel(widget.LabelOpts.LabelText(label)))

	if spec.Control == settingControlBool {
		value, _ := settingBoolValue(c.game.pendingConfig, name)
		state := widget.WidgetUnchecked
		if value {
			state = widget.WidgetChecked
		}
		checkbox := widget.NewCheckbox(
			widget.CheckboxOpts.InitialState(state),
			widget.CheckboxOpts.StateChangedHandler(func(args *widget.CheckboxChangedEventArgs) {
				value := args.State == widget.WidgetChecked
				current, _ := settingBoolValue(c.game.pendingConfig, name)
				// Events are deferred; a programmatic sync has already updated
				// pendingConfig and must not move the keyboard selection.
				if !c.game.showSettings || current == value {
					return
				}
				c.game.settingsIndex = index
				setSettingBoolValue(&c.game.pendingConfig, name, value)
			}),
		)
		c.settingCheckboxes[name] = checkbox
		editorCell.AddChild(checkbox)
		return labelCell, editorCell
	}

	if spec.Control == settingControlEnum {
		options := spec.Options
		entries := make([]any, len(options))
		for i := range options {
			entries[i] = options[i]
		}
		current := getSettingValueStringFromConfig(c.game.pendingConfig, name)
		combo := widget.NewListComboButton(
			widget.ListComboButtonOpts.Entries(entries),
			widget.ListComboButtonOpts.InitialEntry(current),
			widget.ListComboButtonOpts.EntryLabelFunc(
				func(entry interface{}) string { return fmt.Sprint(entry) },
				func(entry any) string { return fmt.Sprint(entry) },
			),
			widget.ListComboButtonOpts.EntrySelectedHandler(func(args *widget.ListComboButtonEntrySelectedEventArgs) {
				// EbitenUI emits a deferred selection event while applying
				// InitialEntry. It has no previous entry and must not move the
				// Settings keyboard selection away from the first row.
				value := fmt.Sprint(args.Entry)
				if !c.game.showSettings || args.PreviousEntry == nil || getSettingValueStringFromConfig(c.game.pendingConfig, name) == value {
					return
				}
				c.game.settingsIndex = index
				setSettingEnumValue(&c.game.pendingConfig, name, value)
			}),
		)
		c.settingEnums[name] = combo
		editorCell.AddChild(combo)
		return labelCell, editorCell
	}

	input := c.newNumericSettingInput(index, spec)
	editorCell.AddChild(
		widget.NewButton(widget.ButtonOpts.TextLabel("−"), widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
			c.commitNumericInput(name, input)
			c.game.settingsIndex = index
			c.game.settingsAdjust(true)
		})),
		input,
		widget.NewButton(widget.ButtonOpts.TextLabel("+"), widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
			c.commitNumericInput(name, input)
			c.game.settingsIndex = index
			c.game.settingsAdjust(false)
		})),
	)
	if spec.Unit != "" {
		editorCell.AddChild(widget.NewLabel(widget.LabelOpts.LabelText(spec.Unit)))
	}
	if spec.Hint != "" {
		editorCell.AddChild(widget.NewLabel(
			widget.LabelOpts.LabelText(spec.Hint),
			widget.LabelOpts.LabelColor(&widget.LabelColor{
				Idle:     color.NRGBA{R: 190, G: 190, B: 195, A: 255},
				Disabled: color.NRGBA{R: 190, G: 190, B: 195, A: 255},
			}),
		))
	}
	return labelCell, editorCell
}

func editableSettingsEqual(a, b Config) bool {
	for _, spec := range editableSettingSpecs {
		// Keep float comparisons independent from the rounded display text. The
		// text renderer intentionally uses fewer decimal places than the editor
		// accepts, so comparing it would make distinct pending values look equal.
		switch spec.ID {
		case "FontSize":
			if a.FontSize != b.FontSize {
				return false
			}
			continue
		case "AspectRatioThreshold":
			if a.AspectRatioThreshold != b.AspectRatioThreshold {
				return false
			}
			continue
		case "Mouse.WheelSensitivity":
			if a.MouseSettings.WheelSensitivity != b.MouseSettings.WheelSensitivity {
				return false
			}
			continue
		case "Mouse.DragSensitivity":
			if a.MouseSettings.DragSensitivity != b.MouseSettings.DragSensitivity {
				return false
			}
			continue
		}
		if getSettingValueStringFromConfig(a, spec.ID) != getSettingValueStringFromConfig(b, spec.ID) {
			return false
		}
	}
	return true
}

func (c *UIController) updateSettingsSelection(previous, current int) {
	for _, index := range []int{previous, current} {
		if index < 0 || index >= len(c.settingsRows) {
			continue
		}
		background := c.settingRowIdle
		if index == current {
			background = c.settingRowActive
		}
		row := c.settingsRows[index]
		row.label.SetBackgroundImage(background)
		row.editor.SetBackgroundImage(background)
	}
}

func (c *UIController) selectSettingOnFocus(container *widget.Container, index int) {
	container.GetWidget().FocusEvent.AddHandler(func(args any) {
		if focus, ok := args.(*widget.WidgetFocusEventArgs); ok && focus.Focused {
			c.game.settingsIndex = index
		}
		c.redrawRequested = true
	})
}

func (c *UIController) scrollToSettingsSelection() bool {
	if !c.game.showSettings || !c.settingsSelectionPending || c.settingsScroller == nil {
		return false
	}
	c.settingsSelectionPending = false
	index := c.game.settingsIndex
	// Save and Cancel remain outside the scrolling content.
	if index < 0 || index >= len(editableSettingSpecs) {
		return false
	}
	scroller := c.settingsScroller
	view := scroller.ViewRect()
	overflow := scroller.ContentRect().Dy() - view.Dy()
	if overflow <= 0 {
		return false
	}
	row := c.settingsRows[index]
	rect := row.label.GetWidget().Rect.Union(row.editor.GetWidget().Rect)
	delta := 0
	if rect.Min.Y < view.Min.Y {
		delta = rect.Min.Y - view.Min.Y
	} else if rect.Max.Y > view.Max.Y {
		delta = rect.Max.Y - view.Max.Y
	}
	if delta == 0 {
		return false
	}
	scroller.ScrollTop = min(1, max(0, scroller.ScrollTop+float64(delta)/float64(overflow)))
	c.settingsSlider.Current = int(math.Round(scroller.ScrollTop * 1000))
	return true
}

func (c *UIController) syncSettingsControls() {
	if c == nil || c.game == nil {
		return
	}
	for name, checkbox := range c.settingCheckboxes {
		value, ok := settingBoolValue(c.game.pendingConfig, name)
		if !ok {
			continue
		}
		state := widget.WidgetUnchecked
		if value {
			state = widget.WidgetChecked
		}
		if checkbox.State() != state {
			checkbox.SetState(state)
		}
	}
	for name, combo := range c.settingEnums {
		value := getSettingValueStringFromConfig(c.game.pendingConfig, name)
		if fmt.Sprint(combo.SelectedEntry()) != value {
			combo.SetSelectedEntry(value)
		}
	}
	for name, input := range c.numericInputs {
		value, ok := getSettingNumericInputText(c.game.pendingConfig, name)
		if ok && input.GetText() != value {
			input.SetText(value)
		}
	}
}

func newSettingCell(label string, background color.Color) *widget.Container {
	cell := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(uiimage.NewNineSliceColor(background)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Padding(&widget.Insets{Left: 10, Right: 10, Top: 5, Bottom: 5}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{})),
	)
	cell.AddChild(widget.NewLabel(widget.LabelOpts.LabelText(label)))
	return cell
}

func (c *UIController) newNumericSettingInput(index int, spec settingSpec) *widget.TextInput {
	value, _ := getSettingNumericInputText(c.game.pendingConfig, spec.ID)
	input := widget.NewTextInput(
		widget.TextInputOpts.Validation(func(candidate string) (bool, *string) {
			return validSettingNumericInput(spec.Number, candidate), nil
		}),
		widget.TextInputOpts.ChangedHandler(func(*widget.TextInputChangedEventArgs) {
			c.redrawRequested = true
		}),
		widget.TextInputOpts.WidgetOpts(widget.WidgetOpts.MinSize(140, 0)),
	)
	input.SetText(value)
	input.SubmitEvent.AddHandler(func(any) {
		c.commitNumericInput(spec.ID, input)
		if c.ui != nil {
			c.ui.ClearFocus()
		}
	})
	input.GetWidget().FocusEvent.AddHandler(func(args any) {
		focused, ok := args.(*widget.WidgetFocusEventArgs)
		if !ok || focused.Widget != input {
			return
		}
		c.redrawRequested = true
		if focused.Focused {
			c.game.settingsIndex = index
			return
		}
		c.commitNumericInput(spec.ID, input)
	})
	c.numericInputs[spec.ID] = input
	return input
}

func (c *UIController) commitNumericInput(name string, input *widget.TextInput) bool {
	if c == nil || c.game == nil || !c.game.showSettings || input == nil || c.suppressNumericCommit {
		return false
	}
	normalized, ok := applySettingNumericInput(&c.game.pendingConfig, name, input.GetText())
	if !ok {
		if current, found := getSettingNumericInputText(c.game.pendingConfig, name); found {
			input.SetText(current)
		}
		return false
	}
	input.SetText(normalized)
	// Numeric text is already synchronized in place; avoid disturbing Tab
	// focus merely because it was committed.
	c.lastSettingsConfig = c.game.pendingConfig
	c.redrawRequested = true
	debugKV("config", "settings_text_commit", "setting", name, "value", normalized)
	return true
}

func settingBoolValue(cfg Config, name string) (bool, bool) {
	switch name {
	case "Fullscreen":
		return cfg.Fullscreen, true
	case "BookMode":
		return cfg.BookMode, true
	case "RightToLeft":
		return cfg.RightToLeft, true
	case "FitWidthAlignTop":
		return cfg.FitWidthAlignTop, true
	case "FitHeightAlignLeft":
		return cfg.FitHeightAlignLeft, true
	case "PreloadEnabled":
		return cfg.PreloadEnabled, true
	case "Mouse.EnableMouse":
		return cfg.MouseSettings.EnableMouse, true
	case "Mouse.WheelInverted":
		return cfg.MouseSettings.WheelInverted, true
	case "Mouse.EnableDragPan":
		return cfg.MouseSettings.EnableDragPan, true
	case "Mouse.DragPanInverted":
		return cfg.MouseSettings.DragPanInverted, true
	default:
		return false, false
	}
}

func setSettingBoolValue(cfg *Config, name string, value bool) {
	if cfg == nil {
		return
	}
	switch name {
	case "Fullscreen":
		cfg.Fullscreen = value
	case "BookMode":
		cfg.BookMode = value
	case "RightToLeft":
		cfg.RightToLeft = value
	case "FitWidthAlignTop":
		cfg.FitWidthAlignTop = value
	case "FitHeightAlignLeft":
		cfg.FitHeightAlignLeft = value
	case "PreloadEnabled":
		cfg.PreloadEnabled = value
	case "Mouse.EnableMouse":
		cfg.MouseSettings.EnableMouse = value
	case "Mouse.WheelInverted":
		cfg.MouseSettings.WheelInverted = value
	case "Mouse.EnableDragPan":
		cfg.MouseSettings.EnableDragPan = value
	case "Mouse.DragPanInverted":
		cfg.MouseSettings.DragPanInverted = value
	}
}

func setSettingEnumValue(cfg *Config, name, value string) {
	if cfg == nil {
		return
	}
	switch name {
	case "SortMethod":
		for method := 0; method < 3; method++ {
			if getSortMethodName(method) == value {
				cfg.SortMethod = method
				return
			}
		}
	case "InitialZoomMode":
		cfg.InitialZoomMode = value
	}
}

func (c *UIController) displayHasFailure() bool {
	if c == nil || c.game == nil || c.game.displayContent == nil {
		return false
	}
	if _, ok := displayImageFailure(c.game.displayContent.LeftImage); ok {
		return true
	}
	_, ok := displayImageFailure(c.game.displayContent.RightImage)
	return ok
}

func (c *UIController) syncErrorWindows(width, height int) {
	// Windows are separate EbitenUI input layers. Remove page-local cards
	// while a modal is open so an obscured card cannot consume its wheel or
	// pointer events.
	if c.game.showHelp || c.game.showSettings {
		for slot := range c.errorWindows {
			c.removeErrorWindow(slot)
		}
		return
	}

	var images [2]DisplayImage
	var pages [2]int
	actualImages := 0
	if content := c.game.displayContent; content != nil {
		images = [2]DisplayImage{content.LeftImage, content.RightImage}
		pages = [2]int{content.Metadata.LeftPage, content.Metadata.RightPage}
		actualImages = content.Metadata.ActualImages
	}

	for slot := range c.errorWindows {
		failure, failed := displayImageFailure(images[slot])
		if !failed {
			c.removeErrorWindow(slot)
			continue
		}
		key := fmt.Sprintf("%d|%d|%s|%s", slot, pages[slot], failure.Source, failure.Message)
		if c.errorWindows[slot].key != key {
			c.removeErrorWindow(slot)
			window := c.newErrorWindow(failure, pages[slot])
			c.errorWindows[slot] = uiErrorWindow{
				key:    key,
				window: window,
				remove: c.ui.AddWindowQuietly(window, false),
			}
		}
		slotRect := errorCardSlotRect(slot, actualImages, width, height)
		if actualImages == 2 && images[1-slot] != nil {
			if _, otherFailed := displayImageFailure(images[1-slot]); !otherFailed {
				failedRect := c.game.renderer.displayImageScreenRect(width, height, images[0], images[1], slot)
				validRect := c.game.renderer.displayImageScreenRect(width, height, images[0], images[1], 1-slot)
				if available, ok := mixedErrorCardAvailableRect(width, height, failedRect, validRect); ok {
					slotRect = available
				}
			}
		}
		rect := errorCardRectWithin(slotRect)
		c.errorWindows[slot].rect = rect
		c.errorWindows[slot].window.SetLocation(rect)
	}
}

func (c *UIController) removeErrorWindow(slot int) {
	ew := &c.errorWindows[slot]
	if ew.remove != nil {
		ew.remove()
	}
	*ew = uiErrorWindow{}
}

func (c *UIController) newErrorWindow(failure ImageLoadFailure, page int) *widget.Window {
	card := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(uiimage.NewBorderedNineSliceColor(
			color.NRGBA{R: 78, G: 20, B: 24, A: 245},
			color.NRGBA{R: 235, G: 150, B: 150, A: 255}, 2,
		)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Spacing(0, 8),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
		)),
	)
	title := fmt.Sprintf("Image error — page %d — %s", page, filepath.Base(failure.Source))
	card.AddChild(widget.NewLabel(
		widget.LabelOpts.LabelText(title),
		widget.LabelOpts.LabelPadding(widget.NewInsetsSimple(10)),
	))
	details := fmt.Sprintf("Source: %s\n\nReason: %s", failure.Source, failure.Message)
	card.AddChild(widget.NewTextArea(
		widget.TextAreaOpts.ContainerOpts(widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(180, 100))),
		widget.TextAreaOpts.Text(details),
		widget.TextAreaOpts.TextPadding(widget.Insets{Left: 10, Right: 20, Top: 8, Bottom: 8}),
		widget.TextAreaOpts.ShowVerticalScrollbar(),
	))
	return widget.NewWindow(
		widget.WindowOpts.Contents(card),
		widget.WindowOpts.DrawLayer(errorWindowDrawLayer),
		widget.WindowOpts.BlockLower(false),
		widget.WindowOpts.DisableRelayering(true),
	)
}

func errorCardRect(slot, actualImages, width, height int) image.Rectangle {
	return errorCardRectWithin(errorCardSlotRect(slot, actualImages, width, height))
}

func errorCardSlotRect(slot, actualImages, width, height int) image.Rectangle {
	slotRect := image.Rect(0, 0, width, height)
	if actualImages == 2 {
		mid := width / 2
		if slot == 0 {
			slotRect.Max.X = mid
		} else {
			slotRect.Min.X = mid
		}
	}
	return slotRect
}

func errorCardRectWithin(slotRect image.Rectangle) image.Rectangle {
	margin := min(28, max(4, min(slotRect.Dx(), slotRect.Dy())/8))
	maxWidth, maxHeight := 640, 360
	cardWidth := min(maxWidth, max(1, slotRect.Dx()-margin*2))
	cardHeight := min(maxHeight, max(1, slotRect.Dy()-margin*2))
	x := slotRect.Min.X + (slotRect.Dx()-cardWidth)/2
	y := slotRect.Min.Y + (slotRect.Dy()-cardHeight)/2
	return image.Rect(x, y, x+cardWidth, y+cardHeight)
}

// mixedErrorCardAvailableRect returns the portion of the screen on the
// failed page's side of the successfully rendered page. This preserves the
// roomy half-screen cards used for two failures while preventing the mixed
// success/failure case from covering the valid image when it crosses the
// screen midpoint.
func mixedErrorCardAvailableRect(width, height int, failedRect, validRect image.Rectangle) (image.Rectangle, bool) {
	screen := image.Rect(0, 0, width, height)
	failedRect = failedRect.Intersect(screen)
	validRect = validRect.Intersect(screen)
	if failedRect.Empty() || validRect.Empty() {
		return image.Rectangle{}, false
	}

	failedCenterX := failedRect.Min.X + failedRect.Max.X
	failedCenterY := failedRect.Min.Y + failedRect.Max.Y
	validCenterX := validRect.Min.X + validRect.Max.X
	validCenterY := validRect.Min.Y + validRect.Max.Y
	dx := failedCenterX - validCenterX
	if dx < 0 {
		dx = -dx
	}
	dy := failedCenterY - validCenterY
	if dy < 0 {
		dy = -dy
	}

	available := screen
	if dx >= dy {
		if failedCenterX < validCenterX {
			available.Max.X = validRect.Min.X
		} else {
			available.Min.X = validRect.Max.X
		}
	} else if failedCenterY < validCenterY {
		available.Max.Y = validRect.Min.Y
	} else {
		available.Min.Y = validRect.Max.Y
	}
	if available.Empty() {
		return image.Rectangle{}, false
	}
	return available, true
}
