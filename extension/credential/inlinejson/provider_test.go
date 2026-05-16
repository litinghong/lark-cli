// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package inlinejson

import (
	"context"
	"errors"
	"testing"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/invocation"
)

func TestProvider_ResolveAccountAndToken(t *testing.T) {
	raw := `{"app_id":"cli_inline","brand":"feishu","default_as":"user","user_open_id":"ou_x","user_access_token":"u_x","refresh_token":"r_x","scope":"calendar:read"}`
	ctx := invocation.WithContext(context.Background(), invocation.Options{UserCredentialJSON: raw})
	p := &Provider{}

	acct, err := p.ResolveAccount(ctx)
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct == nil {
		t.Fatal("ResolveAccount() acct = nil, want non-nil")
	}
	if acct.AppID != "cli_inline" {
		t.Fatalf("AppID = %q, want %q", acct.AppID, "cli_inline")
	}

	tok, err := p.ResolveToken(ctx, credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatalf("ResolveToken(UAT) error = %v", err)
	}
	if tok == nil || tok.Value != "u_x" {
		t.Fatalf("ResolveToken(UAT) = %#v, want token value u_x", tok)
	}
}

func TestProvider_InvalidJSONReturnsBlockError(t *testing.T) {
	ctx := invocation.WithContext(context.Background(), invocation.Options{
		UserCredentialJSON: `{"app_id":"cli_inline","brand":"bad","user_access_token":"u_x"}`,
	})
	_, err := (&Provider{}).ResolveAccount(ctx)
	if err == nil {
		t.Fatal("ResolveAccount() error = nil, want non-nil")
	}
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("error type = %T, want *credential.BlockError", err)
	}
}
