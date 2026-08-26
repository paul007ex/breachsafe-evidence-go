// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package main

import (
	"archive/zip"
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

type fakeRunner struct {
	calls         [][]string
	artifactCount int
	artifactPaths []string
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if args[0] == "render" {
		for index, arg := range args {
			if (arg == "--pdf" || arg == "--result") && index+1 < len(args) {
				if err := os.WriteFile(args[index+1], []byte(arg), 0o600); err != nil {
					return nil, err
				}
			}
		}
	}
	if args[0] == "build" {
		if err := os.WriteFile(args[1], []byte("fake epack"), 0o600); err != nil {
			return nil, err
		}
	}
	if args[0] == "inspect" {
		count := f.artifactCount
		if count == 0 {
			count = 1
		}
		paths := f.artifactPaths
		if len(paths) == 0 {
			paths = []string{"artifacts/cbom/input.json"}
		}
		return json.Marshal(map[string]any{
			"artifact_count": count,
			"artifacts": func() []map[string]string {
				result := make([]map[string]string, 0, len(paths))
				for _, path := range paths {
					result = append(result, map[string]string{"path": path})
				}
				return result
			}(),
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

func TestPackRejectsSymlinkArtifact(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "real.json")
	linkPath := filepath.Join(dir, "link.json")
	if err := os.WriteFile(realPath, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}
	err := pack(context.Background(), &fakeRunner{}, []string{"--stream", "test", "--other", linkPath, "--output", filepath.Join(dir, "out.epack")})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected symlink rejection, got %v", err)
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

func TestReportRendersThenPacks(t *testing.T) {
	dir := t.TempDir()
	paths := map[string]string{
		"request": filepath.Join(dir, "request.json"), "scan": filepath.Join(dir, "scan.json"),
		"cbom": filepath.Join(dir, "cbom.json"), "pdf": filepath.Join(dir, "report.pdf"),
		"result": filepath.Join(dir, "report.result.json"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("input"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "evidence.epack")
	epack := &fakeRunner{artifactCount: 5, artifactPaths: []string{
		"artifacts/cbom/cbom.json", "artifacts/pdf/report.pdf", "artifacts/request/request.json",
		"artifacts/result/report.result.json", "artifacts/scan/scan.json",
	}}
	pdf := &fakeRunner{}
	args := []string{"--profile", "breachsafe/community", "--request", paths["request"], "--scan-json", paths["scan"], "--cbom", paths["cbom"], "--pdf", paths["pdf"], "--result", paths["result"], "--stream", "breachsafe/test", "--output", out}
	if err := reportWithRunners(context.Background(), epack, pdf, args); err != nil {
		t.Fatal(err)
	}
	if len(pdf.calls) != 1 || pdf.calls[0][0] != "render" {
		t.Fatalf("expected one PDF render call, got %#v", pdf.calls)
	}
	if len(epack.calls) != 3 {
		t.Fatalf("expected build/inspect/verify, got %#v", epack.calls)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("ePack output missing: %v", err)
	}
}

func TestRenderCreatesOrdinaryZipWithoutEpack(t *testing.T) {
	dir := t.TempDir()
	paths := map[string]string{
		"request": filepath.Join(dir, "request.json"), "scan": filepath.Join(dir, "scan.json"),
		"cbom": filepath.Join(dir, "cbom.json"), "pdf": filepath.Join(dir, "report.pdf"),
		"result": filepath.Join(dir, "report.result.json"), "log": filepath.Join(dir, "raw.log"),
	}
	for _, name := range []string{"request", "scan", "cbom"} {
		path := paths[name]
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "report.zip")
	runner := &fakeRunner{}
	args := []string{"--profile", "breachsafe/community", "--request", paths["request"], "--scan-json", paths["scan"], "--cbom", paths["cbom"], "--pdf", paths["pdf"], "--result", paths["result"], "--log", paths["log"], "--zip", out}
	if err := renderWithRunner(context.Background(), runner, args); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0][0] != "render" {
		t.Fatalf("expected one PDF render call, got %#v", runner.calls)
	}
	archive, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = archive.Close() }()
	want := map[string]bool{"request.json": true, "scan.cdx.json": true, "scan.json": true, "report.pdf": true, "report.result.json": true, "raw.log": true}
	for _, entry := range archive.File {
		delete(want, entry.Name)
	}
	if len(want) != 0 {
		t.Fatalf("ZIP missing entries: %v", want)
	}
}

func TestRenderWritesLogOutput(t *testing.T) {
	dir := t.TempDir()
	paths := map[string]string{
		"request": filepath.Join(dir, "request.json"), "scan": filepath.Join(dir, "scan.json"),
		"cbom": filepath.Join(dir, "cbom.json"), "pdf": filepath.Join(dir, "report.pdf"),
		"result": filepath.Join(dir, "report.result.json"), "zip": filepath.Join(dir, "report.zip"),
	}
	for _, name := range []string{"request", "scan", "cbom"} {
		path := paths[name]
		if err := os.WriteFile(path, []byte("input"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runner := &fakeRunner{}
	err := renderWithRunner(context.Background(), runner, []string{
		"--profile", "breachsafe/community", "--request", paths["request"],
		"--scan-json", paths["scan"], "--cbom", paths["cbom"], "--pdf", paths["pdf"],
		"--result", paths["result"], "--log", filepath.Join(dir, "raw.log"), "--zip", paths["zip"],
	})
	if err != nil {
		t.Fatalf("expected log output success, got %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one PDF render call, got %#v", runner.calls)
	}
	log, err := os.ReadFile(filepath.Join(dir, "raw.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(log) != "ok\n" {
		t.Fatalf("unexpected render log: %q", log)
	}
}
