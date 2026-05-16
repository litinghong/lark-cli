// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/bus"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	dialRetryInterval = 50 * time.Millisecond
	dialTimeout       = 3 * time.Second
)

var (
	embeddedBusModeEnabled bool
	embeddedBusModeMu      sync.RWMutex
	embeddedBusMu          sync.Mutex
	embeddedBusByApp       = map[string]*embeddedBusHandle{}
)

type embeddedBusHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// SetEmbeddedBusMode toggles in-process event bus startup for library callers.
// Returns a restore function that puts the previous value back.
func SetEmbeddedBusMode(enabled bool) func() {
	embeddedBusModeMu.Lock()
	prev := embeddedBusModeEnabled
	embeddedBusModeEnabled = enabled
	embeddedBusModeMu.Unlock()
	return func() {
		embeddedBusModeMu.Lock()
		embeddedBusModeEnabled = prev
		embeddedBusModeMu.Unlock()
	}
}

func isEmbeddedBusModeEnabled() bool {
	embeddedBusModeMu.RLock()
	defer embeddedBusModeMu.RUnlock()
	return embeddedBusModeEnabled
}

// EnsureBus dials the bus daemon for appID, forking a new one if none is running.
// apiClient nil skips remote-connection probe. Local-bus hits skip remote check (see `event status`).
func EnsureBus(ctx context.Context, tr transport.IPC, appID, appSecret, profileName, domain string, apiClient APIClient, errOut io.Writer) (net.Conn, error) {
	if errOut == nil {
		errOut = os.Stderr //nolint:forbidigo // library-caller fallback
	}
	addr := tr.Address(appID)

	if conn, err := probeAndDialBus(tr, addr); err == nil {
		return conn, nil
	}
	fmt.Fprintf(errOut, "[event] local bus not found; checking remote connections...\n")

	if apiClient != nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		count, checkErr := CheckRemoteConnections(ctx, apiClient)
		if checkErr != nil {
			fmt.Fprintf(errOut, "[event] remote connection check failed: %v (proceeding to start local bus)\n", checkErr)
		} else {
			fmt.Fprintf(errOut, "[event] remote connection check: online_instance_cnt=%d\n", count)
			if count > 0 {
				return nil, fmt.Errorf("another event bus is already connected to this app "+
					"(%d active connection(s) detected via API).\n"+
					"Only one bus should run globally to avoid duplicate event delivery.\n"+
					"Use 'lark-cli event status' to check, or 'lark-cli event stop' on the other machine first", count)
			}
		}
	} else {
		fmt.Fprintf(errOut, "[event] no API client supplied; skipping remote connection check\n")
	}

	// ErrHeld = another consume is forking; let dial retry catch its bus.
	pid, forkErr := forkBus(tr, appID, appSecret, profileName, domain)
	if forkErr != nil && !errors.Is(forkErr, lockfile.ErrHeld) {
		eventsRoot := filepath.Join(core.GetConfigDir(), "events")
		return nil, fmt.Errorf("failed to start event bus daemon: %w\n"+
			"Check: disk space, permissions on %s, and 'lark-cli doctor'", forkErr, eventsRoot)
	}
	if pid > 0 {
		announceForkedBus(errOut, pid)
	}

	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(dialRetryInterval):
		}
		if conn, err := tr.Dial(addr); err == nil {
			return conn, nil
		}
	}

	logPath := filepath.Join(core.GetConfigDir(), "events", event.SanitizeAppID(appID), "bus.log")
	fmt.Fprintln(errOut, "[event] event bus exited unexpectedly.")
	fmt.Fprintln(errOut, "[event] please check app credentials (lark-cli config show) and retry.")
	fmt.Fprintf(errOut, "[event] logs: %s\n", logPath)
	return nil, fmt.Errorf("failed to connect to event bus within %v (app=%s)", dialTimeout, appID)
}

// probeAndDialBus distinguishes a healthy bus from a mid-shutdown listener via StatusQuery first.
func probeAndDialBus(tr transport.IPC, addr string) (net.Conn, error) {
	probe, err := tr.Dial(addr)
	if err != nil {
		return nil, err
	}
	probe.SetDeadline(time.Now().Add(2 * time.Second))
	if err := protocol.Encode(probe, protocol.NewStatusQuery()); err != nil {
		probe.Close()
		return nil, fmt.Errorf("bus probe: encode: %w", err)
	}
	br := bufio.NewReader(probe)
	line, err := protocol.ReadFrame(br)
	probe.Close()
	if err != nil {
		return nil, fmt.Errorf("bus probe: read status: %w", err)
	}
	msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		return nil, fmt.Errorf("bus probe: decode status: %w", err)
	}
	if _, ok := msg.(*protocol.StatusResponse); !ok {
		return nil, fmt.Errorf("bus probe: expected StatusResponse, got %T", msg)
	}

	return tr.Dial(addr)
}

// forkBus holds bus.fork.lock until the spawned daemon is dial-able, so concurrent callers can't race past the empty-socket gap and fork independent buses.
func forkBus(tr transport.IPC, appID, appSecret, profileName, domain string) (int, error) {
	lockPath := filepath.Join(core.GetConfigDir(), "events", event.SanitizeAppID(appID), "bus.fork.lock")
	if err := vfs.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return 0, err
	}

	lock := lockfile.New(lockPath)
	if err := lock.TryLock(); err != nil {
		return 0, err
	}
	defer lock.Unlock()

	if isEmbeddedBusModeEnabled() {
		return ensureEmbeddedBus(tr, appID, appSecret, domain)
	}

	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}

	args := buildForkArgs(profileName, domain)
	cmd := exec.Command(exe, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	applyDetachAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return 0, err
	}

	addr := tr.Address(appID)
	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		if conn, dialErr := tr.Dial(addr); dialErr == nil {
			conn.Close()
			return cmd.Process.Pid, nil
		}
		time.Sleep(dialRetryInterval)
	}
	return cmd.Process.Pid, fmt.Errorf("bus did not become ready within %v", dialTimeout)
}

func ensureEmbeddedBus(tr transport.IPC, appID, appSecret, domain string) (int, error) {
	addr := tr.Address(appID)
	embeddedBusMu.Lock()
	defer embeddedBusMu.Unlock()

	if h, ok := embeddedBusByApp[appID]; ok {
		select {
		case <-h.done:
			delete(embeddedBusByApp, appID)
		default:
			if conn, err := tr.Dial(addr); err == nil {
				conn.Close()
				return os.Getpid(), nil
			}
		}
	}

	eventsDir := filepath.Join(core.GetConfigDir(), "events", event.SanitizeAppID(appID))
	logger, err := bus.SetupBusLogger(eventsDir)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	embeddedBusByApp[appID] = &embeddedBusHandle{cancel: cancel, done: done}

	go func() {
		defer close(done)
		b := bus.NewBus(appID, appSecret, domain, tr, logger)
		_ = b.Run(ctx)
		embeddedBusMu.Lock()
		delete(embeddedBusByApp, appID)
		embeddedBusMu.Unlock()
	}()

	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		if conn, dialErr := tr.Dial(addr); dialErr == nil {
			conn.Close()
			return os.Getpid(), nil
		}
		time.Sleep(dialRetryInterval)
	}
	cancel()
	return 0, fmt.Errorf("embedded bus did not become ready within %v", dialTimeout)
}

func buildForkArgs(profileName, domain string) []string {
	args := []string{"event", "_bus", "--profile", profileName}
	if domain != "" {
		args = append(args, "--domain", domain)
	}
	return args
}

// announceForkedBus: "auto-exits 30s" must track bus.idleTimeout.
func announceForkedBus(w io.Writer, pid int) {
	fmt.Fprintf(w, "[event] started bus daemon pid=%d (auto-exits 30s after last consumer)\n", pid)
}
