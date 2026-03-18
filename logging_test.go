package main

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	lru "github.com/hashicorp/golang-lru/v2"
	"nv/navlogic"
)

func TestFormatLogLineStablePrefixAndQuoting(t *testing.T) {
	line := formatLogLine(logLevelDebug, "cache", "cache_load_complete",
		"path", "/tmp/a b.png",
		"idx", 3,
		"ok", true,
		"err", errors.New("boom"),
	)

	wantPrefix := `level=debug component="cache" event="cache_load_complete"`
	if !strings.HasPrefix(line, wantPrefix) {
		t.Fatalf("prefix = %q, want prefix %q", line, wantPrefix)
	}
	if !strings.Contains(line, ` path="/tmp/a b.png"`) {
		t.Fatalf("expected quoted path in %q", line)
	}
	if !strings.Contains(line, ` idx=3`) {
		t.Fatalf("expected numeric field in %q", line)
	}
	if !strings.Contains(line, ` ok=true`) {
		t.Fatalf("expected bool field in %q", line)
	}
	if !strings.Contains(line, ` err="boom"`) {
		t.Fatalf("expected quoted error in %q", line)
	}
}

func TestFormatLogLineOddKeyValueCount(t *testing.T) {
	line := formatLogLine(logLevelInfo, "input", "action", "source", "keyboard", "dangling")

	if !strings.Contains(line, ` kv_error="odd_argument_count"`) {
		t.Fatalf("expected kv_error in %q", line)
	}
	if !strings.Contains(line, ` orphan_value="dangling"`) {
		t.Fatalf("expected orphan_value in %q", line)
	}
}

func TestDebugKVRespectsDebugMode(t *testing.T) {
	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	prevDebugMode := debugMode
	log.SetOutput(&buf)
	log.SetFlags(0)
	debugMode = false
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		debugMode = prevDebugMode
	})

	debugKV("input", "action", "source", "keyboard")
	if buf.Len() != 0 {
		t.Fatalf("expected no debug output, got %q", buf.String())
	}

	debugMode = true
	debugKV("input", "action", "source", "keyboard")
	got := buf.String()
	if !strings.Contains(got, `level=debug component="input" event="action"`) {
		t.Fatalf("unexpected debug output %q", got)
	}
	if !strings.Contains(got, ` source="keyboard"`) {
		t.Fatalf("missing source in %q", got)
	}
}

func TestInfoWarnErrorAlwaysLog(t *testing.T) {
	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	prevDebugMode := debugMode
	log.SetOutput(&buf)
	log.SetFlags(0)
	debugMode = false
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		debugMode = prevDebugMode
	})

	infoKV("config", "loaded", "path", "/tmp/config.json")
	warnKV("config", "fallback", "reason", "invalid")
	errorKV("cache", "load_failed", "path", "/tmp/a.png")

	got := buf.String()
	for _, want := range []string{
		`level=info component="config" event="loaded"`,
		`level=warn component="config" event="fallback"`,
		`level=error component="cache" event="load_failed"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestLogDisplayPlanEmitsStructuredNavigationEvents(t *testing.T) {
	left := ebiten.NewImage(100, 200)
	right := ebiten.NewImage(100, 200)
	g := &Game{
		imageManager: &stubImageManager{
			paths:  []ImagePath{{Path: "left.png"}, {Path: "right.png"}},
			images: []*ebiten.Image{left, right},
		},
		zoomState: NewZoomState(),
		config: Config{
			BookMode:             true,
			AspectRatioThreshold: 1.5,
		},
		bookMode: true,
	}

	state := g.navigationState()
	plan := navlogic.PlanDisplay(state, g.pageMetricsAt)
	got := captureLogOutput(t, true, func() {
		g.logDisplayPlan("calculate_display_content", state, plan)
	})

	for _, want := range []string{
		`event="display_plan"`,
		`context="calculate_display_content"`,
		`event="book_decision"`,
		`left_idx=0`,
		`right_idx=1`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestDefaultImageManagerGetImageLogsCacheMiss(t *testing.T) {
	cache, err := lru.NewWithEvict[string, *ebiten.Image](2, func(_ string, img *ebiten.Image) {
		if img != nil {
			img.Deallocate()
		}
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	manager := newDefaultImageManager(cache)
	t.Cleanup(func() {
		manager.StopPreload()
	})
	manager.SetPaths([]ImagePath{{Path: "/tmp/missing.png"}})

	got := captureLogOutput(t, true, func() {
		_ = manager.GetImage(0)
	})

	for _, want := range []string{
		`event="cache_lookup_miss"`,
		`idx=0`,
		`path="/tmp/missing.png"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestConfigureLogOutputWritesToFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "nv.log")
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})

	log.SetFlags(0)
	file, err := configureLogOutput(logPath)
	if err != nil {
		t.Fatalf("configureLogOutput() error = %v", err)
	}
	if file == nil {
		t.Fatal("expected log file handle")
	}

	infoKV("startup", "log_file_enabled", "path", logPath)
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `event="log_file_enabled"`) {
		t.Fatalf("missing log event in %q", got)
	}
	if !strings.Contains(got, `path=`) {
		t.Fatalf("missing path field in %q", got)
	}
}

func captureLogOutput(t *testing.T, debug bool, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	prevDebugMode := debugMode
	log.SetOutput(&buf)
	log.SetFlags(0)
	debugMode = debug
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		debugMode = prevDebugMode
	})

	fn()
	return buf.String()
}
