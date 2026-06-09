#!/usr/bin/env bash
# scripts/ingest_all.sh - Ingest all three MVP products into the SQLite DB.
#
# Prerequisites:
#   - Run `go build -o bin/ingest ./cmd/ingest` first, or use `go run ./cmd/ingest`
#   - ACI: optionally run scripts/fetch_aci_jsonmeta.py first (see notes below)
#
# ACI note: the aci-meta.json alone provides minimal data (no descriptions,
# no DN formats). For full API guide data, first download per-class APIC JSON:
#
#   python3 scripts/fetch_aci_jsonmeta.py \
#       --apic 10.122.208.110 \
#       --meta ../assets/aci-meta.json \
#       --out  ../assets/aci-jsonmeta
#
# Then re-run this script (or set ACI_AUX_DIR below).

set -euo pipefail

ASSETS_DIR="${ASSETS_DIR:-../assets}"
DB="${DB:-./internal/embeddb/api.db}"
INGEST="${INGEST:-go run ./cmd/ingest}"
ACI_AUX_DIR="${ACI_AUX_DIR:-}"  # optional: path to downloaded APIC per-class JSON dir

echo "=== Cisco API Guide — ingest all products ==="
echo "DB: $DB"
echo "Assets: $ASSETS_DIR"
echo

mkdir -p "$(dirname "$DB")"

# ---------------------------------------------------------------------------
# Helper: seed a product row, then run ingest for one or more input files.
# Usage: ingest_product <product> <name> <description> <base-url> <auth-type>
#                       <auth-notes> <aliases> <format> <release> [extra-flags...]
#                       -- <file> [file...]
# ---------------------------------------------------------------------------
run_ingest() {
    local flags=("$@")
    $INGEST --db "$DB" "${flags[@]}"
}

# ---------------------------------------------------------------------------
# 1. NDFC
# ---------------------------------------------------------------------------
echo "--- NDFC: initialising product ---"
run_ingest \
    --product ndfc \
    --init \
    --name "Nexus Dashboard Fabric Controller (NDFC)" \
    --description "REST API for Cisco NDFC (formerly DCNM). Manages data-centre fabric infrastructure, VXLANs, and network policies." \
    --base-url "https://<ndfc-host>" \
    --auth-type "bearer" \
    --auth-notes "Obtain a token via POST /login with {userName, userPasswd, domain}. Pass as Authorization: Bearer <token>." \
    --alias "dcnm,ndfc,nexus dashboard fabric controller"

echo "--- NDFC: ingesting release 3.2.2m ---"
run_ingest \
    --product ndfc \
    --release "3.2.2m" \
    --format openapi3 \
    --input "$ASSETS_DIR/ndfc/3.2.2m/ndfc-apischema.json" \
    --prune-major

echo "--- NDFC: ingesting release 4.1.1 (3 files) ---"
NDFC_411_FILES=(infra manage oneManage)
for i in "${!NDFC_411_FILES[@]}"; do
    f="${NDFC_411_FILES[$i]}"
    echo "  -> $f.json"
    # Only prune on the first file to avoid deleting sibling files mid-run.
    PRUNE_FLAG=()
    if [ "$i" -eq 0 ]; then PRUNE_FLAG=(--prune-major); fi
    run_ingest \
        --product ndfc \
        --release "4.1.1" \
        --format openapi3 \
        --input "$ASSETS_DIR/ndfc/4.1.1/${f}.json" \
        "${PRUNE_FLAG[@]}"
done

echo

# ---------------------------------------------------------------------------
# 2. Intersight
# ---------------------------------------------------------------------------
echo "--- Intersight: initialising product ---"
run_ingest \
    --product intersight \
    --init \
    --name "Cisco Intersight" \
    --description "REST API for Cisco Intersight, a SaaS platform for infrastructure management across data centre, edge, and cloud." \
    --base-url "https://intersight.com" \
    --auth-type "oauth2" \
    --auth-notes "Intersight uses OAuth 2.0 client credentials or API key authentication. Generate an API key in the Intersight portal." \
    --alias "intersight"

INTERSIGHT_YAML=$(ls "$ASSETS_DIR"/intersight-openapi-v3-*.yaml 2>/dev/null | head -1)
if [ -z "$INTERSIGHT_YAML" ]; then
    echo "ERROR: No Intersight YAML found in $ASSETS_DIR" >&2
    exit 1
fi
# Extract version from filename: intersight-openapi-v3-1.0.11-*.yaml -> 1.0.11
INTERSIGHT_VERSION=$(basename "$INTERSIGHT_YAML" | sed 's/intersight-openapi-v3-\([0-9][0-9.]*\).*/\1/')

echo "--- Intersight: ingesting release $INTERSIGHT_VERSION ---"
run_ingest \
    --product intersight \
    --release "$INTERSIGHT_VERSION" \
    --format openapi3 \
    --input "$INTERSIGHT_YAML" \
    --prune-major

echo

# ---------------------------------------------------------------------------
# 3. ACI
# ---------------------------------------------------------------------------
echo "--- ACI: initialising product ---"
run_ingest \
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
else
    echo "--- ACI: no ACI_AUX_DIR set; ingesting minimal data from aci-meta.json only ---"
    echo "         (run scripts/fetch_aci_jsonmeta.py for full descriptions and DN paths)"
fi

echo "--- ACI: ingesting release 5.2 ---"
run_ingest \
    --product aci \
    --release "5.2" \
    --format aci-meta \
    --input "$ASSETS_DIR/aci-meta.json" \
    --prune-major \
    "${ACI_EXTRA_FLAGS[@]}"

echo
echo "=== Ingest complete ==="
