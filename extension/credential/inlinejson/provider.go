// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package inlinejson

import (
	"context"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/credentialfile"
	"github.com/larksuite/cli/internal/invocation"
)

// Provider resolves credentials from --user-credential-json.
type Provider struct{}

func (p *Provider) Name() string  { return "inline_json" }
func (p *Provider) Priority() int { return 1 }
func (p *Provider) Builtin() bool { return true }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	opts, ok := invocation.FromContext(ctx)
	if !ok || opts.UserCredentialJSON == "" {
		return nil, nil
	}
	rec, err := credentialfile.ParseInlineJSON(opts.UserCredentialJSON)
	if err != nil {
		return nil, &credential.BlockError{Provider: p.Name(), Reason: err.Error()}
	}
	return toAccount(rec), nil
}

func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	opts, ok := invocation.FromContext(ctx)
	if !ok || opts.UserCredentialJSON == "" {
		return nil, nil
	}
	rec, err := credentialfile.ParseInlineJSON(opts.UserCredentialJSON)
	if err != nil {
		return nil, &credential.BlockError{Provider: p.Name(), Reason: err.Error()}
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
