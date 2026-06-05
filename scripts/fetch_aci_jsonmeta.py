#!/usr/bin/env python3
"""
fetch_aci_jsonmeta.py - Download per-class JSON documentation from an APIC.

The APIC exposes rich class metadata (descriptions, DN formats, property types,
valid values, etc.) at:

    https://<apic>/doc/jsonmeta/<package>/<ClassName>.json

This script reads the pyaci aci-meta.json file for the class inventory, then
downloads each configurable+non-abstract class's doc JSON into an output
directory tree: <output-dir>/<package>/<ClassName>.json

No authentication is required for the /doc/jsonmeta/ endpoint.

Usage:
    python3 scripts/fetch_aci_jsonmeta.py \\
        --apic 10.122.208.110 \\
        --meta ~/src/cisco-api-guide-mcp/assets/aci-meta.json \\
        --out  ~/src/cisco-api-guide-mcp/assets/aci-jsonmeta \\
        [--all]          # include abstract/non-configurable classes too
        [--workers 20]   # parallel downloads (default: 10)
        [--no-verify]    # skip TLS certificate verification (default: skip)
"""

import argparse
import json
import os
import re
import ssl
import sys
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

CLASS_RE = re.compile(r'^([a-z][a-z0-9]*)([A-Z].*)$')


def split_class_name(name: str):
    """Split camelCase ACI class name into (package, ClassName).

    "fvTenant"    -> ("fv",    "Tenant")
    "l3extOut"    -> ("l3ext", "Out")
    "aaaAProvider"-> ("aaa",   "AProvider")
    Returns (None, None) if the name doesn't match the expected pattern.
    """
    m = CLASS_RE.match(name)
    if not m:
        return None, None
    return m.group(1), m.group(2)


def fetch_one(url: str, dest: Path, ctx: ssl.SSLContext) -> tuple[str, str | None]:
    """Download url to dest. Returns (url, error_msg | None)."""
    try:
        with urllib.request.urlopen(url, context=ctx, timeout=15) as resp:
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(resp.read())
        return url, None
    except urllib.error.HTTPError as e:
        return url, f"HTTP {e.code}"
    except Exception as e:
        return url, str(e)


def main():
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--apic', required=True,
                        help='APIC hostname or IP (e.g. 10.122.208.110)')
    parser.add_argument('--meta', required=True,
                        help='Path to aci-meta.json (pyaci format)')
    parser.add_argument('--out', required=True,
                        help='Output directory for per-class JSON files')
    parser.add_argument('--all', action='store_true',
                        help='Download all classes, not just configurable+non-abstract')
    parser.add_argument('--workers', type=int, default=10,
                        help='Number of parallel download workers (default: 10)')
    parser.add_argument('--no-verify', action='store_true', default=True,
                        help='Skip TLS certificate verification (default: True)')
    parser.add_argument('--skip-existing', action='store_true', default=True,
                        help='Skip classes whose output file already exists (default: True)')
    args = parser.parse_args()

    meta_path = Path(args.meta).expanduser()
    out_dir = Path(args.out).expanduser()

    print(f"Loading {meta_path} ...", file=sys.stderr)
    with open(meta_path) as f:
        meta = json.load(f)

    classes = meta.get('classes', {})
    print(f"Total classes in meta: {len(classes)}", file=sys.stderr)

    # Build work list
    tasks = []  # list of (url, dest_path)
    skipped_parse = 0
    skipped_filter = 0

    for class_name, cls_data in classes.items():
        if not args.all:
            if cls_data.get('isAbstract') or not cls_data.get('isConfigurable'):
                skipped_filter += 1
                continue

        pkg, cls = split_class_name(class_name)
        if not pkg:
            skipped_parse += 1
            continue

        dest = out_dir / pkg / f"{cls}.json"
        if args.skip_existing and dest.exists():
            continue

        url = f"https://{args.apic}/doc/jsonmeta/{pkg}/{cls}.json"
        tasks.append((url, dest))

    print(f"Classes to download: {len(tasks)} "
          f"(filtered: {skipped_filter}, unparseable names: {skipped_parse})",
          file=sys.stderr)

    if not tasks:
        print("Nothing to download.", file=sys.stderr)
        return

    # TLS context
    ctx = ssl.create_default_context()
    if args.no_verify:
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE

    # Download in parallel
    errors = []
    done = 0
    total = len(tasks)

    with ThreadPoolExecutor(max_workers=args.workers) as pool:
        futures = {pool.submit(fetch_one, url, dest, ctx): url
                   for url, dest in tasks}
        for fut in as_completed(futures):
            url, err = fut.result()
            done += 1
            if err:
                errors.append((url, err))
                print(f"  ERROR {url}: {err}", file=sys.stderr)
            else:
                if done % 100 == 0 or done == total:
                    print(f"  {done}/{total} downloaded", file=sys.stderr)

    if errors:
        print(f"\n{len(errors)} errors:", file=sys.stderr)
        for url, err in errors[:20]:
            print(f"  {err:20s}  {url}", file=sys.stderr)
        if len(errors) > 20:
            print(f"  ... and {len(errors) - 20} more", file=sys.stderr)

    print(f"\nDone. Downloaded {total - len(errors)}/{total} class docs to {out_dir}",
          file=sys.stderr)


if __name__ == '__main__':
    main()
