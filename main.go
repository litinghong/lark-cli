// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// lark-cli — Feishu/Lark CLI tool (Go implementation).
package main

import (
	"os"

	"github.com/larksuite/cli/cmd"

	_ "github.com/larksuite/cli/extension/credential/env"        // activate env credential provider
	_ "github.com/larksuite/cli/extension/credential/exefile"    // activate executable-side credential provider
	_ "github.com/larksuite/cli/extension/credential/inlinejson" // activate inline JSON credential provider
)

func main() {
	os.Exit(cmd.Execute())
}
