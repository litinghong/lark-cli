// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credentialfile"
	"github.com/larksuite/cli/internal/vfs"
)

type executableTestFS struct {
	vfs.OsFs
	exe string
}

func (f executableTestFS) Executable() (string, error) { return f.exe, nil }

func TestPersistLoginCredentialFile_Fallback(t *testing.T) {
	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := vfs.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile blocker = %v", err)
	}
	vfs.DefaultFS = executableTestFS{OsFs: vfs.OsFs{}, exe: filepath.Join(blocker, "lark-cli")}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	var out bytes.Buffer
	var errOut bytes.Buffer
	f := &cmdutil.Factory{IOStreams: cmdutil.NewIOStreams(strings.NewReader(""), &out, &errOut)}
	cfg := &core.CliConfig{
		AppID:     "cli_login",
		AppSecret: "sec_login",
		Brand:     core.BrandFeishu,
	}
	tok := &larkauth.StoredUAToken{
		UserOpenId:       "ou_login",
		AppId:            "cli_login",
		AccessToken:      "u_login",
		RefreshToken:     "r_login",
		ExpiresAt:        100,
		RefreshExpiresAt: 200,
		Scope:            "calendar:read",
		GrantedAt:        10,
	}

	if err := persistLoginCredentialFile(f, cfg, "ou_login", "user_login", tok); err != nil {
		t.Fatalf("persistLoginCredentialFile() error = %v", err)
	}
	if !strings.Contains(errOut.String(), "fallback path") {
		t.Fatalf("stderr = %q, want fallback warning", errOut.String())
	}
	path := filepath.Join(tmp, credentialfile.FileName)
	if _, err := vfs.Stat(path); err != nil {
		t.Fatalf("expected fallback file at %s: %v", path, err)
	}
}

func TestPersistLoginCredentialFile_BothFail(t *testing.T) {
	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	tmp := t.TempDir()
	primaryBlocker := filepath.Join(tmp, "primary-blocker")
	if err := vfs.WriteFile(primaryBlocker, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile primary blocker = %v", err)
	}
	fallbackBlocker := filepath.Join(tmp, "fallback-blocker")
	if err := vfs.WriteFile(fallbackBlocker, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile fallback blocker = %v", err)
	}
	vfs.DefaultFS = executableTestFS{OsFs: vfs.OsFs{}, exe: filepath.Join(primaryBlocker, "lark-cli")}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(fallbackBlocker, "blocked"))

	f := &cmdutil.Factory{IOStreams: cmdutil.NewIOStreams(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})}
	cfg := &core.CliConfig{AppID: "cli_login", AppSecret: "sec_login", Brand: core.BrandFeishu}
	tok := &larkauth.StoredUAToken{UserOpenId: "ou_login", AppId: "cli_login", AccessToken: "u_login", RefreshToken: "r_login"}
	if err := persistLoginCredentialFile(f, cfg, "ou_login", "user_login", tok); err == nil {
		t.Fatal("persistLoginCredentialFile() error = nil, want non-nil")
	}
}
