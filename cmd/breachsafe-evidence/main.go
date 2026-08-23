// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/paul007ex/breachsafe-evidence-go/internal/epackcli"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: breachsafe-evidence <pack|inspect|verify|extract|unpack|diff|version>")
	}
	path := os.Getenv("BREACHSAFE_EPACK_BIN")
	if path == "" {
		return errors.New("BREACHSAFE_EPACK_BIN must name the pinned ePack executable")
	}
	runner := epackcli.Runner{Path: path}
	switch args[0] {
	case "version", "inspect", "verify", "extract", "unpack", "diff":
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
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func pack(ctx context.Context, runner epackcli.Runner, args []string) error {
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
	}
	for _, path := range []string{scan, cbom, pdf, oscal, log, request, result} {
		if path != "" {
			other = append(other, path)
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
	for _, path := range other {
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
		argv = append(argv, "--file", path+":artifacts/"+filepath.Base(path))
	}
	if _, err := runner.Run(ctx, argv...); err != nil {
		return err
	}
	if _, err := runner.Run(ctx, "inspect", "--json", tmpPack); err != nil {
		return err
	}
	if _, err := runner.Run(ctx, "verify", "--integrity-only", tmpPack); err != nil {
		return err
	}
	return os.Rename(tmpPack, output)
}
