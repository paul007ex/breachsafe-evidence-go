// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paul007ex/breachsafe-evidence-go/internal/epackcli"
)

type fakeRunner struct{ calls [][]string }

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if args[0] == "build" {
		if err := os.WriteFile(args[1], []byte("fake epack"), 0o600); err != nil {
			return nil, err
		}
	}
	if args[0] == "inspect" {
		return json.Marshal(map[string]any{
			"artifact_count": 1,
			"artifacts":      []map[string]string{{"path": "artifacts/cbom/input.json"}},
		})
	}
	return []byte("ok\n"), nil
}

var _ epackcli.CommandRunner = (*fakeRunner)(nil)

func TestPackRejectsMissingRequiredFlags(t *testing.T) {
	if err := pack(context.Background(), &fakeRunner{}, nil); err == nil || !strings.Contains(err.Error(), "--stream") {
		t.Fatalf("expected required flag error, got %v", err)
	}
}

func TestPackPublishesAndUsesRoleDestination(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.json")
	if err := os.WriteFile(input, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "evidence.epack")
	runner := &fakeRunner{}
	if err := pack(context.Background(), runner, []string{"--stream", "test/stream", "--cbom", input, "--output", out}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output was not published: %v", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("expected build, inspect, verify; got %d calls", len(runner.calls))
	}
	if !strings.Contains(strings.Join(runner.calls[0], " "), "artifacts/cbom/input.json") {
		t.Fatalf("role destination missing: %#v", runner.calls[0])
	}
}

func TestPackRejectsExistingOutput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "existing.epack")
	if err := os.WriteFile(out, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := pack(context.Background(), &fakeRunner{}, []string{"--stream", "test", "--other", out, "--output", out})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected no-clobber error, got %v", err)
	}
}

func TestVerifyExecutableDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "epack")
	if err := os.WriteFile(path, []byte("approved"), 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("approved"))
	t.Setenv("BREACHSAFE_EPACK_SHA256", hex.EncodeToString(digest[:]))
	if err := verifyExecutableDigest(path); err != nil {
		t.Fatalf("valid digest rejected: %v", err)
	}
	t.Setenv("BREACHSAFE_EPACK_SHA256", strings.Repeat("0", 64))
	if err := verifyExecutableDigest(path); err == nil {
		t.Fatal("mismatched digest accepted")
	}
}
