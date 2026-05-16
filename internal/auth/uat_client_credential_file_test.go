// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credentialfile"
	"github.com/larksuite/cli/internal/vfs"
)

type executableTestFS struct {
	vfs.OsFs
	exe string
}

func (f executableTestFS) Executable() (string, error) { return f.exe, nil }

func TestPersistRefreshedCredentialFile_PreserveExistingFields(t *testing.T) {
	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	tmp := t.TempDir()
	vfs.DefaultFS = executableTestFS{OsFs: vfs.OsFs{}, exe: filepath.Join(tmp, "bin", "lark-cli")}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	existing := &credentialfile.Record{
		AppID:             "cli_refresh",
		AppSecret:         "sec_old",
		Brand:             "feishu",
		DefaultAs:         "user",
		UserOpenID:        "ou_old",
		UserName:          "old_name",
		UserAccessToken:   "u_old",
		RefreshToken:      "r_old",
		ExpiresAt:         1,
		RefreshExpiresAt:  2,
		Scope:             "old:scope",
		GrantedAt:         3,
		TenantAccessToken: "t_old",
	}
	if _, _, err := credentialfile.SaveToPreferredPaths(existing); err != nil {
		t.Fatalf("SaveToPreferredPaths(existing) error = %v", err)
	}

	tok := &StoredUAToken{
		UserOpenId:       "ou_new",
		AppId:            "cli_refresh",
		AccessToken:      "u_new",
		RefreshToken:     "r_new",
		ExpiresAt:        10,
		RefreshExpiresAt: 20,
		Scope:            "calendar:read",
		GrantedAt:        30,
	}
	opts := UATCallOptions{
		UserOpenId: "ou_new",
		AppId:      "cli_refresh",
		AppSecret:  "",
		Domain:     core.BrandFeishu,
	}
	if err := persistRefreshedCredentialFile(opts, tok); err != nil {
		t.Fatalf("persistRefreshedCredentialFile() error = %v", err)
	}

	updated, _, err := credentialfile.LoadFromPreferredPaths()
	if err != nil {
		t.Fatalf("LoadFromPreferredPaths() error = %v", err)
	}
	if updated == nil {
		t.Fatal("updated credential file is nil")
	}
	if updated.UserAccessToken != "u_new" {
		t.Fatalf("UserAccessToken = %q, want %q", updated.UserAccessToken, "u_new")
	}
	if updated.TenantAccessToken != "t_old" {
		t.Fatalf("TenantAccessToken = %q, want %q", updated.TenantAccessToken, "t_old")
	}
	if updated.UserName != "old_name" {
		t.Fatalf("UserName = %q, want %q", updated.UserName, "old_name")
	}
}

func TestPersistRefreshedCredentialFile_BothFail(t *testing.T) {
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

	err := persistRefreshedCredentialFile(UATCallOptions{
		UserOpenId: "ou_fail",
		AppId:      "cli_fail",
		AppSecret:  "sec_fail",
		Domain:     core.BrandFeishu,
	}, &StoredUAToken{
		UserOpenId:  "ou_fail",
		AppId:       "cli_fail",
		AccessToken: "u_fail",
	})
	if err == nil {
		t.Fatal("persistRefreshedCredentialFile() error = nil, want non-nil")
	}
}
