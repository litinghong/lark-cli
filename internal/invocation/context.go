// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package invocation

import "context"

// Options carries invocation-scoped inputs that must be visible from deep
// runtime layers (for example credential providers).
type Options struct {
	Profile            string
	UserCredentialJSON string
}

type ctxKey struct{}

// WithContext attaches invocation options to ctx.
func WithContext(ctx context.Context, opts Options) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKey{}, opts)
}

// FromContext reads invocation options from ctx.
func FromContext(ctx context.Context) (Options, bool) {
	if ctx == nil {
		return Options{}, false
	}
	opts, ok := ctx.Value(ctxKey{}).(Options)
	return opts, ok
}
