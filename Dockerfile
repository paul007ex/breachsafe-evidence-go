# syntax=docker/dockerfile:1
# SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

ARG GOLDEN_IMAGE=ghcr.io/paul007ex/breachsafe-golden-go:1.26.6
ARG EPACK_REF=main

FROM ${GOLDEN_IMAGE} AS build

WORKDIR /src
ENV CGO_ENABLED=0 \
	GOFLAGS=-mod=readonly

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -trimpath -ldflags="-s -w" -o /out/breachsafe-evidence ./cmd/breachsafe-evidence

# Build the components-disabled official ePack core from the requested upstream
# branch at image-build time. The publish workflow disables cache so EPACK_REF=main
# really tracks the current upstream main branch.
RUN git clone --depth 1 --branch "${EPACK_REF}" https://github.com/locktivity/epack.git /tmp/epack \
	&& cd /tmp/epack \
	&& git rev-parse HEAD > /out/epack.revision \
	&& go build -trimpath -ldflags="-s -w" -o /out/epack ./cmd/epack \
	&& cp LICENSE NOTICE /out/

FROM scratch

LABEL org.opencontainers.image.title="BreachSAFE Evidence composer" \
	org.opencontainers.image.description="Thin CLI boundary over the pinned official ePack core" \
	org.opencontainers.image.licenses="PolyForm-Noncommercial-1.0.0"

WORKDIR /work
COPY --from=build /out/breachsafe-evidence /usr/local/bin/breachsafe-evidence
COPY --from=build /out/epack /usr/local/bin/epack
COPY --from=build /out/epack.revision /usr/share/breachsafe/epack.revision
COPY --from=build /out/LICENSE /usr/share/licenses/epack/LICENSE
COPY --from=build /out/NOTICE /usr/share/licenses/epack/NOTICE

ENV BREACHSAFE_EPACK_BIN=/usr/local/bin/epack
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/breachsafe-evidence"]
