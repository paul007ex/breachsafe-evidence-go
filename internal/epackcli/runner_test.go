// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package epackcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerRejectsMissingInputs(t *testing.T) {
	if _, err := (Runner{}).Run(context.Background(), "version"); err == nil {
		t.Fatal("expected missing executable error")
	}
	if _, err := (Runner{Path: "/bin/echo"}).Run(context.Background()); err == nil {
		t.Fatal("expected missing command error")
	}
}

func TestRunnerPropagatesFailureAndBoundsOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho failure >&2\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runner{Path: script}).Run(context.Background(), "version"); err == nil || !strings.Contains(err.Error(), "failure") {
		t.Fatalf("expected stderr in error, got %v", err)
	}
}
