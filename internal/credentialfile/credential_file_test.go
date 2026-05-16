// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credentialfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

type executableTestFS struct {
	vfs.OsFs
	exe string
}

func (f executableTestFS) Executable() (string, error) { return f.exe, nil }

func TestParseInlineJSON_Valid(t *testing.T) {
	raw := `{"app_id":"cli_1","brand":"feishu","user_access_token":"u-1"}`
	rec, err := ParseInlineJSON(raw)
	if err != nil {
		t.Fatalf("ParseInlineJSON() error = %v", err)
	}
	if rec.AppID != "cli_1" {
		t.Fatalf("AppID = %q, want %q", rec.AppID, "cli_1")
	}
}

func TestParseInlineJSON_Invalid(t *testing.T) {
	_, err := ParseInlineJSON(`{"app_id":"cli_1","brand":"bad","user_access_token":"u-1"}`)
	if err == nil {
		t.Fatal("ParseInlineJSON() error = nil, want non-nil")
	}
}

func TestLoadFromPreferredPaths_Fallback(t *testing.T) {
	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	tmp := t.TempDir()
	vfs.DefaultFS = executableTestFS{
		OsFs: vfs.OsFs{},
		exe:  filepath.Join(tmp, "bin", "lark-cli"),
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	fallback := filepath.Join(tmp, FileName)
	if err := os.WriteFile(fallback, []byte(`{"app_id":"cli_2","brand":"feishu","user_access_token":"u-2"}`), 0600); err != nil {
		t.Fatalf("WriteFile fallback = %v", err)
	}

	rec, path, err := LoadFromPreferredPaths()
	if err != nil {
		t.Fatalf("LoadFromPreferredPaths() error = %v", err)
	}
	if rec == nil {
		t.Fatal("LoadFromPreferredPaths() record = nil, want non-nil")
	}
	if rec.AppID != "cli_2" {
		t.Fatalf("AppID = %q, want %q", rec.AppID, "cli_2")
	}
	if path != fallback {
		t.Fatalf("path = %q, want %q", path, fallback)
	}
}

func TestSaveToPreferredPaths_Fallback(t *testing.T) {
	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile blocker = %v", err)
	}
	vfs.DefaultFS = executableTestFS{
		OsFs: vfs.OsFs{},
		exe:  filepath.Join(blocker, "lark-cli"),
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	rec := &Record{
		AppID:           "cli_3",
		Brand:           "feishu",
		UserAccessToken: "u-3",
	}
	path, usedFallback, err := SaveToPreferredPaths(rec)
	if err != nil {
		t.Fatalf("SaveToPreferredPaths() error = %v", err)
	}
	if !usedFallback {
		t.Fatal("usedFallback = false, want true")
	}
	if path != filepath.Join(tmp, FileName) {
		t.Fatalf("saved path = %q, want %q", path, filepath.Join(tmp, FileName))
	}
}

func TestSaveToPreferredPaths_BothFail(t *testing.T) {
	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	tmp := t.TempDir()
	primaryBlocker := filepath.Join(tmp, "primary-blocker")
	if err := os.WriteFile(primaryBlocker, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile primary blocker = %v", err)
	}
	fallbackBlocker := filepath.Join(tmp, "fallback-blocker")
	if err := os.WriteFile(fallbackBlocker, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile fallback blocker = %v", err)
	}
	vfs.DefaultFS = executableTestFS{
		OsFs: vfs.OsFs{},
		exe:  filepath.Join(primaryBlocker, "lark-cli"),
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(fallbackBlocker, "blocked"))

	rec := &Record{
		AppID:           "cli_4",
		Brand:           "feishu",
		UserAccessToken: "u-4",
	}
	if _, _, err := SaveToPreferredPaths(rec); err == nil {
		t.Fatal("SaveToPreferredPaths() error = nil, want non-nil")
	}
}
