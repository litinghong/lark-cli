// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/larksuite/cli/cmd"
	_ "github.com/larksuite/cli/extension/credential/env" // activate env credential provider
)

func main() {
	catalog := cmd.BuildCommandCatalog(context.Background())
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(catalog); err != nil {
		fmt.Fprintf(os.Stderr, "encode command catalog: %v\n", err)
		os.Exit(1)
	}
}
