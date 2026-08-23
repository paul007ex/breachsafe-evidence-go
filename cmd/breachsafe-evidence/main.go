// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/paul007ex/breachsafe-evidence-go/internal/epackcli"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: breachsafe-evidence <pack|report|inspect|verify|extract|unpack|diff|version>")
	}
	path := os.Getenv("BREACHSAFE_EPACK_BIN")
	if path == "" {
		return errors.New("BREACHSAFE_EPACK_BIN must name the pinned ePack executable")
	}
	if !filepath.IsAbs(path) {
		return errors.New("BREACHSAFE_EPACK_BIN must be an absolute path")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("BREACHSAFE_EPACK_BIN is not an executable regular file: %s", path)
	}
	runner := epackcli.Runner{Path: path}
	if err := verifyExecutableDigest(path); err != nil {
		return err
	}
	switch args[0] {
	case "version", "inspect", "verify", "extract", "unpack", "diff":
		for _, arg := range args[1:] {
			if arg == "--force" {
				return errors.New("--force is not permitted by the BreachSAFE facade")
			}
		}
		command := args[0]
		if command == "unpack" {
			command = "extract"
		}
		out, err := runner.Run(ctx, append([]string{command}, args[1:]...)...)
		if err == nil {
			fmt.Print(string(out))
		}
		return err
	case "pack":
		return pack(ctx, runner, args[1:])
	case "report":
		return report(ctx, runner, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func report(ctx context.Context, epackRunner epackcli.CommandRunner, args []string) error {
	pdfPath := os.Getenv("BREACHSAFE_PDF_BIN")
	if pdfPath == "" {
		return errors.New("BREACHSAFE_PDF_BIN must name the approved breachsafe-pdf executable")
	}
	if !filepath.IsAbs(pdfPath) {
		return errors.New("BREACHSAFE_PDF_BIN must be an absolute path")
	}
	info, err := os.Stat(pdfPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("BREACHSAFE_PDF_BIN is not an executable regular file: %s", pdfPath)
	}
	if err := verifyExecutableDigestAt(pdfPath, "BREACHSAFE_PDF_SHA256", "/usr/share/breachsafe/breachsafe-pdf.sha256", "breachsafe-pdf"); err != nil {
		return err
	}
	return reportWithRunners(ctx, epackRunner, epackcli.Runner{Path: pdfPath}, args)
}

func reportWithRunners(ctx context.Context, epackRunner, pdfRunner epackcli.CommandRunner, args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var profile, request, scan, cbom, pdf, result, stream, output string
	fs.StringVar(&profile, "profile", "", "breachsafe-pdf report profile")
	fs.StringVar(&request, "request", "", "render request JSON")
	fs.StringVar(&scan, "scan-json", "", "QuReddy scan JSON")
	fs.StringVar(&cbom, "cbom", "", "CycloneDX CBOM JSON")
	fs.StringVar(&pdf, "pdf", "", "generated PDF output")
	fs.StringVar(&result, "result", "", "PDF RenderResult output")
	fs.StringVar(&stream, "stream", "", "ePack stream")
	fs.StringVar(&output, "output", "", "output ePack")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if profile == "" || request == "" || scan == "" || cbom == "" || pdf == "" || result == "" || stream == "" || output == "" {
		return errors.New("report requires --profile, --request, --scan-json, --cbom, --pdf, --result, --stream, and --output")
	}
	pdfReceipt, err := pdfRunner.Run(ctx, "render", "--profile", profile, "--request", request, "--cbom", cbom, "--scan-json", scan, "--pdf", pdf, "--result", result)
	if err != nil {
		return fmt.Errorf("breachsafe-pdf render: %w", err)
	}
	if len(pdfReceipt) > 0 {
		if _, err := os.Stderr.Write(pdfReceipt); err != nil {
			return err
		}
	}
	return pack(ctx, epackRunner, []string{"--stream", stream, "--request", request, "--scan", scan, "--cbom", cbom, "--pdf", pdf, "--result", result, "--output", output})
}

func pack(ctx context.Context, runner epackcli.CommandRunner, args []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var scan, cbom, pdf, oscal, log, request, result, stream, output string
	var other []string
	fs.StringVar(&scan, "scan", "", "scan artifact")
	fs.StringVar(&cbom, "cbom", "", "CBOM artifact")
	fs.StringVar(&pdf, "pdf", "", "PDF artifact")
	fs.StringVar(&oscal, "oscal", "", "OSCAL artifact")
	fs.StringVar(&log, "log", "", "log artifact")
	fs.StringVar(&request, "request", "", "request artifact")
	fs.StringVar(&result, "result", "", "result artifact")
	fs.StringVar(&stream, "stream", "", "ePack stream")
	fs.StringVar(&output, "output", "", "output ePack")
	fs.Func("other", "additional artifact (repeatable)", func(v string) error { other = append(other, v); return nil })
	if err := fs.Parse(args); err != nil {
		return err
	}
	if stream == "" || output == "" {
		return errors.New("pack requires --stream and --output")
	}
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("output already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check output %s: %w", output, err)
	}
	inputs := []struct{ path, role string }{
		{scan, "scan"}, {cbom, "cbom"}, {pdf, "pdf"}, {oscal, "oscal"},
		{log, "log"}, {request, "request"}, {result, "result"},
	}
	for _, input := range inputs {
		if input.path != "" {
			other = append(other, input.path+"\x00"+input.role)
		}
	}
	if len(other) == 0 {
		return errors.New("pack requires at least one artifact")
	}
	tmp, err := os.MkdirTemp(filepath.Dir(output), ".breachsafe-evidence-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	tmpPack := filepath.Join(tmp, filepath.Base(output))
	argv := []string{"build", tmpPack, "--stream", stream}
	expected := make(map[string]bool)
	for _, encoded := range other {
		parts := strings.SplitN(encoded, "\x00", 2)
		path, role := parts[0], "extra"
		if len(parts) == 2 {
			role = parts[1]
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("artifact %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact %s is not a regular file", path)
		}
		if info.Size() == 0 {
			return fmt.Errorf("artifact %s is empty", path)
		}
		destination := "artifacts/" + role + "/" + filepath.Base(path)
		if expected[destination] {
			return fmt.Errorf("duplicate artifact destination: %s", destination)
		}
		expected[destination] = true
		argv = append(argv, "--file", path+":"+destination)
	}
	if _, err := runner.Run(ctx, argv...); err != nil {
		return err
	}
	inspect, err := runner.Run(ctx, "inspect", "--json", tmpPack)
	if err != nil {
		return err
	}
	var receipt inspectReceipt
	if err := json.Unmarshal(inspect, &receipt); err != nil {
		return fmt.Errorf("parse ePack inspect receipt: %w", err)
	}
	if receipt.ArtifactCount != len(expected) {
		return fmt.Errorf("ePack artifact count %d does not match requested %d", receipt.ArtifactCount, len(expected))
	}
	for _, artifact := range receipt.Artifacts {
		delete(expected, artifact.Path)
	}
	if len(expected) != 0 {
		return fmt.Errorf("ePack receipt omitted requested artifacts: %s", strings.Join(sortedKeys(expected), ", "))
	}
	if _, err := runner.Run(ctx, "verify", "--integrity-only", tmpPack); err != nil {
		return err
	}
	if err := os.Link(tmpPack, output); err != nil {
		return fmt.Errorf("publish ePack without overwriting output: %w", err)
	}
	if _, err := io.WriteString(os.Stdout, string(inspect)); err != nil {
		return err
	}
	return os.Remove(tmpPack)
}

type inspectReceipt struct {
	ArtifactCount int `json:"artifact_count"`
	Artifacts     []struct {
		Path string `json:"path"`
	} `json:"artifacts"`
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func verifyExecutableDigest(path string) error {
	return verifyExecutableDigestAt(path, "BREACHSAFE_EPACK_SHA256", "/usr/share/breachsafe/epack.sha256", "ePack")
}

func verifyExecutableDigestAt(path, envName, bundledPath, label string) error {
	want := os.Getenv(envName)
	if want == "" {
		if data, err := os.ReadFile(bundledPath); err == nil {
			fields := strings.Fields(string(data))
			if len(fields) > 0 {
				want = fields[0]
			}
		}
	}
	if len(want) != 64 {
		return fmt.Errorf("%s (or bundled %s digest) is required", envName, label)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s executable: %w", label, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash %s executable: %w", label, err)
	}
	if !strings.EqualFold(want, hex.EncodeToString(h.Sum(nil))) {
		return fmt.Errorf("%s executable digest does not match the approved SHA-256", label)
	}
	return nil
}
