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

# scripts/ingest_all.sh - Ingest products into modular SQLite DBs and generate modules.json.

set -euo pipefail

ASSETS_DIR="${ASSETS_DIR:-./assets}"
OUTPUT_DIR="${OUTPUT_DIR:-./data}"
INGEST="${INGEST:-go run ./cmd/ingest}"
ACI_AUX_DIR="${ACI_AUX_DIR:-}" # optional: path to downloaded APIC per-class JSON dir

echo "=== Cisco API Guide — Ingest Modular Products ==="
echo "Assets Dir: $ASSETS_DIR"
echo "Output Dir: $OUTPUT_DIR"
echo

mkdir -p "$OUTPUT_DIR"

run_ingest() {
  local target_db="$1"
  shift
  $INGEST --db "$target_db" "$@"
}

# ---------------------------------------------------------------------------
# 1. NDFC
# ---------------------------------------------------------------------------
NDFC_DB="$OUTPUT_DIR/ndfc.db"
rm -f "$NDFC_DB" "$NDFC_DB.gz"
echo "--- NDFC: initialising product ---"
run_ingest "$NDFC_DB" \
  --product ndfc \
  --init \
  --name "Nexus Dashboard Fabric Controller (NDFC)" \
  --description "REST API for Cisco NDFC (formerly DCNM). Manages data-centre fabric infrastructure, VXLANs, and network policies." \
  --base-url "https://<ndfc-host>" \
  --auth-type "bearer" \
  --auth-notes "Obtain a token via POST /login with {userName, userPasswd, domain}. Pass as Authorization: Bearer <token>." \
  --alias "dcnm,ndfc,nexus dashboard fabric controller"

if [ -f "$ASSETS_DIR/ndfc/3.2.2m/ndfc-apischema.json" ]; then
  echo "--- NDFC: ingesting release 3.2.2m ---"
  run_ingest "$NDFC_DB" \
    --product ndfc \
    --release "3.2.2m" \
    --format openapi3 \
    --input "$ASSETS_DIR/ndfc/3.2.2m/ndfc-apischema.json" \
    --prune-major
fi

if [ -d "$ASSETS_DIR/ndfc/4.1.1" ]; then
  echo "--- NDFC: ingesting release 4.1.1 (3 files) ---"
  NDFC_411_FILES=(infra manage oneManage)
  for i in "${!NDFC_411_FILES[@]}"; do
    f="${NDFC_411_FILES[$i]}"
    echo "  -> $f.json"
    PRUNE_FLAG=()
    if [ "$i" -eq 0 ]; then PRUNE_FLAG=(--prune-major); fi
    run_ingest "$NDFC_DB" \
      --product ndfc \
      --release "4.1.1" \
      --format openapi3 \
      --input "$ASSETS_DIR/ndfc/4.1.1/${f}.json" \
      "${PRUNE_FLAG[@]}"
  done
fi

gzip -c "$NDFC_DB" > "$NDFC_DB.gz"
echo "NDFC DB size: $(du -sh "$NDFC_DB" | cut -f1) (compressed: $(du -sh "$NDFC_DB.gz" | cut -f1))"
echo

# ---------------------------------------------------------------------------
# 2. Intersight
# ---------------------------------------------------------------------------
INTERSIGHT_DB="$OUTPUT_DIR/intersight.db"
rm -f "$INTERSIGHT_DB" "$INTERSIGHT_DB.gz"
echo "--- Intersight: initialising product ---"
run_ingest "$INTERSIGHT_DB" \
  --product intersight \
  --init \
  --name "Cisco Intersight" \
  --description "REST API for Cisco Intersight, a SaaS platform for infrastructure management across data centre, edge, and cloud." \
  --base-url "https://intersight.com" \
  --auth-type "oauth2" \
  --auth-notes "Intersight uses OAuth 2.0 client credentials or API key authentication. Generate an API key in the Intersight portal." \
  --alias "intersight,ucs"

INTERSIGHT_YAML=$(ls "$ASSETS_DIR"/intersight/*.yaml "$ASSETS_DIR"/intersight/*.json 2>/dev/null | head -1 || true)
if [ -n "$INTERSIGHT_YAML" ]; then
  INTERSIGHT_VERSION=$(basename "$INTERSIGHT_YAML" | sed -E 's/.*([0-9]+\.[0-9]+\.[0-9]+).*/\1/')
  if [ -z "$INTERSIGHT_VERSION" ]; then INTERSIGHT_VERSION="1.0.11"; fi
  echo "--- Intersight: ingesting release $INTERSIGHT_VERSION ---"
  run_ingest "$INTERSIGHT_DB" \
    --product intersight \
    --release "$INTERSIGHT_VERSION" \
    --format openapi3 \
    --input "$INTERSIGHT_YAML" \
    --prune-major
fi

gzip -c "$INTERSIGHT_DB" > "$INTERSIGHT_DB.gz"
echo "Intersight DB size: $(du -sh "$INTERSIGHT_DB" | cut -f1) (compressed: $(du -sh "$INTERSIGHT_DB.gz" | cut -f1))"
echo

# ---------------------------------------------------------------------------
# 3. ACI
# ---------------------------------------------------------------------------
ACI_DB="$OUTPUT_DIR/aci.db"
rm -f "$ACI_DB" "$ACI_DB.gz"
echo "--- ACI: initialising product ---"
run_ingest "$ACI_DB" \
  --product aci \
  --init \
  --name "Cisco ACI (APIC REST API)" \
  --description "REST API for Cisco Application Centric Infrastructure (ACI). Uses a managed-object model; every resource is a managed object addressed by its distinguished name (DN)." \
  --base-url "https://<apic>" \
  --auth-type "cookie" \
  --auth-notes "Authenticate via POST /api/aaaLogin.json with {aaaUser:{attributes:{name,pwd}}}. The response sets an APIC-cookie. Pass this cookie in subsequent requests." \
  --alias "aci,apic,application centric infrastructure"

ACI_EXTRA_FLAGS=()
if [ -n "$ACI_AUX_DIR" ] && [ -d "$ACI_AUX_DIR" ]; then
  echo "--- ACI: using aux-dir $ACI_AUX_DIR for per-class APIC JSON docs ---"
  ACI_EXTRA_FLAGS=(--aux-dir "$ACI_AUX_DIR")
elif [ -d "$ASSETS_DIR/aci/jsonmeta" ]; then
  echo "--- ACI: found $ASSETS_DIR/aci/jsonmeta for per-class APIC JSON docs ---"
  ACI_EXTRA_FLAGS=(--aux-dir "$ASSETS_DIR/aci/jsonmeta")
fi

if [ -f "$ASSETS_DIR/aci/aci-meta.json" ]; then
  echo "--- ACI: ingesting release 5.2 ---"
  run_ingest "$ACI_DB" \
    --product aci \
    --release "5.2" \
    --format aci-meta \
    --input "$ASSETS_DIR/aci/aci-meta.json" \
    --prune-major \
    "${ACI_EXTRA_FLAGS[@]}"
fi

gzip -c "$ACI_DB" > "$ACI_DB.gz"
echo "ACI DB size: $(du -sh "$ACI_DB" | cut -f1) (compressed: $(du -sh "$ACI_DB.gz" | cut -f1))"
echo

# ---------------------------------------------------------------------------
# 4. Generate modules.json manifest
# ---------------------------------------------------------------------------
echo "--- Generating modules.json ---"

calc_sha256() {
  shasum -a 256 "$1" | cut -d' ' -f1
}

ACI_HASH=$(calc_sha256 "$ACI_DB")
NDFC_HASH=$(calc_sha256 "$NDFC_DB")
INTERSIGHT_HASH=$(calc_sha256 "$INTERSIGHT_DB")

cat << MANIFEST_EOF > "$OUTPUT_DIR/modules.json"
{
  "version": 1,
  "modules": {
    "aci": {
      "name": "Cisco ACI (APIC REST API)",
      "product_id": "aci",
      "version": "5.2",
      "description": "REST API for Cisco Application Centric Infrastructure (ACI).",
      "size_bytes": $(wc -c < "$ACI_DB" | tr -d ' '),
      "sha256": "$ACI_HASH",
      "url": "https://github.com/cisco-open/cisco-api-guide-mcp/releases/download/data-modules-latest/aci.db.gz",
      "aliases": ["apic", "application centric infrastructure"]
    },
    "ndfc": {
      "name": "Nexus Dashboard Fabric Controller (NDFC)",
      "product_id": "ndfc",
      "version": "4.1.1",
      "description": "REST API for Cisco NDFC (formerly DCNM).",
      "size_bytes": $(wc -c < "$NDFC_DB" | tr -d ' '),
      "sha256": "$NDFC_HASH",
      "url": "https://github.com/cisco-open/cisco-api-guide-mcp/releases/download/data-modules-latest/ndfc.db.gz",
      "aliases": ["dcnm", "nexus dashboard fabric controller"]
    },
    "intersight": {
      "name": "Cisco Intersight",
      "product_id": "intersight",
      "version": "1.0.11",
      "description": "REST API for Cisco Intersight SaaS platform.",
      "size_bytes": $(wc -c < "$INTERSIGHT_DB" | tr -d ' '),
      "sha256": "$INTERSIGHT_HASH",
      "url": "https://github.com/cisco-open/cisco-api-guide-mcp/releases/download/data-modules-latest/intersight.db.gz",
      "aliases": ["ucs"]
    }
  }
}
MANIFEST_EOF

cp "$OUTPUT_DIR/modules.json" ./modules.json

echo "=== Ingest & Modular Packaging complete ==="
