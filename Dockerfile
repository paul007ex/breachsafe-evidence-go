# syntax=docker/dockerfile:1
# SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

ARG GOLDEN_IMAGE=ghcr.io/paul007ex/breachsafe-golden-go:1.26.6

FROM ${GOLDEN_IMAGE} AS build

WORKDIR /src
ENV CGO_ENABLED=0 \
	GOFLAGS=-mod=readonly

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -trimpath -ldflags="-s -w" -o /out/breachsafe-evidence ./cmd/breachsafe-evidence

FROM scratch

LABEL org.opencontainers.image.title="BreachSAFE Evidence composer" \
	org.opencontainers.image.description="Thin CLI boundary over the pinned official ePack core" \
	org.opencontainers.image.licenses="PolyForm-Noncommercial-1.0.0"

WORKDIR /work
COPY --from=build /out/breachsafe-evidence /usr/local/bin/breachsafe-evidence

# The official ePack core is supplied separately and mounted at this path.
# This keeps ePack provenance and licensing explicit instead of copying an
# unpinned or unverified binary into the BreachSAFE image.
ENV BREACHSAFE_EPACK_BIN=/usr/local/bin/epack
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/breachsafe-evidence"]
