//go:build !windows

package main

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type singleInstanceListener interface {
	Accept() (io.ReadWriteCloser, error)
	Close() error
}

type unixSingleInstanceListener struct {
	net.Listener
	path string
}

func listenSingleInstance(endpoint string) (singleInstanceListener, error) {
	socketPath, err := unixSingleInstanceSocketPath(endpoint)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("unix", socketPath)
	if err == nil {
		return &unixSingleInstanceListener{Listener: listener, path: socketPath}, nil
	}

	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, err
	}

	if conn, dialErr := net.DialTimeout("unix", socketPath, 200*time.Millisecond); dialErr == nil {
		_ = conn.Close()
		return nil, errSingleInstanceEndpointInUse
	}

	if removeErr := os.Remove(socketPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return nil, removeErr
	}

	listener, err = net.Listen("unix", socketPath)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return nil, errSingleInstanceEndpointInUse
		}
		return nil, err
	}
	return &unixSingleInstanceListener{Listener: listener, path: socketPath}, nil
}

func dialSingleInstance(endpoint string, timeout time.Duration) (io.ReadWriteCloser, error) {
	socketPath, err := unixSingleInstanceSocketPath(endpoint)
	if err != nil {
		return nil, err
	}
	return net.DialTimeout("unix", socketPath, timeout)
}

func isSingleInstanceListenerClosed(err error) bool {
	return errors.Is(err, net.ErrClosed)
}

func (l *unixSingleInstanceListener) Accept() (io.ReadWriteCloser, error) {
	return l.Listener.Accept()
}

func (l *unixSingleInstanceListener) Close() error {
	err := l.Listener.Close()
	removeErr := os.Remove(l.path)
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && err == nil {
		err = removeErr
	}
	return err
}

func unixSingleInstanceSocketPath(endpoint string) (string, error) {
	runtimeDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(runtimeDir, "nekomimist", "nv")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, endpoint+".sock"), nil
}
