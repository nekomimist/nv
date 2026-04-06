package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const singleInstanceRequestQueueSize = 8

var errSingleInstanceEndpointInUse = errors.New("single-instance endpoint already in use")

type singleInstanceRequest struct {
	ActivateOnly bool     `json:"activate_only"`
	Args         []string `json:"args,omitempty"`
}

type singleInstanceResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type pendingLaunchRequest struct {
	ActivateOnly bool
	Args         []string
	Paths        []ImagePath
}

type singleInstanceBridge struct {
	requests   chan pendingLaunchRequest
	sortMethod int
	mu         sync.RWMutex
}

func newSingleInstanceBridge(initialSortMethod int) *singleInstanceBridge {
	return &singleInstanceBridge{
		requests:   make(chan pendingLaunchRequest, singleInstanceRequestQueueSize),
		sortMethod: initialSortMethod,
	}
}

func (b *singleInstanceBridge) Requests() <-chan pendingLaunchRequest {
	return b.requests
}

func (b *singleInstanceBridge) SetSortMethod(sortMethod int) {
	b.mu.Lock()
	b.sortMethod = sortMethod
	b.mu.Unlock()
}

func (b *singleInstanceBridge) currentSortMethod() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.sortMethod
}

func (b *singleInstanceBridge) prepareRequest(req singleInstanceRequest) singleInstanceResponse {
	if req.ActivateOnly {
		return b.enqueue(pendingLaunchRequest{ActivateOnly: true})
	}

	if len(req.Args) == 0 {
		return singleInstanceResponse{OK: false, Error: "missing launch arguments"}
	}

	paths, err := collectImages(req.Args, b.currentSortMethod())
	if err != nil {
		return singleInstanceResponse{OK: false, Error: err.Error()}
	}
	if len(paths) == 0 {
		return singleInstanceResponse{OK: false, Error: "no images found"}
	}

	args := append([]string(nil), req.Args...)
	preparedPaths := append([]ImagePath(nil), paths...)
	return b.enqueue(pendingLaunchRequest{
		Args:  args,
		Paths: preparedPaths,
	})
}

func (b *singleInstanceBridge) enqueue(req pendingLaunchRequest) singleInstanceResponse {
	select {
	case b.requests <- req:
		return singleInstanceResponse{OK: true}
	default:
		return singleInstanceResponse{OK: false, Error: "existing instance is busy"}
	}
}

type singleInstanceManager struct {
	endpoint string
	listener singleInstanceListener
}

func newSingleInstanceManager(configPath string) (*singleInstanceManager, error) {
	normalizedConfigPath, err := normalizeConfigPath(configPath)
	if err != nil {
		return nil, err
	}

	return &singleInstanceManager{
		endpoint: singleInstanceEndpoint(normalizedConfigPath),
	}, nil
}

func (m *singleInstanceManager) AcquireOrForward(args []string, bridge *singleInstanceBridge) (bool, error) {
	listener, err := listenSingleInstance(m.endpoint)
	if err == nil {
		m.listener = listener
		go m.serve(bridge)
		return true, nil
	}

	if !errors.Is(err, errSingleInstanceEndpointInUse) {
		return false, err
	}

	req, err := newForwardRequest(args)
	if err != nil {
		return false, err
	}

	resp, err := m.send(req)
	if err != nil {
		return false, err
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "request rejected by existing instance"
		}
		return false, errors.New(resp.Error)
	}
	return false, nil
}

func (m *singleInstanceManager) Close() error {
	if m.listener == nil {
		return nil
	}
	return m.listener.Close()
}

func (m *singleInstanceManager) serve(bridge *singleInstanceBridge) {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			if isSingleInstanceListenerClosed(err) {
				return
			}
			warnKV("single_instance", "accept_failed", "error", err)
			continue
		}
		m.handleConn(conn, bridge)
	}
}

func (m *singleInstanceManager) handleConn(conn io.ReadWriteCloser, bridge *singleInstanceBridge) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	decoder := json.NewDecoder(reader)
	encoder := json.NewEncoder(writer)

	var req singleInstanceRequest
	if err := decoder.Decode(&req); err != nil {
		_ = encoder.Encode(singleInstanceResponse{OK: false, Error: "invalid request"})
		_ = writer.Flush()
		return
	}

	resp := bridge.prepareRequest(req)
	if err := encoder.Encode(resp); err != nil {
		warnKV("single_instance", "encode_response_failed", "error", err)
		return
	}
	if err := writer.Flush(); err != nil {
		warnKV("single_instance", "flush_response_failed", "error", err)
	}
}

func (m *singleInstanceManager) send(req singleInstanceRequest) (singleInstanceResponse, error) {
	conn, err := dialSingleInstance(m.endpoint, 3*time.Second)
	if err != nil {
		return singleInstanceResponse{}, err
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	encoder := json.NewEncoder(writer)
	decoder := json.NewDecoder(reader)

	if err := encoder.Encode(req); err != nil {
		return singleInstanceResponse{}, err
	}
	if err := writer.Flush(); err != nil {
		return singleInstanceResponse{}, err
	}

	var resp singleInstanceResponse
	if err := decoder.Decode(&resp); err != nil {
		return singleInstanceResponse{}, err
	}
	return resp, nil
}

func newForwardRequest(args []string) (singleInstanceRequest, error) {
	if len(args) == 0 {
		return singleInstanceRequest{ActivateOnly: true}, nil
	}

	normalizedArgs := make([]string, 0, len(args))
	for _, arg := range args {
		absPath, err := filepath.Abs(arg)
		if err != nil {
			return singleInstanceRequest{}, fmt.Errorf("normalize launch arg %q: %w", arg, err)
		}
		normalizedArgs = append(normalizedArgs, filepath.Clean(absPath))
	}

	return singleInstanceRequest{
		Args: normalizedArgs,
	}, nil
}

func normalizeConfigPath(configPath string) (string, error) {
	if configPath == "" {
		configPath = getConfigPath()
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", fmt.Errorf("resolve config path %q: %w", configPath, err)
	}
	return filepath.Clean(absPath), nil
}

func singleInstanceEndpoint(normalizedConfigPath string) string {
	sum := sha256.Sum256([]byte(normalizedConfigPath))
	return "nv-" + hex.EncodeToString(sum[:8])
}

func bestEffortActivateWindow() {
	if ebiten.IsWindowMinimized() {
		ebiten.RestoreWindow()
	}
	ebiten.RequestAttention()
}

func initSingleInstanceBridge(bridge *singleInstanceBridge, g *Game) {
	g.instanceBridge = bridge
	g.externalOpenRequests = bridge.Requests()
	bridge.SetSortMethod(g.config.SortMethod)
}
