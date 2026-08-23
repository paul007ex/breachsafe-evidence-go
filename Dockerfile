# syntax=docker/dockerfile:1
# SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

ARG GOLDEN_IMAGE=ghcr.io/paul007ex/breachsafe-golden-go:1.26.6
ARG EPACK_REF=main

FROM ${GOLDEN_IMAGE} AS build

ARG EPACK_REF

WORKDIR /src
ENV CGO_ENABLED=0 \
	GOFLAGS=-mod=readonly

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN mkdir -p /tmp/out \
	&& go build -trimpath -ldflags="-s -w" -o /tmp/out/breachsafe-evidence ./cmd/breachsafe-evidence

# Build the components-disabled official ePack core from the requested upstream
# branch at image-build time. The publish workflow disables cache so EPACK_REF=main
# really tracks the current upstream main branch.
RUN git clone --depth 1 --branch "${EPACK_REF}" https://github.com/locktivity/epack.git /tmp/epack \
	&& cd /tmp/epack \
	&& git rev-parse HEAD > /tmp/out/epack.revision \
	&& go build -trimpath -ldflags="-s -w" -o /tmp/out/epack ./cmd/epack \
	&& cp LICENSE NOTICE /tmp/out/

FROM scratch

LABEL org.opencontainers.image.title="BreachSAFE Evidence composer" \
	org.opencontainers.image.vendor="BreachSAFE" \
	org.opencontainers.image.source="https://github.com/paul007ex/breachsafe-evidence-go" \
	org.opencontainers.image.description="Thin CLI boundary over the pinned official ePack core" \
	org.opencontainers.image.licenses="PolyForm-Noncommercial-1.0.0" \
	org.opencontainers.image.base.name="ghcr.io/paul007ex/breachsafe-golden-go:1.26.6"

WORKDIR /work
COPY --from=build /tmp/out/breachsafe-evidence /usr/local/bin/breachsafe-evidence
COPY --from=build /tmp/out/epack /usr/local/bin/epack
COPY --from=build /tmp/out/epack.revision /usr/share/breachsafe/epack.revision
COPY --from=build /tmp/out/LICENSE /usr/share/licenses/epack/LICENSE
COPY --from=build /tmp/out/NOTICE /usr/share/licenses/epack/NOTICE

ENV BREACHSAFE_EPACK_BIN=/usr/local/bin/epack
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/breachsafe-evidence"]
