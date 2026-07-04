package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// requireFixture skips the test if the given fixture file is not present.
// test_images/ is gitignored (see .gitignore), so these fixtures are
// developer-provided locally rather than checked into the repo.
func requireFixture(t *testing.T, path string) string {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture %s not available: %v", path, err)
	}
	return path
}

// firstFileEntryName returns the name of the first non-directory entry in
// archivePath, in physical order.
func firstFileEntryName(t *testing.T, archivePath string) string {
	t.Helper()
	h, err := openArchiveHandle(archivePath)
	if err != nil {
		t.Fatalf("openArchiveHandle(%s): %v", archivePath, err)
	}
	defer h.Close()

	for _, e := range h.Entries() {
		if !e.IsDir {
			return e.Name
		}
	}
	t.Fatalf("no file entries found in %s", archivePath)
	return ""
}

func fileEntryNames(t *testing.T, archivePath string) []string {
	t.Helper()
	h, err := openArchiveHandle(archivePath)
	if err != nil {
		t.Fatalf("openArchiveHandle(%s): %v", archivePath, err)
	}
	defer h.Close()

	var names []string
	for _, e := range h.Entries() {
		if !e.IsDir {
			names = append(names, e.Name)
		}
	}
	return names
}

func copyFileForTest(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("writing %s: %v", dst, err)
	}
}

func setMTimeForTest(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// TestPureArchiveHandleRoundTrip verifies Entries() ordering and that
// sequential ReadEntry calls (in the order Entries() reports) return the
// same bytes as an independent freshly-opened handle, for each supported
// archive format.
func TestPureArchiveHandleRoundTrip(t *testing.T) {
	fixtures := []struct {
		name string
		path string
	}{
		{"zip", filepath.Join("test_images", "test_archive.zip")},
		{"7z", filepath.Join("test_images", "test_archive.7z")},
		{"rar", filepath.Join("test_images", "test11.rar")},
	}

	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			path := requireFixture(t, fx.path)

			listHandle, err := openArchiveHandle(path)
			if err != nil {
				t.Fatalf("openArchiveHandle: %v", err)
			}
			entries := listHandle.Entries()
			if err := listHandle.Close(); err != nil {
				t.Fatalf("closing enumeration handle: %v", err)
			}
			if len(entries) == 0 {
				t.Fatalf("expected at least one entry in %s", path)
			}

			readHandle, err := openArchiveHandle(path)
			if err != nil {
				t.Fatalf("openArchiveHandle: %v", err)
			}
			defer readHandle.Close()

			sawFile := false
			for _, entry := range entries {
				if entry.IsDir {
					continue
				}
				sawFile = true

				data, err := readHandle.ReadEntry(entry.Name)
				if err != nil {
					t.Fatalf("ReadEntry(%q): %v", entry.Name, err)
				}
				if len(data) == 0 {
					t.Fatalf("ReadEntry(%q) returned empty data", entry.Name)
				}

				compareHandle, err := openArchiveHandle(path)
				if err != nil {
					t.Fatalf("openArchiveHandle (compare): %v", err)
				}
				want, err := compareHandle.ReadEntry(entry.Name)
				if err != nil {
					compareHandle.Close()
					t.Fatalf("compare ReadEntry(%q): %v", entry.Name, err)
				}
				compareHandle.Close()

				if !bytes.Equal(data, want) {
					t.Fatalf("ReadEntry(%q) mismatch: got %d bytes, want %d bytes", entry.Name, len(data), len(want))
				}
			}
			if !sawFile {
				t.Fatalf("expected at least one non-directory entry in %s", path)
			}
		})
	}
}

// TestPureArchiveHandleOutOfOrder verifies that zip/7z support true random
// access on one handle, while rar reports errArchivePositionPassed when an
// earlier entry is requested after a later one.
func TestPureArchiveHandleOutOfOrder(t *testing.T) {
	cases := []struct {
		name         string
		path         string
		expectRarErr bool
	}{
		{"zip", filepath.Join("test_images", "test_archive.zip"), false},
		{"7z", filepath.Join("test_images", "test_archive.7z"), false},
		{"rar", filepath.Join("test_images", "test11.rar"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := requireFixture(t, tc.path)
			names := fileEntryNames(t, path)
			if len(names) < 2 {
				t.Fatalf("need at least 2 file entries in %s, got %d", path, len(names))
			}
			earlier, later := names[0], names[len(names)-1]

			handle, err := openArchiveHandle(path)
			if err != nil {
				t.Fatalf("openArchiveHandle: %v", err)
			}
			defer handle.Close()

			if _, err := handle.ReadEntry(later); err != nil {
				t.Fatalf("ReadEntry(later=%q): %v", later, err)
			}

			_, err = handle.ReadEntry(earlier)
			if tc.expectRarErr {
				if !errors.Is(err, errArchivePositionPassed) {
					t.Fatalf("ReadEntry(earlier=%q) after later: got err=%v, want errArchivePositionPassed", earlier, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadEntry(earlier=%q) after later: %v", earlier, err)
			}
		})
	}
}

// TestPureArchiveHandleCacheReuseCountsOpens exercises the whole point of
// this change: reading entries from a solid-style forward-only rar in
// increasing order should reuse a single open handle, while reading
// backward forces exactly one reopen.
func TestPureArchiveHandleCacheReuseCountsOpens(t *testing.T) {
	path := requireFixture(t, filepath.Join("test_images", "test11.rar"))
	names := fileEntryNames(t, path)
	if len(names) < 3 {
		t.Fatalf("need at least 3 file entries in %s, got %d", path, len(names))
	}

	before := archiveOpenCount.Load()
	cache := newArchiveHandleCache()
	defer cache.closeAll()

	for _, name := range names[:3] {
		if _, err := cache.readEntry(path, name); err != nil {
			t.Fatalf("readEntry(%q): %v", name, err)
		}
	}
	if got := archiveOpenCount.Load() - before; got != 1 {
		t.Fatalf("expected exactly 1 open after 3 in-order reads, got %d", got)
	}

	// Reading an earlier entry must trigger exactly one reopen+retry.
	if _, err := cache.readEntry(path, names[0]); err != nil {
		t.Fatalf("readEntry(%q) (earlier, after reopen): %v", names[0], err)
	}
	if got := archiveOpenCount.Load() - before; got != 2 {
		t.Fatalf("expected exactly 2 opens after reading an earlier entry, got %d", got)
	}
}

// TestPureArchiveHandleCacheStaleness verifies that when the archive file
// on disk changes (different content, different mtime/size), the cache
// notices and serves fresh data instead of stale bytes from the old
// handle.
func TestPureArchiveHandleCacheStaleness(t *testing.T) {
	original := requireFixture(t, filepath.Join("test_images", "test_archive.zip"))
	replacement := requireFixture(t, filepath.Join("test_images", "test9", "04.zip"))

	originalEntry := firstFileEntryName(t, original)
	replacementEntry := firstFileEntryName(t, replacement)

	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "swap.zip")

	copyFileForTest(t, original, archivePath)
	setMTimeForTest(t, archivePath, time.Now().Add(-time.Hour))

	cache := newArchiveHandleCache()
	defer cache.closeAll()

	gotFirst, err := cache.readEntry(archivePath, originalEntry)
	if err != nil {
		t.Fatalf("readEntry(%q) before swap: %v", originalEntry, err)
	}

	wantFirst, err := (func() ([]byte, error) {
		h, err := openArchiveHandle(original)
		if err != nil {
			return nil, err
		}
		defer h.Close()
		return h.ReadEntry(originalEntry)
	})()
	if err != nil {
		t.Fatalf("reading ground truth from %s: %v", original, err)
	}
	if !bytes.Equal(gotFirst, wantFirst) {
		t.Fatalf("readEntry before swap returned unexpected bytes")
	}

	// Overwrite the same path with a different archive, with a distinctly
	// different mtime/size so staleness detection is unambiguous.
	copyFileForTest(t, replacement, archivePath)
	setMTimeForTest(t, archivePath, time.Now())

	gotSecond, err := cache.readEntry(archivePath, replacementEntry)
	if err != nil {
		t.Fatalf("readEntry(%q) after swap: %v", replacementEntry, err)
	}

	wantSecond, err := (func() ([]byte, error) {
		h, err := openArchiveHandle(replacement)
		if err != nil {
			return nil, err
		}
		defer h.Close()
		return h.ReadEntry(replacementEntry)
	})()
	if err != nil {
		t.Fatalf("reading ground truth from %s: %v", replacement, err)
	}
	if !bytes.Equal(gotSecond, wantSecond) {
		t.Fatalf("readEntry after swap returned stale or incorrect bytes")
	}
}

// TestPureArchiveHandleCacheLRUBound verifies the cache never holds more
// than maxCachedArchiveHandles open handles, and that exceeding the bound
// closes the oldest one.
func TestPureArchiveHandleCacheLRUBound(t *testing.T) {
	sources := []string{
		filepath.Join("test_images", "test_archive.zip"),
		filepath.Join("test_images", "test_archive.7z"),
		filepath.Join("test_images", "test11.rar"),
		filepath.Join("test_images", "test9", "04.zip"),
	}
	for _, s := range sources {
		requireFixture(t, s)
	}

	tempDir := t.TempDir()
	var paths []string
	var entryNames []string
	for i, s := range sources {
		dst := filepath.Join(tempDir, "archive"+string(rune('0'+i))+filepath.Ext(s))
		copyFileForTest(t, s, dst)
		paths = append(paths, dst)
		entryNames = append(entryNames, firstFileEntryName(t, dst))
	}

	closedBefore := archiveCloseCount.Load()
	cache := newArchiveHandleCache()
	defer cache.closeAll()

	for i, p := range paths {
		if _, err := cache.readEntry(p, entryNames[i]); err != nil {
			t.Fatalf("readEntry(%s): %v", p, err)
		}
		if got := cache.lru.Len(); got > maxCachedArchiveHandles {
			t.Fatalf("cache exceeded bound after opening archive %d: len=%d", i, got)
		}
	}

	if got := cache.lru.Len(); got != maxCachedArchiveHandles {
		t.Fatalf("expected cache to hold %d handles, got %d", maxCachedArchiveHandles, got)
	}
	if cache.lru.Contains(paths[0]) {
		t.Fatalf("expected the first archive (%s) to have been evicted", paths[0])
	}
	if got := archiveCloseCount.Load() - closedBefore; got != 1 {
		t.Fatalf("expected exactly 1 close from eviction, got %d", got)
	}
}
