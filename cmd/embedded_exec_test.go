// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"testing"
)

func TestExecuteEmbedded_SchemaJSON(t *testing.T) {
	res := ExecuteEmbedded(context.Background(), []string{"schema", "calendar", "--format", "json"}, EmbeddedExecOptions{})
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr=%s", res.ExitCode, string(res.Stderr))
	}
	var out map[string]any
	if err := json.Unmarshal(res.Stdout, &out); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, string(res.Stdout))
	}
}

func TestBuildCommandCatalog_ModeClassification(t *testing.T) {
	catalog := BuildCommandCatalog(context.Background())
	foundStream := false
	foundInteractive := false
	for _, c := range catalog.Commands {
		if c.Path == "event consume" {
			foundStream = c.Mode == CommandModeStream
		}
		if c.Path == "auth login" {
			foundInteractive = c.Mode == CommandModeInteractiveLimited
		}
	}
	if !foundStream {
		t.Fatalf("event consume should be classified as %q", CommandModeStream)
	}
	if !foundInteractive {
		t.Fatalf("auth login should be classified as %q", CommandModeInteractiveLimited)
	}
}
