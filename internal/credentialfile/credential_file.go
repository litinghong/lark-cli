// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credentialfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const FileName = ".lark-cli-credentials.json"

// Record is the shared credential schema used by both:
// 1) --user-credential-json
// 2) executable-side credential file
type Record struct {
	AppID             string `json:"app_id"`
	AppSecret         string `json:"app_secret"`
	Brand             string `json:"brand"`
	DefaultAs         string `json:"default_as,omitempty"`
	UserOpenID        string `json:"user_open_id,omitempty"`
	UserName          string `json:"user_name,omitempty"`
	UserAccessToken   string `json:"user_access_token,omitempty"`
	RefreshToken      string `json:"refresh_token,omitempty"`
	ExpiresAt         int64  `json:"expires_at,omitempty"`
	RefreshExpiresAt  int64  `json:"refresh_expires_at,omitempty"`
	Scope             string `json:"scope,omitempty"`
	GrantedAt         int64  `json:"granted_at,omitempty"`
	TenantAccessToken string `json:"tenant_access_token,omitempty"`
}

func (r *Record) normalize() {
	if r.Brand == "" {
		r.Brand = string(core.BrandFeishu)
	}
}

// Validate checks the record for required/allowed values.
func (r *Record) Validate() error {
	if r.AppID == "" {
		return fmt.Errorf("missing app_id")
	}
	switch core.LarkBrand(r.Brand) {
	case core.BrandFeishu, core.BrandLark:
	default:
		return fmt.Errorf("invalid brand %q (want feishu or lark)", r.Brand)
	}
	switch r.DefaultAs {
	case "", string(core.AsUser), string(core.AsBot), string(core.AsAuto):
	default:
		return fmt.Errorf("invalid default_as %q (want user, bot, or auto)", r.DefaultAs)
	}
	if r.UserAccessToken == "" && r.TenantAccessToken == "" && r.AppSecret == "" {
		return fmt.Errorf("missing credentials: provide at least one of user_access_token, tenant_access_token, or app_secret")
	}
	return nil
}

func decodeRecord(data []byte) (*Record, error) {
	var rec Record
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rec); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing JSON content")
	}
	rec.normalize()
	if err := rec.Validate(); err != nil {
		return nil, err
	}
	return &rec, nil
}

// ParseInlineJSON parses and validates an inline credential JSON payload.
func ParseInlineJSON(raw string) (*Record, error) {
	rec, err := decodeRecord([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid --user-credential-json: %w", err)
	}
	return rec, nil
}

// ResolvePrimaryPath returns the credential file path next to the executable.
func ResolvePrimaryPath() (string, error) {
	exe, err := vfs.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), FileName), nil
}

// ResolveFallbackPath returns the fallback path under config dir.
func ResolveFallbackPath() string {
	return filepath.Join(core.GetConfigDir(), FileName)
}

// LoadFromPreferredPaths loads record from primary path first, then fallback path.
// Returns (nil, "", nil) when neither file exists.
func LoadFromPreferredPaths() (*Record, string, error) {
	primary, err := ResolvePrimaryPath()
	if err == nil {
		rec, err := LoadFromPath(primary)
		if err == nil && rec != nil {
			return rec, primary, nil
		}
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, "", fmt.Errorf("read credential file %s: %w", primary, err)
			}
		}
	}

	fallback := ResolveFallbackPath()
	rec, err := LoadFromPath(fallback)
	if err == nil && rec != nil {
		return rec, fallback, nil
	}
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("read credential file %s: %w", fallback, err)
		}
	}
	return nil, "", nil
}

// LoadFromPath loads one credential record from path.
func LoadFromPath(path string) (*Record, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rec, err := decodeRecord(data)
	if err != nil {
		return nil, fmt.Errorf("invalid credential JSON: %w", err)
	}
	return rec, nil
}

// SaveToPreferredPaths writes to primary path first, then fallback path.
// If primary fails and fallback succeeds, returns fallback path with usedFallback=true.
func SaveToPreferredPaths(rec *Record) (savedPath string, usedFallback bool, err error) {
	if rec == nil {
		return "", false, fmt.Errorf("record is nil")
	}
	rec.normalize()
	if err := rec.Validate(); err != nil {
		return "", false, err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", false, err
	}
	data = append(data, '\n')

	primary, primaryPathErr := ResolvePrimaryPath()
	var primaryWriteErr error
	if primaryPathErr == nil {
		if err := saveToPath(primary, data); err == nil {
			return primary, false, nil
		} else {
			primaryWriteErr = err
		}
	} else {
		primaryWriteErr = fmt.Errorf("resolve executable path: %w", primaryPathErr)
	}
	fallback := ResolveFallbackPath()
	err2 := saveToPath(fallback, data)
	if err2 == nil {
		return fallback, true, nil
	}
	return "", false, fmt.Errorf("write credential file failed: primary(%s): %v; fallback(%s): %v", primary, primaryWriteErr, fallback, err2)
}

func saveToPath(path string, data []byte) error {
	if err := vfs.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return validate.AtomicWrite(path, data, 0600)
}
