package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

// maxCachedArchiveHandles bounds how many archive files can be kept open
// at once. Archive image loads are rarely interleaved across more than a
// couple of archives at a time (current archive plus maybe one being
// preloaded across a boundary), so a small bound keeps file-descriptor and
// memory use predictable.
const maxCachedArchiveHandles = 3

// cachedArchiveHandle pairs an open archiveHandle with the archive file's
// stat data at open time, so staleness (the file changing on disk) can be
// detected before serving a read from it.
type cachedArchiveHandle struct {
	handle  archiveHandle
	modTime time.Time
	size    int64
}

// archiveHandleCache is a small bounded cache of open archiveHandles,
// keyed by archive path. It exists to avoid the O(n^2) cost of reopening
// (and, for solid RAR/7z archives, re-decompressing entries 0..k-1) an
// archive on every single entry read during a read-through.
//
// Ownership: archiveHandleCache is NOT safe for concurrent use and takes
// no locks of its own. It is owned exclusively by the single
// asyncLoadWorker goroutine in DefaultImageManager; do not share an
// instance across goroutines or call into it from more than one goroutine
// without adding synchronization.
type archiveHandleCache struct {
	lru *lru.Cache[string, *cachedArchiveHandle]
}

func newArchiveHandleCache() *archiveHandleCache {
	l, err := lru.NewWithEvict[string, *cachedArchiveHandle](maxCachedArchiveHandles,
		func(_ string, cached *cachedArchiveHandle) {
			if cached != nil && cached.handle != nil {
				cached.handle.Close()
			}
		})
	if err != nil {
		// maxCachedArchiveHandles is a positive constant; NewWithEvict only
		// errors when size <= 0, so this is unreachable in practice.
		panic(fmt.Sprintf("archive handle cache: %v", err))
	}
	return &archiveHandleCache{lru: l}
}

// readEntry returns the decompressed bytes of entryName inside the
// archive at archivePath, reusing a cached handle when possible.
func (c *archiveHandleCache) readEntry(archivePath, entryName string) ([]byte, error) {
	info, err := os.Stat(archivePath)
	if err != nil {
		return nil, err
	}

	cached, err := c.handleFor(archivePath, info)
	if err != nil {
		return nil, err
	}

	data, err := cached.handle.ReadEntry(entryName)
	if errors.Is(err, errArchivePositionPassed) {
		// rar only: the requested entry lies before the reader's current
		// position and can't be re-read sequentially. Reopen fresh and
		// retry exactly once.
		c.lru.Remove(archivePath)
		cached, err = c.open(archivePath, info)
		if err != nil {
			return nil, err
		}
		data, err = cached.handle.ReadEntry(entryName)
	}
	return data, err
}

// handleFor returns a cached handle for archivePath, transparently
// reopening it if it's missing or stale (mtime/size no longer match the
// handle that was cached).
func (c *archiveHandleCache) handleFor(archivePath string, info os.FileInfo) (*cachedArchiveHandle, error) {
	if cached, ok := c.lru.Get(archivePath); ok {
		if cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
			return cached, nil
		}
		// The archive on disk changed since we cached it; drop and reopen.
		c.lru.Remove(archivePath)
	}
	return c.open(archivePath, info)
}

func (c *archiveHandleCache) open(archivePath string, info os.FileInfo) (*cachedArchiveHandle, error) {
	handle, err := openArchiveHandle(archivePath)
	if err != nil {
		return nil, err
	}
	cached := &cachedArchiveHandle{handle: handle, modTime: info.ModTime(), size: info.Size()}
	c.lru.Add(archivePath, cached)
	return cached, nil
}

// closeAll closes and purges every cached handle. Call this when the
// owning goroutine (asyncLoadWorker) exits so file handles are released
// promptly instead of waiting for GC.
func (c *archiveHandleCache) closeAll() {
	c.lru.Purge()
}
