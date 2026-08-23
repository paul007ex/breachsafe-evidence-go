<!-- SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0 -->

# BreachSAFE Evidence Go

Thin CLI composer for packaging caller-supplied evidence through the latest official ePack core.

This repository does not scan, generate CBOM/OSCAL, render PDFs, or implement the ePack format.
Those responsibilities belong to QuReddy, Mint-OSCAL, breachsafe-pdf, and ePack respectively.

The repository consumes the shared `breachsafe-golden-go` toolchain image for CI gates.

The executable boundary is deliberately explicit: `BREACHSAFE_EPACK_BIN` must identify the
ePack binary and `BREACHSAFE_EPACK_SHA256` must contain its approved SHA-256 digest. The
published image supplies the digest at `/usr/share/breachsafe/epack.sha256`.

The image also bundles the canonical `breachsafe-pdf` executable. Its digest and resolved
source revision are recorded at `/usr/share/breachsafe/breachsafe-pdf.sha256` and
`/usr/share/breachsafe/breachsafe-pdf.revision`.

`pack` validates the ePack inspection receipt before publication and writes that JSON receipt
to stdout. Role inputs are placed under deterministic paths such as `artifacts/cbom/` and
`artifacts/pdf/`; `--other` inputs use `artifacts/extra/`.

`report` calls `breachsafe-pdf render --profile ...`, then packages the generated PDF,
RenderResult, source inputs, and request with the same verified ePack path.

## Container boundary

`Dockerfile` uses the published GitHub golden Go image for compilation and emits a
minimal runtime image containing this CLI, the canonical `breachsafe-pdf` compiler, and the
components-disabled official ePack core built from the upstream `main` branches. The image
records the resolved upstream commits and carries both tools' license and NOTICE files.
The publish workflow disables Docker build cache so `EPACK_REF=main` and `PDF_REF=main`
track the current upstream tips on each image build. A scheduled rebuild keeps the
`:latest` image fresh; versioned image tags remain the audit/release references.

The image runs as UID 65532. For bind-mounted output directories, grant that directory
write permission or run with an explicit operator UID; do not make the image privileged.

## Copy/paste commands

The easiest workflow is: put your input files in one directory, mount that directory at
`/work`, then use paths beginning with `/work/` inside the container.

```bash
# 1. Pull the published image.
docker pull ghcr.io/paul007ex/breachsafe-evidence-go:latest

# 2. Create a working directory and place your files there.
mkdir -p evidence-input
cp /path/to/scan.json evidence-input/scan.json
cp /path/to/cbom.json evidence-input/cbom.json
cp /path/to/report.pdf evidence-input/report.pdf
# For the report command, also provide a breachsafe-pdf render request.
cp /path/to/request.json evidence-input/request.json

# 3. Build an ePack. The JSON receipt is written to the terminal.
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/evidence-input:/work" \
  ghcr.io/paul007ex/breachsafe-evidence-go:latest pack \
  --stream breachsafe/my-scan \
  --scan /work/scan.json \
  --cbom /work/cbom.json \
  --pdf /work/report.pdf \
  --output /work/evidence.epack

# 4. Verify the pack.
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/evidence-input:/work" \
  ghcr.io/paul007ex/breachsafe-evidence-go:latest \
  verify --integrity-only /work/evidence.epack

# Or generate the PDF and package the complete result in one operation.
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/evidence-input:/work" \
  ghcr.io/paul007ex/breachsafe-evidence-go:latest report \
  --profile breachsafe/community \
  --request /work/request.json \
  --cbom /work/cbom.json \
  --scan-json /work/scan.json \
  --pdf /work/report.pdf \
  --result /work/report.result.json \
  --stream breachsafe/my-scan \
  --output /work/evidence.epack

# 5. Inspect its manifest and digests.
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/evidence-input:/work" \
  ghcr.io/paul007ex/breachsafe-evidence-go:latest \
  inspect --json /work/evidence.epack

# 6. Extract the artifacts into evidence-input/unpacked/.
mkdir -p evidence-input/unpacked
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/evidence-input:/work" \
  ghcr.io/paul007ex/breachsafe-evidence-go:latest \
  unpack --all -o /work/unpacked /work/evidence.epack
```

The resulting files are:

```text
evidence-input/
├── evidence.epack
├── scan.json
├── cbom.json
├── report.pdf
└── unpacked/
    ├── artifacts/scan/scan.json
    ├── artifacts/cbom/cbom.json
    └── artifacts/pdf/report.pdf
```

For local development, build the image from this checkout first:

```bash
docker build --pull --no-cache -t breachsafe-evidence-go:local .
```

Then replace `ghcr.io/paul007ex/breachsafe-evidence-go:latest` in the commands above with
`breachsafe-evidence-go:local`.

`--pull --no-cache` is intentional: it refreshes the golden Go base image and rebuilds the
official ePack binary from the current `EPACK_REF` instead of reusing an old Docker layer.

The image must be able to pull `ghcr.io/paul007ex/breachsafe-golden-go:1.26.6` while building.
If that package is private, authenticate Docker to GHCR first or use the published image.

Minimal smoke test:

```bash
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD:/work" -w /work \
  ghcr.io/paul007ex/breachsafe-evidence-go:latest version
```

The UX is not embedded in this image. `breachsafe-ux` may call this CLI in a later P2/P3
gateway integration once the CLI contract is stable.
