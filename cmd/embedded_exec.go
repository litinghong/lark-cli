// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	eventconsume "github.com/larksuite/cli/internal/event/consume"
)

var embeddedExecMu sync.Mutex

// EmbeddedExecOptions configures one embedded CLI invocation.
type EmbeddedExecOptions struct {
	// Stdin is fed as the process stdin stream for this invocation.
	Stdin []byte
	// EnvOverrides applies temporary process env overrides during this invocation.
	// nil value means unset this variable.
	EnvOverrides map[string]*string
	// EnableEmbeddedEventBus forces event consume to start bus in-process.
	EnableEmbeddedEventBus bool
}

// EmbeddedExecResult carries captured outputs and exit code.
type EmbeddedExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// ExecuteEmbedded runs lark-cli command logic in-process with isolated IO capture.
// This path is serialized by a global mutex because the CLI has process-global
// state (for example output.PendingNotice and environment variables).
func ExecuteEmbedded(ctx context.Context, args []string, opts EmbeddedExecOptions) EmbeddedExecResult {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := ExecuteEmbeddedWithIO(ctx, args, opts, stdout, stderr)
	return EmbeddedExecResult{
		ExitCode: code,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
	}
}

// ExecuteEmbeddedWithIO runs one in-process invocation and writes outputs to
// the provided writers. Callers that need stream-style capture should pass
// incremental writers and inspect them concurrently.
func ExecuteEmbeddedWithIO(ctx context.Context, args []string, opts EmbeddedExecOptions, out, errOut io.Writer) int {
	embeddedExecMu.Lock()
	defer embeddedExecMu.Unlock()

	stdin := bytes.NewReader(opts.Stdin)

	defer applyEnvOverrides(opts.EnvOverrides)()
	restoreBusMode := eventconsume.SetEmbeddedBusMode(opts.EnableEmbeddedEventBus)
	defer restoreBusMode()

	inv, err := BootstrapInvocationContext(args)
	if err != nil {
		fmt.Fprintln(errOut, "Error:", err)
		return 1
	}
	configureFlagCompletions(args)

	f, rootCmd := buildInternal(
		ctx, inv,
		WithIO(stdin, out, errOut),
		HideProfile(isSingleAppMode()),
	)
	rootCmd.SetArgs(args)

	if !isCompletionCommand(args) {
		setupNotices()
	}

	if err := rootCmd.Execute(); err != nil {
		return handleRootError(f, err)
	}
	return 0
}

func applyEnvOverrides(overrides map[string]*string) func() {
	if len(overrides) == 0 {
		return func() {}
	}
	type prevEntry struct {
		value string
		found bool
	}
	prev := make(map[string]prevEntry, len(overrides))
	for k := range overrides {
		v, ok := os.LookupEnv(k)
		prev[k] = prevEntry{value: v, found: ok}
	}

	for k, v := range overrides {
		if v == nil {
			_ = os.Unsetenv(k)
			continue
		}
		_ = os.Setenv(k, *v)
	}

	return func() {
		for k, p := range prev {
			if !p.found {
				_ = os.Unsetenv(k)
				continue
			}
			_ = os.Setenv(k, p.value)
		}
	}
}
