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

package search

import (
	"testing"
)

func TestBuildFTSQuery_EmptyInput(t *testing.T) {
	got := BuildFTSQuery("", nil)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestBuildFTSQuery_SingleToken_NoSynonym(t *testing.T) {
	got := BuildFTSQuery("fvTenant", map[string]string{})
	// Input is lowercased before lookup and output.
	if got != "fvtenant" {
		t.Errorf("expected %q, got %q", "fvtenant", got)
	}
}

func TestBuildFTSQuery_SingleToken_WithSynonym(t *testing.T) {
	syns := map[string]string{"vrf": "context fvCtx"}
	got := BuildFTSQuery("VRF", syns)
	want := "(vrf OR context OR fvCtx)"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuildFTSQuery_MultipleTokens_Mixed(t *testing.T) {
	syns := map[string]string{"vrf": "context fvCtx"}
	// "list" and "tenant" have no synonyms; "vrf" is expanded.
	got := BuildFTSQuery("list tenant vrf", syns)
	want := "list tenant (vrf OR context OR fvCtx)"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuildFTSQuery_AllTokensExpanded(t *testing.T) {
	syns := map[string]string{
		"tenant": "fvTenant",
		"vrf":    "fvCtx context",
	}
	got := BuildFTSQuery("tenant vrf", syns)
	want := "(tenant OR fvTenant) (vrf OR fvCtx OR context)"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuildFTSQuery_SynonymLookupIsCaseInsensitive(t *testing.T) {
	// The input is lowercased before synonym lookup, so "TENANT" hits "tenant".
	syns := map[string]string{"tenant": "fvTenant"}
	got := BuildFTSQuery("TENANT", syns)
	want := "(tenant OR fvTenant)"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuildFTSQuery_NilSynonyms(t *testing.T) {
	// Nil synonyms map must not panic.
	got := BuildFTSQuery("fabric", nil)
	if got != "fabric" {
		t.Errorf("expected %q, got %q", "fabric", got)
	}
}

func TestBuildFTSQuery_WhitespaceOnly(t *testing.T) {
	got := BuildFTSQuery("   ", map[string]string{})
	if got != "" {
		t.Errorf("expected empty string for whitespace-only input, got %q", got)
	}
}
