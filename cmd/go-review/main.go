package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/v111nce/go-review/internal/engine"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h", "help":
			printHelp()
			return 0
		case "version", "--version", "-v":
			printVersion()
			return 0
		case "check", "fix":
			return runCommand(args[0], args[1:])
		}
		if len(args[0]) > 0 && args[0][0] != '-' {
			fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
			printHelp()
			return 2
		}
	}
	return runCommand("check", args)
}

func runCommand(command string, args []string) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to go-review YAML config")
	profile := fs.String("profile", "fast", "profile to run")
	workdir := fs.String("workdir", "", "project working directory override")
	reportDir := fs.String("report-dir", "", "directory for latest.md, latest.llm.md, and latest.json reports")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	resolvedConfigPath := *configPath
	if resolvedConfigPath == "" {
		discovered, err := discoverConfig(*workdir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "go-review %s: %v\n", command, err)
			return 2
		}
		resolvedConfigPath = discovered
	}
	resolvedReportDir := *reportDir
	if resolvedReportDir == "" {
		resolvedReportDir = defaultReportDir(resolvedConfigPath)
	}
	summary, err := engine.NewRunner().Run(context.Background(), engine.Options{Command: engine.Command(command), Config: resolvedConfigPath, Profile: *profile, Workdir: *workdir, ReportDir: resolvedReportDir, Stdout: os.Stdout, Stderr: os.Stderr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review %s: %v\n", command, err)
		return 2
	}
	engine.PrintSummary(summary, os.Stdout)
	return summary.ExitCode()
}

func discoverConfig(workdir string) (string, error) {
	base := workdir
	if base == "" {
		base = "."
	}
	candidates := []string{
		filepath.Join(base, ".go-review", "go-review.yaml"),
		filepath.Join(base, "go-review.yaml"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no config found; expected .go-review/go-review.yaml or go-review.yaml under %s, or pass --config", base)
}

func defaultReportDir(configPath string) string {
	if configPath == "" {
		return ""
	}
	dir := filepath.Dir(configPath)
	if filepath.Base(dir) == ".go-review" {
		return filepath.Join(dir, "reports")
	}
	return filepath.Join(dir, ".go-review", "reports")
}

func printHelp() {
	fmt.Fprintln(os.Stdout, `go-review runs configured Go code-review quality gates.

Usage:
  go-review [check] [--config <path>] [--profile fast]
  go-review fix [--config <path>] [--profile fast]
  go-review version

Commands:
  check    run configured adapters without applying edits (default)
  fix      run configured adapters in fix mode when adapters support safe fixes such as gofmt
  version  print build version metadata

Flags:
  --config     path to go-review YAML config; defaults to .go-review/go-review.yaml or go-review.yaml
  --profile    profile name, defaults to fast
  --workdir    project working directory override
  --report-dir directory for latest.md, latest.llm.md, and latest.json reports`)
}

func printVersion() {
	versionValue, commitValue, dateValue := buildMetadata()
	fmt.Fprintf(os.Stdout, "go-review version=%s commit=%s date=%s go=%s os=%s arch=%s\n", versionValue, commitValue, dateValue, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func buildMetadata() (string, string, string) {
	versionValue := version
	commitValue := commit
	dateValue := date

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return versionValue, commitValue, dateValue
	}

	if versionValue == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		versionValue = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if commitValue == "unknown" && setting.Value != "" {
				commitValue = setting.Value
			}
		case "vcs.time":
			if dateValue == "unknown" && setting.Value != "" {
				dateValue = setting.Value
			}
		}
	}

	return versionValue, commitValue, dateValue
}
