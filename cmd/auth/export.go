// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credentialfile"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// ExportOptions holds all inputs for auth export.
type ExportOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdAuthExport creates the auth export subcommand.
func NewCmdAuthExport(f *cmdutil.Factory, runF func(*ExportOptions) error) *cobra.Command {
	opts := &ExportOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export current credential payload as JSON",
		Long: `Export current credential payload as JSON.

The output schema is compatible with .lark-cli-credentials.json and can be
stored/managed by the embedding application instead of writing local files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authExportRun(opts)
		},
	}

	return cmd
}

func authExportRun(opts *ExportOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}
	if config == nil {
		return output.Errorf(output.ExitInternal, "internal", "missing config")
	}

	rec := &credentialfile.Record{
		AppID:      config.AppID,
		AppSecret:  config.AppSecret,
		Brand:      string(config.Brand),
		UserOpenID: config.UserOpenId,
		UserName:   config.UserName,
	}

	stored := (*larkauth.StoredUAToken)(nil)
	if config.UserOpenId != "" {
		stored = larkauth.GetStoredToken(config.AppID, config.UserOpenId)
	}
	if stored != nil {
		rec.DefaultAs = string(core.AsUser)
		rec.UserAccessToken = stored.AccessToken
		rec.RefreshToken = stored.RefreshToken
		rec.ExpiresAt = stored.ExpiresAt
		rec.RefreshExpiresAt = stored.RefreshExpiresAt
		rec.Scope = stored.Scope
		rec.GrantedAt = stored.GrantedAt
	} else if config.DefaultAs != "" {
		rec.DefaultAs = string(config.DefaultAs)
	} else if config.UserOpenId != "" {
		rec.DefaultAs = string(core.AsUser)
	} else {
		rec.DefaultAs = string(core.AsBot)
	}

	if err := rec.Validate(); err != nil {
		return output.ErrWithHint(output.ExitValidation, "credential_export",
			fmt.Sprintf("cannot export credentials: %v", err),
			"check app credentials via `lark-cli config show`, then re-run export")
	}

	output.PrintJson(f.IOStreams.Out, rec)
	return nil
}
