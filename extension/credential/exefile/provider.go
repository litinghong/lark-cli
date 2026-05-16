// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package exefile

import (
	"context"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/credentialfile"
)

// Provider resolves credentials from .lark-cli-credentials.json.
type Provider struct{}

func (p *Provider) Name() string  { return "exe_file" }
func (p *Provider) Priority() int { return 2 }
func (p *Provider) Builtin() bool { return true }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	rec, _, err := credentialfile.LoadFromPreferredPaths()
	if err != nil {
		return nil, &credential.BlockError{Provider: p.Name(), Reason: err.Error()}
	}
	if rec == nil {
		return nil, nil
	}
	return toAccount(rec), nil
}

func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	rec, _, err := credentialfile.LoadFromPreferredPaths()
	if err != nil {
		return nil, &credential.BlockError{Provider: p.Name(), Reason: err.Error()}
	}
	if rec == nil {
		return nil, nil
	}
	switch req.Type {
	case credential.TokenTypeUAT:
		if rec.UserAccessToken == "" {
			return nil, nil
		}
		return &credential.Token{Value: rec.UserAccessToken, Scopes: rec.Scope, Source: p.Name()}, nil
	case credential.TokenTypeTAT:
		if rec.TenantAccessToken == "" {
			return nil, nil
		}
		return &credential.Token{Value: rec.TenantAccessToken, Source: p.Name()}, nil
	default:
		return nil, nil
	}
}

func toAccount(rec *credentialfile.Record) *credential.Account {
	acct := &credential.Account{
		AppID:       rec.AppID,
		AppSecret:   rec.AppSecret,
		Brand:       credential.Brand(rec.Brand),
		DefaultAs:   credential.Identity(rec.DefaultAs),
		OpenID:      rec.UserOpenID,
		ProfileName: "",
	}
	if rec.UserAccessToken != "" {
		acct.SupportedIdentities |= credential.SupportsUser
	}
	if rec.TenantAccessToken != "" || rec.AppSecret != credential.NoAppSecret {
		acct.SupportedIdentities |= credential.SupportsBot
	}
	if acct.DefaultAs == "" {
		switch {
		case rec.UserAccessToken != "":
			acct.DefaultAs = credential.IdentityUser
		case rec.TenantAccessToken != "":
			acct.DefaultAs = credential.IdentityBot
		}
	}
	return acct
}

func init() {
	credential.Register(&Provider{})
}
