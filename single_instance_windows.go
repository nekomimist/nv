//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

type singleInstanceListener interface {
	Accept() (io.ReadWriteCloser, error)
	Close() error
}

type windowsSingleInstanceListener struct {
	name    string
	mu      sync.Mutex
	pending windows.Handle
	closed  bool
}

func listenSingleInstance(endpoint string) (singleInstanceListener, error) {
	listener := &windowsSingleInstanceListener{name: windowsPipeName(endpoint)}

	handle, err := listener.newPipe(true)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return nil, errSingleInstanceEndpointInUse
		}
		return nil, err
	}

	listener.pending = handle
	return listener, nil
}

func dialSingleInstance(endpoint string, timeout time.Duration) (io.ReadWriteCloser, error) {
	pipeName := windowsPipeName(endpoint)
	path, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	for {
		handle, err := windows.CreateFile(
			path,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0,
			nil,
			windows.OPEN_EXISTING,
			0,
			0,
		)
		if err == nil {
			return os.NewFile(uintptr(handle), pipeName), nil
		}

		if !errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return nil, err
		}

		if time.Until(deadline) <= 0 {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func isSingleInstanceListenerClosed(err error) bool {
	return errors.Is(err, os.ErrClosed)
}

func (l *windowsSingleInstanceListener) Accept() (io.ReadWriteCloser, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, os.ErrClosed
	}

	handle := l.pending
	l.pending = 0
	l.mu.Unlock()

	if handle == 0 {
		var err error
		handle, err = l.newPipe(false)
		if err != nil {
			return nil, err
		}
	}

	if err := windows.ConnectNamedPipe(handle, nil); err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
		_ = windows.CloseHandle(handle)
		l.mu.Lock()
		closed := l.closed
		l.mu.Unlock()
		if closed {
			return nil, os.ErrClosed
		}
		return nil, err
	}

	return os.NewFile(uintptr(handle), l.name), nil
}

func (l *windowsSingleInstanceListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.closed = true
	if l.pending != 0 {
		_ = windows.CloseHandle(l.pending)
		l.pending = 0
	}
	return nil
}

func (l *windowsSingleInstanceListener) newPipe(firstInstance bool) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(l.name)
	if err != nil {
		return 0, err
	}

	openMode := uint32(windows.PIPE_ACCESS_DUPLEX)
	if firstInstance {
		openMode |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}

	return windows.CreateNamedPipe(
		name,
		openMode,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES,
		4096,
		4096,
		0,
		nil,
	)
}

func windowsPipeName(endpoint string) string {
	return `\\.\pipe\` + endpoint
}
