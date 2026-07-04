package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode"
)

// archiveEntry describes one entry inside an archive, in physical
// (on-disk) order.
type archiveEntry struct {
	Name  string
	IsDir bool
}

// errArchivePositionPassed is returned by a forward-only archiveHandle
// (currently only the rar implementation) when the requested entry lies
// at or before the last entry whose data was already consumed. Callers
// (archiveHandleCache) should close the handle, reopen the archive, and
// retry once.
var errArchivePositionPassed = errors.New("archive: entry position already passed")

// archiveHandle is a minimal read abstraction over ZIP/RAR/7z archives.
// Implementations may optimize monotonically increasing ReadEntry access
// (matching the order returned by Entries); random access must still work
// correctly, just without the speedup — the rar implementation instead
// reports errArchivePositionPassed so the caller can reopen and retry.
type archiveHandle interface {
	// Entries returns the archive's entries in physical/on-disk order.
	Entries() []archiveEntry
	// ReadEntry returns the decompressed bytes of the named entry.
	ReadEntry(name string) ([]byte, error)
	Close() error
}

// archiveOpenCount counts calls to openArchiveHandle, and archiveCloseCount
// counts completed archiveHandle.Close calls. Both exist purely as test
// seams for verifying that archiveHandleCache avoids redundant reopens and
// that evicted/stale handles actually get closed.
var (
	archiveOpenCount  atomic.Int64
	archiveCloseCount atomic.Int64
)

// openArchiveHandle opens archivePath and returns an archiveHandle
// implementation chosen by file extension, matching the formats
// recognized by isArchiveExt.
func openArchiveHandle(archivePath string) (archiveHandle, error) {
	archiveOpenCount.Add(1)

	ext := strings.ToLower(filepath.Ext(archivePath))
	switch ext {
	case ".zip":
		return newZipArchiveHandle(archivePath)
	case ".rar":
		return newRarArchiveHandle(archivePath)
	case ".7z":
		return newSevenZipArchiveHandle(archivePath)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", ext)
	}
}

// zipArchiveHandle wraps a *zip.ReadCloser, indexing all entries at open
// time so ReadEntry supports random access.
type zipArchiveHandle struct {
	archivePath string
	rc          *zip.ReadCloser
	entries     []archiveEntry
	files       map[string]*zip.File
}

func newZipArchiveHandle(archivePath string) (archiveHandle, error) {
	rc, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}

	h := &zipArchiveHandle{
		archivePath: archivePath,
		rc:          rc,
		entries:     make([]archiveEntry, 0, len(rc.File)),
		files:       make(map[string]*zip.File, len(rc.File)),
	}
	for _, f := range rc.File {
		h.entries = append(h.entries, archiveEntry{Name: f.Name, IsDir: f.FileInfo().IsDir()})
		if _, exists := h.files[f.Name]; !exists {
			h.files[f.Name] = f
		}
	}
	return h, nil
}

func (h *zipArchiveHandle) Entries() []archiveEntry { return h.entries }

func (h *zipArchiveHandle) ReadEntry(name string) ([]byte, error) {
	f, ok := h.files[name]
	if !ok {
		return nil, fmt.Errorf("entry %s not found in %s", name, h.archivePath)
	}

	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

func (h *zipArchiveHandle) Close() error {
	archiveCloseCount.Add(1)
	return h.rc.Close()
}

// sevenZipArchiveHandle wraps a *sevenzip.ReadCloser, indexing all entries
// at open time. Keeping the reader open across reads is the main
// optimization: the library keeps a per-folder decompressed-stream pool
// scoped to the OpenReader call, so repeated opens throw that pool away.
type sevenZipArchiveHandle struct {
	archivePath string
	rc          *sevenzip.ReadCloser
	entries     []archiveEntry
	files       map[string]*sevenzip.File
}

func newSevenZipArchiveHandle(archivePath string) (archiveHandle, error) {
	rc, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}

	h := &sevenZipArchiveHandle{
		archivePath: archivePath,
		rc:          rc,
		entries:     make([]archiveEntry, 0, len(rc.File)),
		files:       make(map[string]*sevenzip.File, len(rc.File)),
	}
	for _, f := range rc.File {
		h.entries = append(h.entries, archiveEntry{Name: f.Name, IsDir: f.FileInfo().IsDir()})
		if _, exists := h.files[f.Name]; !exists {
			h.files[f.Name] = f
		}
	}
	return h, nil
}

func (h *sevenZipArchiveHandle) Entries() []archiveEntry { return h.entries }

func (h *sevenZipArchiveHandle) ReadEntry(name string) ([]byte, error) {
	f, ok := h.files[name]
	if !ok {
		return nil, fmt.Errorf("entry %s not found in %s", name, h.archivePath)
	}

	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

func (h *sevenZipArchiveHandle) Close() error {
	archiveCloseCount.Add(1)
	return h.rc.Close()
}

// rarArchiveHandle is a forward-only sequential reader over a RAR archive.
// rardecode.Reader cannot seek backward — reading entry k must first walk
// (and, for solid archives, decompress) through entries 0..k-1 — so this
// type tracks how far the underlying reader has advanced and rejects
// requests for entries at or before that point rather than silently
// re-reading (which is impossible) or returning stale data.
type rarArchiveHandle struct {
	archivePath string
	f           *os.File
	r           *rardecode.Reader
	entries     []archiveEntry
	index       map[string]int // entry name -> position in entries
	pos         int            // position of the last entry whose data was consumed; -1 initially
	eof         bool
}

func newRarArchiveHandle(archivePath string) (archiveHandle, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}

	r, err := rardecode.NewReader(f, "")
	if err != nil {
		f.Close()
		return nil, err
	}

	return &rarArchiveHandle{
		archivePath: archivePath,
		f:           f,
		r:           r,
		index:       make(map[string]int),
		pos:         -1,
	}, nil
}

// advance reads and indexes the next header. It returns io.EOF (without
// touching the underlying reader again) once the archive is exhausted.
func (h *rarArchiveHandle) advance() (*rardecode.FileHeader, error) {
	if h.eof {
		return nil, io.EOF
	}

	header, err := h.r.Next()
	if err != nil {
		if err == io.EOF {
			h.eof = true
		}
		return nil, err
	}

	h.entries = append(h.entries, archiveEntry{Name: header.Name, IsDir: header.IsDir})
	h.index[header.Name] = len(h.entries) - 1
	return header, nil
}

// Entries performs a full forward walk, indexing every remaining header.
// It is meant for one-shot enumeration handles that are closed right
// after — walking to EOF consumes the sequential reader, so a handle used
// this way should not also be used for ReadEntry.
func (h *rarArchiveHandle) Entries() []archiveEntry {
	for {
		if _, err := h.advance(); err != nil {
			break
		}
	}
	return h.entries
}

func (h *rarArchiveHandle) ReadEntry(name string) ([]byte, error) {
	if p, ok := h.index[name]; ok && p <= h.pos {
		return nil, errArchivePositionPassed
	}

	for {
		header, err := h.advance()
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("entry %s not found in %s", name, h.archivePath)
			}
			return nil, err
		}

		if header.Name != name {
			continue
		}

		data, err := io.ReadAll(h.r)
		if err != nil {
			return nil, err
		}
		h.pos = h.index[name]
		return data, nil
	}
}

func (h *rarArchiveHandle) Close() error {
	archiveCloseCount.Add(1)
	return h.f.Close()
}
