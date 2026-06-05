// SPDX-License-Identifier: Apache-2.0

// Copyright 2026 Cisco Systems, Inc. and their affiliates

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mcp

import (
	"encoding/json"
	"testing"
)

func TestToolResult_Structure(t *testing.T) {
	result := ToolResult("hello world")
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("ToolResult should return a map[string]interface{}")
	}
	content, ok := m["content"].([]map[string]interface{})
	if !ok {
		t.Fatal("content field should be []map[string]interface{}")
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}
	if content[0]["type"] != "text" {
		t.Errorf("expected type %q, got %v", "text", content[0]["type"])
	}
	if content[0]["text"] != "hello world" {
		t.Errorf("expected text %q, got %v", "hello world", content[0]["text"])
	}
}

func TestToolResult_EmptyText(t *testing.T) {
	result := ToolResult("")
	m := result.(map[string]interface{})
	content := m["content"].([]map[string]interface{})
	if content[0]["text"] != "" {
		t.Errorf("expected empty text, got %v", content[0]["text"])
	}
}

func TestToolErrorResult_Structure(t *testing.T) {
	result := ToolErrorResult("something broke")
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("ToolErrorResult should return a map[string]interface{}")
	}
	if m["isError"] != true {
		t.Errorf("expected isError=true, got %v", m["isError"])
	}
	content, ok := m["content"].([]map[string]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("content field missing or empty")
	}
	if content[0]["type"] != "text" {
		t.Errorf("expected type %q, got %v", "text", content[0]["type"])
	}
	want := "Error: something broke"
	if content[0]["text"] != want {
		t.Errorf("expected %q, got %v", want, content[0]["text"])
	}
}

func TestOKResponse_Structure(t *testing.T) {
	id := json.RawMessage(`1`)
	resp := OKResponse(id, "my-result")
	if resp.JSONRPC != "2.0" {
		t.Errorf("expected JSONRPC %q, got %q", "2.0", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Error("expected nil Error on success response")
	}
	if resp.Result != "my-result" {
		t.Errorf("expected result %q, got %v", "my-result", resp.Result)
	}
}

func TestErrorResponse_Structure(t *testing.T) {
	id := json.RawMessage(`42`)
	resp := ErrorResponse(id, -32601, "method not found")
	if resp.JSONRPC != "2.0" {
		t.Errorf("expected JSONRPC %q, got %q", "2.0", resp.JSONRPC)
	}
	if resp.Result != nil {
		t.Errorf("expected nil Result on error response, got %v", resp.Result)
	}
	if resp.Error == nil {
		t.Fatal("expected non-nil Error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code %d, got %d", -32601, resp.Error.Code)
	}
	if resp.Error.Message != "method not found" {
		t.Errorf("expected message %q, got %q", "method not found", resp.Error.Message)
	}
}

func TestMarshalResponse_ValidJSONWithNewline(t *testing.T) {
	resp := OKResponse(json.RawMessage(`1`), map[string]string{"key": "val"})
	b, err := MarshalResponse(resp)
	if err != nil {
		t.Fatalf("MarshalResponse returned error: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("MarshalResponse returned empty bytes")
	}
	if b[len(b)-1] != '\n' {
		t.Error("MarshalResponse should append a trailing newline")
	}
	// The bytes before the newline should be valid JSON.
	var check map[string]interface{}
	if err := json.Unmarshal(b[:len(b)-1], &check); err != nil {
		t.Errorf("MarshalResponse produced invalid JSON: %v", err)
	}
	if check["jsonrpc"] != "2.0" {
		t.Errorf("serialised JSON missing jsonrpc field")
	}
}

func TestMarshalResponse_ErrorResponse(t *testing.T) {
	resp := ErrorResponse(json.RawMessage(`"req-1"`), -32700, "parse error")
	b, err := MarshalResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var check struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &check); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if check.Error.Code != -32700 {
		t.Errorf("expected code %d, got %d", -32700, check.Error.Code)
	}
}
