// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package exefile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/credentialfile"
	"github.com/larksuite/cli/internal/vfs"
)

type executableTestFS struct {
	vfs.OsFs
	exe string
}

func (f executableTestFS) Executable() (string, error) { return f.exe, nil }

func TestProvider_ResolveFromFallbackFile(t *testing.T) {
	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	tmp := t.TempDir()
	vfs.DefaultFS = executableTestFS{
		OsFs: vfs.OsFs{},
		exe:  filepath.Join(tmp, "bin", "lark-cli"),
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	path := filepath.Join(tmp, credentialfile.FileName)
	if err := os.WriteFile(path, []byte(`{"app_id":"cli_file","brand":"feishu","user_access_token":"u_file","scope":"calendar:read"}`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	p := &Provider{}
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct == nil {
		t.Fatal("ResolveAccount() acct = nil, want non-nil")
	}
	if acct.AppID != "cli_file" {
		t.Fatalf("AppID = %q, want %q", acct.AppID, "cli_file")
	}

	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatalf("ResolveToken(UAT) error = %v", err)
	}
	if tok == nil || tok.Value != "u_file" {
		t.Fatalf("ResolveToken(UAT) = %#v, want token value u_file", tok)
	}
}

func TestProvider_InvalidPrimaryFileReturnsBlockError(t *testing.T) {
	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	tmp := t.TempDir()
	exe := filepath.Join(tmp, "bin", "lark-cli")
	if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(exe), credentialfile.FileName), []byte(`{"app_id":"bad"`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	vfs.DefaultFS = executableTestFS{OsFs: vfs.OsFs{}, exe: exe}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	_, err := (&Provider{}).ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("ResolveAccount() error = nil, want non-nil")
	}
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("error type = %T, want *credential.BlockError", err)
	}
}
