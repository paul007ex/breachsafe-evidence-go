// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package main

import (
	"archive/zip"
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
		return errors.New("usage: breachsafe-evidence <render|pack|report|inspect|verify|extract|unpack|diff|version>")
	}
	if args[0] == "render" {
		return render(ctx, args[1:])
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

func render(ctx context.Context, args []string) error {
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
	return renderWithRunner(ctx, epackcli.Runner{Path: pdfPath}, args)
}

func renderWithRunner(ctx context.Context, pdfRunner epackcli.CommandRunner, args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var profile, request, scan, cbom, pdf, result, log, archive string
	fs.StringVar(&profile, "profile", "", "breachsafe-pdf report profile")
	fs.StringVar(&request, "request", "", "render request JSON")
	fs.StringVar(&scan, "scan-json", "", "QuReddy scan JSON")
	fs.StringVar(&cbom, "cbom", "", "CycloneDX CBOM JSON")
	fs.StringVar(&pdf, "pdf", "", "generated PDF output")
	fs.StringVar(&result, "result", "", "PDF RenderResult output")
	fs.StringVar(&log, "log", "", "optional existing raw log artifact")
	fs.StringVar(&archive, "zip", "", "ordinary OSS ZIP output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if profile == "" || request == "" || scan == "" || cbom == "" || pdf == "" || result == "" || archive == "" {
		return errors.New("render requires --profile, --request, --scan-json, --cbom, --pdf, --result, and --zip")
	}
	for _, path := range []string{request, scan, cbom} {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("render input %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("render input is not a regular file: %s", path)
		}
	}
	if log != "" {
		info, err := os.Lstat(log)
		if err != nil {
			return fmt.Errorf("render log %s: %w", log, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("render log is not a regular file: %s", log)
		}
	}
	for _, path := range []string{pdf, result, archive} {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("output already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check output %s: %w", path, err)
		}
	}
	tempArchive, err := temporaryOutputPath(archive)
	if err != nil {
		return err
	}
	completed := false
	defer func() {
		_ = os.Remove(tempArchive)
		if !completed {
			_ = os.Remove(pdf)
			_ = os.Remove(result)
		}
	}()
	receipt, err := pdfRunner.Run(ctx, "render", "--profile", profile, "--request", request, "--cbom", cbom, "--scan-json", scan, "--pdf", pdf, "--result", result)
	if err != nil {
		return fmt.Errorf("breachsafe-pdf render: %w", err)
	}
	if err := writeOSSZip(tempArchive, request, cbom, scan, pdf, result, log); err != nil {
		return err
	}
	if err := os.Rename(tempArchive, archive); err != nil {
		return fmt.Errorf("publish %s: %w", archive, err)
	}
	completed = true
	if _, err := os.Stdout.Write(receipt); err != nil {
		return err
	}
	return nil
}

func temporaryOutputPath(path string) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create temporary output for %s: %w", path, err)
	}
	temporary := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return "", fmt.Errorf("close temporary output for %s: %w", path, err)
	}
	if err := os.Remove(temporary); err != nil {
		return "", fmt.Errorf("remove temporary output for %s: %w", path, err)
	}
	return temporary, nil
}

func writeOSSZip(output string, request, cbom, scan, pdf, result, log string) error {
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create OSS ZIP: %w", err)
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	inputs := []struct{ path, name string }{
		{request, "request.json"}, {cbom, "scan.cdx.json"}, {scan, "scan.json"},
		{pdf, "report.pdf"}, {result, "report.result.json"},
	}
	if log != "" {
		inputs = append(inputs, struct{ path, name string }{log, "raw.log"})
	}
	for _, input := range inputs {
		if err := addZipFile(archive, input.path, input.name); err != nil {
			return errors.Join(err, archive.Close(), file.Close())
		}
	}
	if err := archive.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("close OSS ZIP: %w", err)
	}
	return file.Close()
}

func addZipFile(archive *zip.Writer, source, name string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("read ZIP artifact %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("ZIP artifact is not a regular file: %s", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open ZIP artifact %s: %w", source, err)
	}
	dest, err := archive.Create(name)
	if err != nil {
		return fmt.Errorf("create ZIP entry %s: %w", name, err)
	}
	if _, err := io.Copy(dest, input); err != nil {
		_ = input.Close()
		return fmt.Errorf("write ZIP entry %s: %w", name, err)
	}
	if err := input.Close(); err != nil {
		return fmt.Errorf("close ZIP artifact %s: %w", source, err)
	}
	return nil
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
	for index, encoded := range other {
		parts := strings.SplitN(encoded, "\x00", 2)
		path, role := parts[0], "extra"
		if len(parts) == 2 {
			role = parts[1]
		}
		stagedPath, err := stageArtifact(tmp, index, path)
		if err != nil {
			return err
		}
		destination := "artifacts/" + role + "/" + filepath.Base(path)
		if expected[destination] {
			return fmt.Errorf("duplicate artifact destination: %s", destination)
		}
		expected[destination] = true
		argv = append(argv, "--file", stagedPath+":"+destination)
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
	if _, err := os.Stdout.Write(inspect); err != nil {
		return err
	}
	return os.Remove(tmpPack)
}

// stageArtifact snapshots an artifact before handing it to the external packer.
// Lstat rejects symlinks, while the open descriptor ensures the bytes copied are
// the bytes validated even if the path is replaced concurrently.
func stageArtifact(tmp string, index int, path string) (string, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("artifact %s: %w", path, err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return "", fmt.Errorf("artifact %s is not a regular file", path)
	}
	if before.Size() == 0 {
		return "", fmt.Errorf("artifact %s is empty", path)
	}
	source, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open artifact %s: %w", path, err)
	}
	defer source.Close()
	opened, err := source.Stat()
	if err != nil {
		return "", fmt.Errorf("stat opened artifact %s: %w", path, err)
	}
	if !opened.Mode().IsRegular() || opened.Size() != before.Size() {
		return "", fmt.Errorf("artifact %s changed while opening", path)
	}
	after, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("recheck artifact %s: %w", path, err)
	}
	if after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || after.Size() != before.Size() {
		return "", fmt.Errorf("artifact %s changed while staging", path)
	}
	sources := filepath.Join(tmp, "sources")
	if err := os.MkdirAll(sources, 0o700); err != nil {
		return "", fmt.Errorf("create artifact staging directory: %w", err)
	}
	staged := filepath.Join(sources, fmt.Sprintf("%06d-%s", index, filepath.Base(path)))
	destination, err := os.OpenFile(staged, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create staged artifact: %w", err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		destination.Close()
		return "", fmt.Errorf("stage artifact %s: %w", path, err)
	}
	if err := destination.Close(); err != nil {
		return "", fmt.Errorf("close staged artifact: %w", err)
	}
	return staged, nil
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
