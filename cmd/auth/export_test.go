// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"testing"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credentialfile"
)

func TestAuthExportRun_WithUserToken(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:      "cli_app",
		AppSecret:  "sec_app",
		Brand:      core.BrandFeishu,
		DefaultAs:  core.AsAuto,
		UserOpenId: "ou_user",
		UserName:   "tester",
	})

	tok := &larkauth.StoredUAToken{
		UserOpenId:       "ou_user",
		AppId:            "cli_app",
		AccessToken:      "u_token",
		RefreshToken:     "r_token",
		ExpiresAt:        111,
		RefreshExpiresAt: 222,
		Scope:            "calendar:calendar:readonly",
		GrantedAt:        100,
	}
	if err := larkauth.SetStoredToken(tok); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	t.Cleanup(func() { _ = larkauth.RemoveStoredToken("cli_app", "ou_user") })

	if err := authExportRun(&ExportOptions{Factory: f}); err != nil {
		t.Fatalf("authExportRun() error = %v", err)
	}

	var rec credentialfile.Record
	if err := json.Unmarshal(stdout.Bytes(), &rec); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if rec.AppID != "cli_app" || rec.AppSecret != "sec_app" || rec.Brand != "feishu" {
		t.Fatalf("unexpected app payload: %+v", rec)
	}
	if rec.DefaultAs != "user" {
		t.Fatalf("default_as = %q, want user", rec.DefaultAs)
	}
	if rec.UserOpenID != "ou_user" || rec.UserName != "tester" {
		t.Fatalf("unexpected user payload: %+v", rec)
	}
	if rec.UserAccessToken != "u_token" || rec.RefreshToken != "r_token" {
		t.Fatalf("unexpected token payload: %+v", rec)
	}
}

func TestAuthExportRun_AppOnly(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:     "cli_app",
		AppSecret: "sec_app",
		Brand:     core.BrandLark,
		DefaultAs: core.AsBot,
	})

	if err := authExportRun(&ExportOptions{Factory: f}); err != nil {
		t.Fatalf("authExportRun() error = %v", err)
	}

	var rec credentialfile.Record
	if err := json.Unmarshal(stdout.Bytes(), &rec); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if rec.DefaultAs != "bot" {
		t.Fatalf("default_as = %q, want bot", rec.DefaultAs)
	}
	if rec.UserAccessToken != "" || rec.RefreshToken != "" || rec.UserOpenID != "" {
		t.Fatalf("unexpected user fields in app-only export: %+v", rec)
	}
}
