// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

package credential

import (
	"testing"

	_ "github.com/larksuite/cli/extension/credential/env"
	_ "github.com/larksuite/cli/extension/credential/exefile"
	_ "github.com/larksuite/cli/extension/credential/inlinejson"
	_ "github.com/larksuite/cli/extension/credential/sidecar"
)

func TestProviderPriority_SidecarBeforeInlineAndFile(t *testing.T) {
	index := map[string]int{}
	for i, p := range Providers() {
		index[p.Name()] = i
	}
	required := []string{"sidecar", "inline_json", "exe_file", "env"}
	for _, name := range required {
		if _, ok := index[name]; !ok {
			t.Fatalf("provider %q is not registered", name)
		}
	}
	if !(index["sidecar"] < index["inline_json"] && index["inline_json"] < index["exe_file"] && index["exe_file"] < index["env"]) {
		t.Fatalf("unexpected provider order: sidecar=%d inline=%d file=%d env=%d",
			index["sidecar"], index["inline_json"], index["exe_file"], index["env"])
	}
}

