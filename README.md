<!-- SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0 -->

# BreachSAFE Evidence Go

Thin CLI composer for packaging caller-supplied evidence through the pinned official ePack core.

This repository does not scan, generate CBOM/OSCAL, render PDFs, or implement the ePack format.
Those responsibilities belong to QuReddy, Mint-OSCAL, breachsafe-pdf, and ePack respectively.

The repository consumes the shared `breachsafe-golden-go` toolchain image for CI gates.
