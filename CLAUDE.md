<!-- SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0 -->

# breachsafe-evidence-go guidance for Claude Code

## Contents

1. [What this is](#1-what-this-is)
2. [Repository card](#2-repository-card)
3. [Scope guardrails](#3-scope-guardrails)
4. [Executable boundary: the digest env contract](#4-executable-boundary-the-digest-env-contract)
5. [Layout](#5-layout)
6. [Gates](#6-gates)
7. [P0 acceptance and the Docker workflow](#7-p0-acceptance-and-the-docker-workflow)
8. [Licence](#8-licence)
9. [Change procedure](#9-change-procedure)

## 1. What this is

A thin Go CLI composer. It packages caller-supplied evidence (a scan, a CBOM, a PDF, and
extras) through the official ePack core, and can drive `breachsafe-pdf render` to produce the
PDF first. It owns composition and receipt verification, nothing else. `README.md` is the
product-facing reference; this file is the agent operating card.

Subcommands, verified in `cmd/breachsafe-evidence/main.go`: `pack`, `report`, `version`,
`inspect`, `verify`, `extract`, `unpack`, `diff`.

## 2. Repository card

| | |
|---|---|
| Remote | `paul007ex/breachsafe-evidence-go`, public |
| Module | `github.com/paul007ex/breachsafe-evidence-go` |
| Language | Go `1.26.0`, toolchain `go1.26.6` (`go.mod`) |
| Licence | PolyForm-Noncommercial-1.0.0 (not the qureddy Apache carve-out) |
| Toolchain image | `ghcr.io/paul007ex/breachsafe-golden-go` |
| No `Justfile` | Gates are Go commands, run in the golden-go image. See §6 |

## 3. Scope guardrails

This repository is a thin composer. It does NOT:

- scan a target or acquire evidence,
- generate CBOM or OSCAL,
- render or lay out PDFs,
- implement the ePack format.

Those belong to QuReddy (scan), Mint-OSCAL (OSCAL), `breachsafe-pdf` (PDF render), and the
upstream ePack core respectively. A change that reaches into any of those responsibilities
belongs in that repository, not here. Keep this CLI a composer over pinned external binaries.

## 4. Executable boundary: the digest env contract

The two external tools are selected by env var and pinned by SHA-256. This is deliberate and
enforced in `cmd/breachsafe-evidence/main.go`.

| Var | Meaning | Digest checked against |
|---|---|---|
| `BREACHSAFE_EPACK_BIN` | absolute path to the ePack executable | `BREACHSAFE_EPACK_SHA256`, else `/usr/share/breachsafe/epack.sha256` |
| `BREACHSAFE_EPACK_SHA256` | approved ePack digest | — |
| `BREACHSAFE_PDF_BIN` | absolute path to `breachsafe-pdf` | `BREACHSAFE_PDF_SHA256`, else `/usr/share/breachsafe/breachsafe-pdf.sha256` |
| `BREACHSAFE_PDF_SHA256` | approved `breachsafe-pdf` digest | — |

`*_BIN` must be an absolute path to an executable regular file, or the command errors before
doing any work. The published image supplies the reference digests under
`/usr/share/breachsafe/`. Do not weaken this to a bare `PATH` lookup or drop the digest check.

## 5. Layout

| Path | Holds |
|---|---|
| `cmd/breachsafe-evidence/` | the CLI entry point and its tests |
| `internal/epackcli/` | the ePack subprocess runner |
| `scripts/p0_acceptance.sh` | the P0 end-to-end acceptance script |
| `testdata/p0/` | sanitized `cbom.json`, `scan.json`, `request.json` fixtures |
| `Dockerfile` | multi-stage build on the golden-go image |
| `.github/workflows/` | `ci.yml` (gates), `container.yml` (image publish) |

## 6. Gates

No local runner script. CI (`.github/workflows/ci.yml`) runs inside
`ghcr.io/paul007ex/breachsafe-golden-go:1.26` (series tag, not the patch) and executes, in
order: `golden-go-doctor`, `gofmt -l` check, `go vet ./...`, `go test -count=1 ./...`,
`go test -race -count=1 ./...`, `go mod verify`, `staticcheck ./...`, `gosec ./...`,
`golangci-lint run`, `govulncheck ./...`, `osv-scanner scan source -r .`, `git diff --check`.

Locally the fast path is `go test ./...` plus `go vet ./...` and `gofmt -l .`. The reproducing
environment for the full gate set is the golden-go image, not a laptop Go install. GitHub
Actions do not run on private BQP repos, but this repo is public, so CI runs hosted.

## 7. P0 acceptance and the Docker workflow

P0 acceptance, from `scripts/p0_acceptance.sh`, needs the four digest env vars set (§4) and
approved ePack and PDF binaries. It generates every pack and PDF in a temp directory (nothing
generated is committed) and asserts the three-artifact pack and five-artifact report pack:

```bash
BREACHSAFE_EVIDENCE_BIN=./breachsafe-evidence \
  BREACHSAFE_EPACK_BIN=/absolute/path/to/epack \
  BREACHSAFE_EPACK_SHA256=<sha256> \
  BREACHSAFE_PDF_BIN=/absolute/path/to/breachsafe-pdf \
  BREACHSAFE_PDF_SHA256=<sha256> \
  ./scripts/p0_acceptance.sh
```

The image is built and published by `container.yml` (nightly `schedule` plus dispatch), which
rebuilds ePack and `breachsafe-pdf` from upstream `main` with the Docker cache disabled.
Versioned image tags are the audit references; `:latest` is the moving nightly. Build locally
with `docker build --pull --no-cache -t breachsafe-evidence-go:local .`; `--pull --no-cache`
is intentional, it refreshes the golden-go base and rebuilds the pinned upstream binaries
instead of reusing a stale layer. The runtime image runs as UID 65532: grant bind-mounted
output directories write permission rather than making the container privileged.

## 8. Licence

PolyForm-Noncommercial-1.0.0 on every first-party file (`REUSE.toml`, `LICENSES/`, `NOTICE`).
Source-available for non-commercial use; describe it that way, not as open source. Repository
history once carried an Apache-2.0 grant that remains effective for copies received under it;
the current prospective licence is PolyForm-NC. Run `reuse lint` after touching headers.

## 9. Change procedure

Follow the BQP platform ten-step change loop (parent `CLAUDE.md` §1): inventory, steelman,
isolated reproduction in a worktree, pressure test, surgical change, regression tests, the §6
gates with real exit codes, architecture review, issue and git workflow, release verification.
Report each step completed or `NOT RUN` with a reason. Work in an isolated worktree or clone.
The release artifact here is the published container image (§7), not a package-index upload.
