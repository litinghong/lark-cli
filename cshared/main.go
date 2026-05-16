// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/larksuite/cli/cmd"
	_ "github.com/larksuite/cli/extension/credential/env" // activate env credential provider
	"github.com/larksuite/cli/internal/envvars"
)

type invokeRequest struct {
	Args                   []string           `json:"args"`
	Stdin                  string             `json:"stdin,omitempty"`
	StdinB64               string             `json:"stdin_b64,omitempty"`
	EnvOverrides           map[string]*string `json:"env_overrides,omitempty"`
	TimeoutMs              int                `json:"timeout_ms,omitempty"`
	EnableEmbeddedEventBus *bool              `json:"enable_embedded_event_bus,omitempty"`
}

type invokeResponse struct {
	OK        bool   `json:"ok"`
	ExitCode  int    `json:"exit_code"`
	StdoutB64 string `json:"stdout_b64,omitempty"`
	StderrB64 string `json:"stderr_b64,omitempty"`
	Error     string `json:"error,omitempty"`
}

type startSessionResponse struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

type pollRequest struct {
	SessionID string `json:"session_id"`
	MaxBytes  int    `json:"max_bytes,omitempty"`
}

type pollResponse struct {
	OK        bool   `json:"ok"`
	Done      bool   `json:"done"`
	ExitCode  int    `json:"exit_code,omitempty"`
	StdoutB64 string `json:"stdout_b64,omitempty"`
	StderrB64 string `json:"stderr_b64,omitempty"`
	Error     string `json:"error,omitempty"`
}

type cancelRequest struct {
	SessionID string `json:"session_id"`
}

type genericResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type streamSession struct {
	id     string
	cancel context.CancelFunc

	mu       sync.Mutex
	stdout   []byte
	stderr   []byte
	readOut  int
	readErr  int
	done     bool
	exitCode int
	err      string
}

type sessionWriter struct {
	session *streamSession
	stderr  bool
}

func (w *sessionWriter) Write(p []byte) (int, error) {
	w.session.mu.Lock()
	defer w.session.mu.Unlock()
	if w.stderr {
		w.session.stderr = append(w.session.stderr, p...)
	} else {
		w.session.stdout = append(w.session.stdout, p...)
	}
	return len(p), nil
}

var (
	sessionMu sync.Mutex
	sessions  = map[string]*streamSession{}
)

func main() {}

//export LarkCLIInvokeJSON
func LarkCLIInvokeJSON(requestJSON *C.char) *C.char {
	payload := cStringToBytes(requestJSON)
	resp := handleInvoke(payload)
	return C.CString(resp)
}

//export LarkCLIStartSessionJSON
func LarkCLIStartSessionJSON(requestJSON *C.char) *C.char {
	payload := cStringToBytes(requestJSON)
	resp := handleStartSession(payload)
	return C.CString(resp)
}

//export LarkCLIPollSessionJSON
func LarkCLIPollSessionJSON(requestJSON *C.char) *C.char {
	payload := cStringToBytes(requestJSON)
	resp := handlePollSession(payload)
	return C.CString(resp)
}

//export LarkCLICancelSessionJSON
func LarkCLICancelSessionJSON(requestJSON *C.char) *C.char {
	payload := cStringToBytes(requestJSON)
	resp := handleCancelSession(payload)
	return C.CString(resp)
}

//export LarkCLIFreeSessionJSON
func LarkCLIFreeSessionJSON(requestJSON *C.char) *C.char {
	var req cancelRequest
	if err := json.Unmarshal(cStringToBytes(requestJSON), &req); err != nil {
		return C.CString(mustJSON(genericResponse{OK: false, Error: "invalid request JSON: " + err.Error()}))
	}
	if req.SessionID == "" {
		return C.CString(mustJSON(genericResponse{OK: false, Error: "session_id is required"}))
	}
	sessionMu.Lock()
	delete(sessions, req.SessionID)
	sessionMu.Unlock()
	return C.CString(mustJSON(genericResponse{OK: true}))
}

//export LarkCLICommandCatalogJSON
func LarkCLICommandCatalogJSON() *C.char {
	catalog := cmd.BuildCommandCatalog(context.Background())
	return C.CString(mustJSON(catalog))
}

//export LarkCLIFreeCString
func LarkCLIFreeCString(s *C.char) {
	if s == nil {
		return
	}
	C.free(unsafe.Pointer(s))
}

func handleInvoke(payload []byte) string {
	if err := checkUnsupportedAuthsidecar(); err != nil {
		return mustJSON(invokeResponse{OK: false, ExitCode: 1, Error: err.Error()})
	}
	var req invokeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return mustJSON(invokeResponse{OK: false, ExitCode: 1, Error: "invalid request JSON: " + err.Error()})
	}
	if len(req.Args) == 0 {
		return mustJSON(invokeResponse{OK: false, ExitCode: 1, Error: "args is required"})
	}

	stdin, err := decodeStdin(req.Stdin, req.StdinB64)
	if err != nil {
		return mustJSON(invokeResponse{OK: false, ExitCode: 1, Error: err.Error()})
	}

	ctx := context.Background()
	if req.TimeoutMs > 0 {
		timeout := time.Duration(req.TimeoutMs) * time.Millisecond
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	enableBus := true
	if req.EnableEmbeddedEventBus != nil {
		enableBus = *req.EnableEmbeddedEventBus
	}

	result := cmd.ExecuteEmbedded(ctx, req.Args, cmd.EmbeddedExecOptions{
		Stdin:                  stdin,
		EnvOverrides:           req.EnvOverrides,
		EnableEmbeddedEventBus: enableBus,
	})

	return mustJSON(invokeResponse{
		OK:        true,
		ExitCode:  result.ExitCode,
		StdoutB64: base64.StdEncoding.EncodeToString(result.Stdout),
		StderrB64: base64.StdEncoding.EncodeToString(result.Stderr),
	})
}

func handleStartSession(payload []byte) string {
	if err := checkUnsupportedAuthsidecar(); err != nil {
		return mustJSON(startSessionResponse{OK: false, Error: err.Error()})
	}
	var req invokeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return mustJSON(startSessionResponse{OK: false, Error: "invalid request JSON: " + err.Error()})
	}
	if len(req.Args) == 0 {
		return mustJSON(startSessionResponse{OK: false, Error: "args is required"})
	}
	stdin, err := decodeStdin(req.Stdin, req.StdinB64)
	if err != nil {
		return mustJSON(startSessionResponse{OK: false, Error: err.Error()})
	}

	enableBus := true
	if req.EnableEmbeddedEventBus != nil {
		enableBus = *req.EnableEmbeddedEventBus
	}

	baseCtx := context.Background()
	var timeoutCancel context.CancelFunc
	if req.TimeoutMs > 0 {
		baseCtx, timeoutCancel = context.WithTimeout(baseCtx, time.Duration(req.TimeoutMs)*time.Millisecond)
	}
	runCtx, runCancel := context.WithCancel(baseCtx)
	cancel := func() {
		runCancel()
		if timeoutCancel != nil {
			timeoutCancel()
		}
	}

	s := &streamSession{id: uuid.NewString(), cancel: cancel}
	sessionMu.Lock()
	sessions[s.id] = s
	sessionMu.Unlock()

	go func() {
		defer cancel()
		code := cmd.ExecuteEmbeddedWithIO(runCtx, req.Args, cmd.EmbeddedExecOptions{
			Stdin:                  stdin,
			EnvOverrides:           req.EnvOverrides,
			EnableEmbeddedEventBus: enableBus,
		}, &sessionWriter{session: s}, &sessionWriter{session: s, stderr: true})
		s.mu.Lock()
		s.done = true
		s.exitCode = code
		if err := runCtx.Err(); err != nil && err != context.Canceled {
			s.err = err.Error()
		}
		s.mu.Unlock()
	}()

	return mustJSON(startSessionResponse{OK: true, SessionID: s.id})
}

func handlePollSession(payload []byte) string {
	var req pollRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return mustJSON(pollResponse{OK: false, Error: "invalid request JSON: " + err.Error()})
	}
	if req.SessionID == "" {
		return mustJSON(pollResponse{OK: false, Error: "session_id is required"})
	}
	sessionMu.Lock()
	s, ok := sessions[req.SessionID]
	sessionMu.Unlock()
	if !ok {
		return mustJSON(pollResponse{OK: false, Error: "session not found"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := s.stdout[s.readOut:]
	errOut := s.stderr[s.readErr:]
	if req.MaxBytes > 0 {
		if len(out) > req.MaxBytes {
			out = out[:req.MaxBytes]
		}
		if len(errOut) > req.MaxBytes {
			errOut = errOut[:req.MaxBytes]
		}
	}
	s.readOut += len(out)
	s.readErr += len(errOut)

	resp := pollResponse{
		OK:        true,
		Done:      s.done,
		ExitCode:  s.exitCode,
		StdoutB64: base64.StdEncoding.EncodeToString(out),
		StderrB64: base64.StdEncoding.EncodeToString(errOut),
		Error:     s.err,
	}
	return mustJSON(resp)
}

func handleCancelSession(payload []byte) string {
	var req cancelRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return mustJSON(genericResponse{OK: false, Error: "invalid request JSON: " + err.Error()})
	}
	if req.SessionID == "" {
		return mustJSON(genericResponse{OK: false, Error: "session_id is required"})
	}
	sessionMu.Lock()
	s, ok := sessions[req.SessionID]
	sessionMu.Unlock()
	if !ok {
		return mustJSON(genericResponse{OK: false, Error: "session not found"})
	}
	s.cancel()
	return mustJSON(genericResponse{OK: true})
}

func decodeStdin(stdinText, stdinB64 string) ([]byte, error) {
	if stdinB64 != "" {
		data, err := base64.StdEncoding.DecodeString(stdinB64)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return []byte(stdinText), nil
}

func cStringToBytes(v *C.char) []byte {
	if v == nil {
		return nil
	}
	return []byte(C.GoString(v))
}

func checkUnsupportedAuthsidecar() error {
	if os.Getenv(envvars.CliAuthProxy) == "" {
		return nil
	}
	return &unsupportedError{msg: envvars.CliAuthProxy + " is set, but c-shared mode does not support authsidecar in this build"}
}

type unsupportedError struct {
	msg string
}

func (e *unsupportedError) Error() string { return e.msg }

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"marshal response failed"}`
	}
	return string(b)
}
