// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	CommandModeSync               = "sync"
	CommandModeStream             = "stream"
	CommandModeInteractiveLimited = "interactive-limited"
)

// CommandCatalog is a flat command inventory suitable for SDK generation.
type CommandCatalog struct {
	Commands []CommandCatalogItem `json:"commands"`
}

// CommandCatalogItem describes one command in the tree.
type CommandCatalogItem struct {
	Path  string `json:"path"`
	Use   string `json:"use"`
	Short string `json:"short,omitempty"`

	Hidden bool   `json:"hidden"`
	Leaf   bool   `json:"leaf"`
	Mode   string `json:"mode"`

	Flags []CommandCatalogFlag `json:"flags,omitempty"`
}

// CommandCatalogFlag describes one command flag.
type CommandCatalogFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage,omitempty"`
	Required  bool   `json:"required"`
	Hidden    bool   `json:"hidden"`
}

// BuildCommandCatalog builds a complete command inventory from the assembled root tree.
func BuildCommandCatalog(ctx context.Context) CommandCatalog {
	root := Build(ctx, cmdutil.InvocationContext{}, HideProfile(isSingleAppMode()))
	items := make([]CommandCatalogItem, 0, 512)

	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c != nil && c.Parent() != nil {
			item := CommandCatalogItem{
				Path:   normalizeCommandPath(c.CommandPath()),
				Use:    c.Use,
				Short:  c.Short,
				Hidden: c.Hidden,
				Leaf:   !c.HasAvailableSubCommands(),
				Mode:   classifyCommandMode(c),
				Flags:  collectFlags(c),
			}
			items = append(items, item)
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)

	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
	return CommandCatalog{Commands: items}
}

func normalizeCommandPath(path string) string {
	parts := strings.Fields(path)
	if len(parts) > 0 && parts[0] == "lark-cli" {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}

func classifyCommandMode(c *cobra.Command) string {
	path := normalizeCommandPath(c.CommandPath())
	name := c.Name()
	switch {
	case path == "event consume":
		return CommandModeStream
	case name == "watch" || strings.HasPrefix(name, "+watch"):
		return CommandModeStream
	case path == "auth login":
		return CommandModeInteractiveLimited
	case path == "config init" || path == "config bind":
		return CommandModeInteractiveLimited
	case path == "profile add":
		return CommandModeInteractiveLimited
	default:
		return CommandModeSync
	}
}

func collectFlags(c *cobra.Command) []CommandCatalogFlag {
	flagByName := make(map[string]CommandCatalogFlag, 32)
	add := func(fs *pflag.FlagSet) {
		if fs == nil {
			return
		}
		fs.VisitAll(func(f *pflag.Flag) {
			if f == nil {
				return
			}
			isReq := false
			if f.Annotations != nil {
				_, isReq = f.Annotations[cobra.BashCompOneRequiredFlag]
			}
			flagByName[f.Name] = CommandCatalogFlag{
				Name:      f.Name,
				Shorthand: f.Shorthand,
				Type:      f.Value.Type(),
				Default:   f.DefValue,
				Usage:     f.Usage,
				Required:  isReq,
				Hidden:    f.Hidden,
			}
		})
	}
	add(c.InheritedFlags())
	add(c.LocalFlags())

	out := make([]CommandCatalogFlag, 0, len(flagByName))
	for _, f := range flagByName {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	return out
}
