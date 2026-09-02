#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0

# Copyright 2026 Cisco Systems, Inc. and their affiliates

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

# http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# scripts/publish_release.sh - Build and publish data modules to GitHub Releases using `gh` CLI.
#
# Usage:
#   ./scripts/publish_release.sh [tag_name]
# Example:
#   ./scripts/publish_release.sh data-modules-latest

set -euo pipefail

TAG="${1:-data-modules-latest}"
DATA_DIR="${DATA_DIR:-./data}"

echo "=== Packaging and Publishing API Data Modules to Release '$TAG' ==="

if [ ! -f "$DATA_DIR/aci.db.gz" ] || [ ! -f "$DATA_DIR/ndfc.db.gz" ] || [ ! -f "$DATA_DIR/intersight.db.gz" ]; then
  echo "Building data modules first..."
  ./scripts/ingest_all.sh
fi

if ! command -v gh &> /dev/null; then
  echo "Error: GitHub CLI 'gh' is required to publish releases." >&2
  exit 1
fi

echo "Checking if release '$TAG' exists..."
if gh release view "$TAG" &>/dev/null; then
  echo "Release '$TAG' exists. Uploading/updating assets..."
  gh release upload "$TAG" \
    "$DATA_DIR"/aci.db.gz \
    "$DATA_DIR"/ndfc.db.gz \
    "$DATA_DIR"/intersight.db.gz \
    modules.json \
    --clobber
else
  echo "Creating new GitHub release '$TAG'..."
  gh release create "$TAG" \
    "$DATA_DIR"/aci.db.gz \
    "$DATA_DIR"/ndfc.db.gz \
    "$DATA_DIR"/intersight.db.gz \
    modules.json \
    --title "API Documentation Data Modules ($TAG)" \
    --notes "Modular SQLite databases for Cisco ACI, NDFC, and Intersight." \
    --latest=false
fi

echo "Successfully published modules to GitHub Release '$TAG'!"
