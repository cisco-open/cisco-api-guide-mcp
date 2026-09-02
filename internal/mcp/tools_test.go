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
	"testing"
)

func TestTools_ReturnsFiveTools(t *testing.T) {
	tools := Tools()
	if len(tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}
}

func TestTools_Names(t *testing.T) {
	want := map[string]bool{
		"search_endpoints":    false,
		"get_endpoint":        false,
		"get_product_guide":   false,
		"search_nac_config":   false,
		"get_nac_config_path": false,
	}
	for _, tool := range Tools() {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		} else {
			t.Errorf("unexpected tool name %q", tool.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected tool %q not found", name)
		}
	}
}

func TestTools_AllHaveDescriptionsAndSchema(t *testing.T) {
	for _, tool := range Tools() {
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil InputSchema", tool.Name)
		}
	}
}

func TestTools_SearchEndpoints_Schema(t *testing.T) {
	var found *ToolDef
	for _, tool := range Tools() {
		if tool.Name == "search_endpoints" {
			copy := tool
			found = &copy
			break
		}
	}
	if found == nil {
		t.Fatal("search_endpoints tool not found")
	}

	schema, ok := found.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("InputSchema is not a map[string]interface{}")
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}
	for _, field := range []string{"query", "product", "release", "limit"} {
		if _, ok := props[field]; !ok {
			t.Errorf("search_endpoints missing property %q", field)
		}
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not []string")
	}
	hasQuery := false
	for _, r := range required {
		if r == "query" {
			hasQuery = true
		}
	}
	if !hasQuery {
		t.Error("query is not listed as required for search_endpoints")
	}
}

func TestTools_GetEndpoint_Schema(t *testing.T) {
	var found *ToolDef
	for _, tool := range Tools() {
		if tool.Name == "get_endpoint" {
			copy := tool
			found = &copy
			break
		}
	}
	if found == nil {
		t.Fatal("get_endpoint tool not found")
	}

	schema, ok := found.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("InputSchema is not a map")
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}
	for _, field := range []string{"product", "method", "path"} {
		if _, ok := props[field]; !ok {
			t.Errorf("get_endpoint missing property %q", field)
		}
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not []string")
	}
	for _, r := range []string{"product", "method", "path"} {
		found := false
		for _, req := range required {
			if req == r {
				found = true
			}
		}
		if !found {
			t.Errorf("%q should be required for get_endpoint", r)
		}
	}
}

func TestTools_GetProductGuide_Schema(t *testing.T) {
	var found *ToolDef
	for _, tool := range Tools() {
		if tool.Name == "get_product_guide" {
			copy := tool
			found = &copy
			break
		}
	}
	if found == nil {
		t.Fatal("get_product_guide tool not found")
	}

	schema, ok := found.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("InputSchema is not a map")
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not []string")
	}
	hasProduct := false
	for _, r := range required {
		if r == "product" {
			hasProduct = true
		}
	}
	if !hasProduct {
		t.Error("product is not required for get_product_guide")
	}
}

func TestTools_ProductEnum_ContainsAllPlatforms(t *testing.T) {
	wantPlatforms := []string{"aci", "ndfc", "intersight", "ucs", "dcnm"}
	restTools := map[string]bool{"search_endpoints": true, "get_endpoint": true}

	for _, tool := range Tools() {
		if !restTools[tool.Name] {
			continue
		}
		schema, ok := tool.InputSchema.(map[string]interface{})
		if !ok {
			continue
		}
		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			continue
		}
		productProp, ok := props["product"].(map[string]interface{})
		if !ok {
			continue
		}
		enum, ok := productProp["enum"].([]string)
		if !ok {
			continue
		}
		enumSet := make(map[string]bool, len(enum))
		for _, e := range enum {
			enumSet[e] = true
		}
		for _, platform := range wantPlatforms {
			if !enumSet[platform] {
				t.Errorf("tool %q: product enum missing %q", tool.Name, platform)
			}
		}
	}
}

func TestTools_GetProductGuide_ProductEnum_ContainsAllPlatforms(t *testing.T) {
	wantPlatforms := []string{"aci", "ndfc", "intersight", "ucs", "dcnm", "nac-aci", "nac-vxlan"}

	var found *ToolDef
	for _, tool := range Tools() {
		if tool.Name == "get_product_guide" {
			copy := tool
			found = &copy
			break
		}
	}
	if found == nil {
		t.Fatal("get_product_guide tool not found")
	}

	schema := found.InputSchema.(map[string]interface{})
	props := schema["properties"].(map[string]interface{})
	productProp := props["product"].(map[string]interface{})
	enum, ok := productProp["enum"].([]string)
	if !ok {
		t.Fatal("enum is not []string")
	}
	enumSet := make(map[string]bool, len(enum))
	for _, e := range enum {
		enumSet[e] = true
	}
	for _, platform := range wantPlatforms {
		if !enumSet[platform] {
			t.Errorf("get_product_guide: product enum missing %q", platform)
		}
	}
}

func TestTools_SearchNACConfig_Schema(t *testing.T) {
	var found *ToolDef
	for _, tool := range Tools() {
		if tool.Name == "search_nac_config" {
			copy := tool
			found = &copy
			break
		}
	}
	if found == nil {
		t.Fatal("search_nac_config tool not found")
	}

	schema, ok := found.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("InputSchema is not a map[string]interface{}")
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}
	for _, field := range []string{"query", "product", "release", "limit"} {
		if _, ok := props[field]; !ok {
			t.Errorf("search_nac_config missing property %q", field)
		}
	}

	productProp, ok := props["product"].(map[string]interface{})
	if !ok {
		t.Fatal("product property is not a map")
	}
	enum, ok := productProp["enum"].([]string)
	if !ok {
		t.Fatal("enum is not []string")
	}
	enumSet := make(map[string]bool, len(enum))
	for _, e := range enum {
		enumSet[e] = true
	}
	for _, platform := range []string{"nac-aci", "nac-vxlan"} {
		if !enumSet[platform] {
			t.Errorf("search_nac_config: product enum missing %q", platform)
		}
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not []string")
	}
	hasQuery := false
	for _, r := range required {
		if r == "query" {
			hasQuery = true
		}
	}
	if !hasQuery {
		t.Error("query is not listed as required for search_nac_config")
	}
}

func TestTools_GetNACConfigPath_Schema(t *testing.T) {
	var found *ToolDef
	for _, tool := range Tools() {
		if tool.Name == "get_nac_config_path" {
			copy := tool
			found = &copy
			break
		}
	}
	if found == nil {
		t.Fatal("get_nac_config_path tool not found")
	}

	schema, ok := found.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("InputSchema is not a map")
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}
	for _, field := range []string{"product", "release", "path"} {
		if _, ok := props[field]; !ok {
			t.Errorf("get_nac_config_path missing property %q", field)
		}
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not []string")
	}
	for _, r := range []string{"product", "path"} {
		found := false
		for _, req := range required {
			if req == r {
				found = true
			}
		}
		if !found {
			t.Errorf("%q should be required for get_nac_config_path", r)
		}
	}
}
