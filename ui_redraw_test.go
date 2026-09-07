package main

import (
	"testing"
	"time"
)

func TestPureNumericInputRedrawDue(t *testing.T) {
	lastDraw := time.Unix(100, 0)
	tests := []struct {
		name     string
		lastDraw time.Time
		now      time.Time
		want     bool
	}{
		{name: "initial draw", lastDraw: time.Time{}, now: lastDraw, want: true},
		{name: "before interval", lastDraw: lastDraw, now: lastDraw.Add(99 * time.Millisecond), want: false},
		{name: "at interval", lastDraw: lastDraw, now: lastDraw.Add(numericInputIdleRedrawInterval), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := numericInputRedrawDue(tt.lastDraw, tt.now); got != tt.want {
				t.Fatalf("numericInputRedrawDue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGUI_NumericInputRedrawThrottle(t *testing.T) {
	_, controller, screen := settingsLayoutTestController(t, 800, 600, 24)
	input := controller.numericInputs["FontSize"]
	if input == nil {
		t.Fatal("FontSize input was not created")
	}

	controller.ui.SetFocusedWidget(input)
	controller.ui.Update()
	controller.Draw(screen, nil)
	// The draw above presents the focus change and starts the idle interval.
	controller.redrawRequested = false
	if !controller.IsEditingNumericInput() {
		t.Fatal("FontSize input did not receive focus")
	}
	if controller.NeedsRedraw() {
		t.Fatal("idle numeric input requested redraw before the throttle interval")
	}

	controller.redrawRequested = true
	if !controller.NeedsRedraw() {
		t.Fatal("dirty UI did not bypass the numeric redraw throttle")
	}

	controller.redrawRequested = false
	candidate := "25.0"
	if input.GetText() == candidate {
		candidate = "26.0"
	}
	input.SetText(candidate)
	// EbitenUI dispatches queued text and focus events during Update.
	controller.ui.Update()
	if !controller.redrawRequested {
		t.Fatal("numeric text change did not request an immediate redraw")
	}

	controller.redrawRequested = false
	controller.ui.ClearFocus()
	controller.ui.Update()
	if !controller.redrawRequested {
		t.Fatal("numeric focus change did not request an immediate redraw")
	}
}
