# syntax=docker/dockerfile:1
# SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

ARG GOLDEN_IMAGE=ghcr.io/paul007ex/breachsafe-golden-go:1.26.6
ARG EPACK_REF=main
ARG PDF_REF=main

FROM ${GOLDEN_IMAGE} AS build

ARG EPACK_REF
ARG PDF_REF

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
	&& git -C /tmp/epack rev-parse HEAD > /tmp/out/epack.revision \
	&& go -C /tmp/epack build -trimpath -ldflags="-s -w" -o /tmp/out/epack ./cmd/epack \
	&& sha256sum /tmp/out/epack > /tmp/out/epack.sha256 \
	&& cp /tmp/epack/LICENSE /tmp/out/epack.LICENSE \
	&& cp /tmp/epack/NOTICE /tmp/out/epack.NOTICE

# Build the canonical profile-driven PDF compiler from the latest upstream main.
RUN git clone --depth 1 --branch "${PDF_REF}" https://github.com/paul007ex/breachsafe-pdf.git /tmp/breachsafe-pdf \
	&& git -C /tmp/breachsafe-pdf rev-parse HEAD > /tmp/out/breachsafe-pdf.revision \
	&& go -C /tmp/breachsafe-pdf build -trimpath -ldflags="-s -w" -o /tmp/out/breachsafe-pdf ./cmd/breachsafe-pdf \
	&& sha256sum /tmp/out/breachsafe-pdf > /tmp/out/breachsafe-pdf.sha256 \
	&& cp LICENSE /tmp/out/breachsafe-pdf.LICENSE \
	&& cp NOTICE /tmp/out/breachsafe-pdf.NOTICE

FROM scratch

LABEL org.opencontainers.image.title="BreachSAFE Evidence composer" \
	org.opencontainers.image.vendor="BreachSAFE" \
	org.opencontainers.image.source="https://github.com/paul007ex/breachsafe-evidence-go" \
	org.opencontainers.image.description="Thin CLI boundary over the latest official ePack and BreachSAFE PDF tools" \
	org.opencontainers.image.licenses="PolyForm-Noncommercial-1.0.0" \
	org.opencontainers.image.base.name="ghcr.io/paul007ex/breachsafe-golden-go:1.26.6"

WORKDIR /work
COPY --from=build /tmp/out/breachsafe-evidence /usr/local/bin/breachsafe-evidence
COPY --from=build /tmp/out/epack /usr/local/bin/epack
COPY --from=build /tmp/out/epack.revision /usr/share/breachsafe/epack.revision
COPY --from=build /tmp/out/epack.sha256 /usr/share/breachsafe/epack.sha256
COPY --from=build /tmp/out/breachsafe-pdf /usr/local/bin/breachsafe-pdf
COPY --from=build /tmp/out/breachsafe-pdf.revision /usr/share/breachsafe/breachsafe-pdf.revision
COPY --from=build /tmp/out/breachsafe-pdf.sha256 /usr/share/breachsafe/breachsafe-pdf.sha256
COPY --from=build /tmp/out/epack.LICENSE /usr/share/licenses/epack/LICENSE
COPY --from=build /tmp/out/epack.NOTICE /usr/share/licenses/epack/NOTICE
COPY --from=build /tmp/out/breachsafe-pdf.LICENSE /usr/share/licenses/breachsafe-pdf/LICENSE
COPY --from=build /tmp/out/breachsafe-pdf.NOTICE /usr/share/licenses/breachsafe-pdf/NOTICE

ENV BREACHSAFE_EPACK_BIN=/usr/local/bin/epack
ENV BREACHSAFE_PDF_BIN=/usr/local/bin/breachsafe-pdf
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/breachsafe-evidence"]
