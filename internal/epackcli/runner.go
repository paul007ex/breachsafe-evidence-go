// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package epackcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

const maxOutputBytes = 1 << 20

type Runner struct{ Path string }

// CommandRunner is the narrow process boundary used by the composer. Keeping
// this interface small makes the orchestration testable without replacing the
// official ePack binary.
type CommandRunner interface {
	Run(context.Context, ...string) ([]byte, error)
}

type cappedBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := maxOutputBytes - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func (r Runner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if r.Path == "" {
		return nil, fmt.Errorf("epack executable path is required")
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("epack command is required")
	}
	// #nosec G204 -- r.Path is the epack executable this Runner was constructed with, not user
	// input, and its digest is verified before use. args are this program's own subcommands.
	cmd := exec.CommandContext(ctx, r.Path, args...)
	var stdout, stderr cappedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("epack %q: %w: %s", args[0], err, stderr.String())
	}
	if stdout.truncated || stderr.truncated {
		return nil, fmt.Errorf("epack %q exceeded the %d-byte output limit", args[0], maxOutputBytes)
	}
	return stdout.Bytes(), nil
}
