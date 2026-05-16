// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestHandleInvoke_InvalidJSON(t *testing.T) {
	respText := handleInvoke([]byte(`{`))
	var resp invokeResponse
	if err := json.Unmarshal([]byte(respText), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected ok=false, got %#v", resp)
	}
}

func TestSessionLifecycle_StartPollFree(t *testing.T) {
	startReq := invokeRequest{
		Args: []string{"schema", "--format", "json"},
	}
	rawStart, _ := json.Marshal(startReq)
	startText := handleStartSession(rawStart)
	var startResp startSessionResponse
	if err := json.Unmarshal([]byte(startText), &startResp); err != nil {
		t.Fatalf("unmarshal start response: %v", err)
	}
	if !startResp.OK || startResp.SessionID == "" {
		t.Fatalf("start failed: %s", startText)
	}

	var done bool
	for i := 0; i < 20; i++ {
		pollReq := pollRequest{SessionID: startResp.SessionID}
		rawPoll, _ := json.Marshal(pollReq)
		pollText := handlePollSession(rawPoll)
		var pollResp pollResponse
		if err := json.Unmarshal([]byte(pollText), &pollResp); err != nil {
			t.Fatalf("unmarshal poll response: %v", err)
		}
		if !pollResp.OK {
			t.Fatalf("poll failed: %s", pollText)
		}
		if pollResp.StdoutB64 != "" {
			if _, err := base64.StdEncoding.DecodeString(pollResp.StdoutB64); err != nil {
				t.Fatalf("invalid stdout base64: %v", err)
			}
		}
		if pollResp.Done {
			done = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !done {
		t.Fatal("session did not finish in expected time")
	}

	freeReq := cancelRequest{SessionID: startResp.SessionID}
	rawFree, _ := json.Marshal(freeReq)
	freeText := string(handleCancelSession(rawFree))
	if freeText == "" {
		t.Fatal("cancel response empty")
	}
}
