<!-- SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0 -->

# BreachSAFE Evidence Go

Thin CLI composer for packaging caller-supplied evidence through the pinned official ePack core.

This repository does not scan, generate CBOM/OSCAL, render PDFs, or implement the ePack format.
Those responsibilities belong to QuReddy, Mint-OSCAL, breachsafe-pdf, and ePack respectively.

The repository consumes the shared `breachsafe-golden-go` toolchain image for CI gates.

## Container boundary

`Dockerfile` uses the published GitHub golden Go image for compilation and emits a
minimal runtime image containing only this CLI. The official ePack core remains an
explicit runtime dependency: mount the approved ePack binary at
`/usr/local/bin/epack` and keep its release/provenance with the deployment record.

The UX is not embedded in this image. `breachsafe-ux` may call this CLI in a later P2/P3
gateway integration once the CLI contract is stable.
