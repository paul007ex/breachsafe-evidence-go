<!-- SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0 -->

# BreachSAFE Evidence Go

Thin CLI composer for packaging caller-supplied evidence through the pinned official ePack core.

This repository does not scan, generate CBOM/OSCAL, render PDFs, or implement the ePack format.
Those responsibilities belong to QuReddy, Mint-OSCAL, breachsafe-pdf, and ePack respectively.

The repository consumes the shared `breachsafe-golden-go` toolchain image for CI gates.

## Container boundary

`Dockerfile` uses the published GitHub golden Go image for compilation and emits a
minimal runtime image containing this CLI and the components-disabled official ePack
core built from the upstream `main` branch. The image records the resolved upstream
commit at `/usr/share/breachsafe/epack.revision` and carries ePack's Apache license and
NOTICE files. The publish workflow disables Docker build cache so `EPACK_REF=main`
tracks the current upstream tip on each image build. A scheduled rebuild keeps the
`:latest` image fresh; versioned image tags remain the audit/release references.

The image runs as UID 65532. For bind-mounted output directories, grant that directory
write permission or run with an explicit operator UID; do not make the image privileged.

Example one-shot use:

```bash
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD:/work" -w /work \
  ghcr.io/paul007ex/breachsafe-evidence-go:latest version
```

The UX is not embedded in this image. `breachsafe-ux` may call this CLI in a later P2/P3
gateway integration once the CLI contract is stable.
