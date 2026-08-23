#!/usr/bin/env bash
# SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0
set -euo pipefail

evidence_bin=${BREACHSAFE_EVIDENCE_BIN:-breachsafe-evidence}
fixture_dir=${BREACHSAFE_P0_FIXTURES:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../testdata/p0" && pwd)}

if ! command -v "$evidence_bin" >/dev/null 2>&1 && [[ ! -x "$evidence_bin" ]]; then
	printf 'evidence CLI is not executable: %s\n' "$evidence_bin" >&2
	exit 1
fi
for required in BREACHSAFE_EPACK_BIN BREACHSAFE_EPACK_SHA256 BREACHSAFE_PDF_BIN BREACHSAFE_PDF_SHA256; do
	if [[ -z "${!required:-}" ]]; then
		printf '%s must be set\n' "$required" >&2
		exit 1
	fi
done
for fixture in request.json scan.json cbom.json; do
	if [[ ! -s "$fixture_dir/$fixture" ]]; then
		printf 'missing P0 fixture: %s\n' "$fixture_dir/$fixture" >&2
		exit 1
	fi
done

workdir=$(mktemp -d "${TMPDIR:-/tmp}/breachsafe-p0-acceptance.XXXXXX")
trap 'rm -rf "$workdir"' EXIT

"$evidence_bin" version >/dev/null
"$evidence_bin" pack \
	--stream breachsafe/p0 \
	--request "$fixture_dir/request.json" \
	--scan "$fixture_dir/scan.json" \
	--cbom "$fixture_dir/cbom.json" \
	--output "$workdir/pack.epack" >"$workdir/pack.inspect.json"
jq -e '.artifact_count == 3 and ([.artifacts[].path] | sort == ["artifacts/cbom/cbom.json", "artifacts/request/request.json", "artifacts/scan/scan.json"])' "$workdir/pack.inspect.json" >/dev/null

"$evidence_bin" inspect --json "$workdir/pack.epack" >"$workdir/inspect.json"
"$evidence_bin" verify --integrity-only "$workdir/pack.epack" >"$workdir/verify.txt"
"$evidence_bin" extract --all "$workdir/pack.epack" --output "$workdir/extracted" >/dev/null
"$evidence_bin" unpack --all "$workdir/pack.epack" --output "$workdir/unpacked" >/dev/null
"$evidence_bin" diff "$workdir/pack.epack" "$workdir/pack.epack" >"$workdir/diff.txt"

"$evidence_bin" report \
	--profile breachsafe/community \
	--request "$fixture_dir/request.json" \
	--scan-json "$fixture_dir/scan.json" \
	--cbom "$fixture_dir/cbom.json" \
	--pdf "$workdir/report.pdf" \
	--result "$workdir/report.result.json" \
	--stream breachsafe/p0 \
	--output "$workdir/report.epack" >"$workdir/report.inspect.json" 2>"$workdir/report.pdf.stderr"
jq -e '.artifact_count == 5 and ([.artifacts[].path] | sort == ["artifacts/cbom/cbom.json", "artifacts/pdf/report.pdf", "artifacts/request/request.json", "artifacts/result/report.result.json", "artifacts/scan/scan.json"])' "$workdir/report.inspect.json" >/dev/null
"$evidence_bin" verify --integrity-only "$workdir/report.epack" >"$workdir/report.verify.txt"

printf 'P0 acceptance: PASS\n'
