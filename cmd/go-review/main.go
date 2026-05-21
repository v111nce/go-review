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
		case "init":
			return runInit(args[1:])
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
			created, initErr := initProject(*workdir)
			if initErr != nil {
				fmt.Fprintf(os.Stderr, "go-review %s: %v\n", command, err)
				return 2
			}
			fmt.Fprintf(os.Stdout, "initialized config=%s\n", created)
			discovered = created
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

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workdir := fs.String("workdir", "", "project working directory override")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	created, err := initProject(*workdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review init: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stdout, "initialized config=%s\n", created)
	return 0
}

func initProject(workdir string) (string, error) {
	base := workdir
	if base == "" {
		base = "."
	}
	if discovered, err := discoverConfig(base); err == nil {
		return discovered, nil
	}
	configPath := filepath.Join(base, ".go-review", "go-review.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, []byte(defaultConfig()), 0o644); err != nil {
		return "", err
	}
	if err := writeDefaultSemanticConfig(filepath.Dir(configPath)); err != nil {
		return "", err
	}
	return configPath, nil
}

func writeDefaultSemanticConfig(configDir string) error {
	semanticDir := filepath.Join(configDir, "semantic")
	if err := os.MkdirAll(semanticDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(semanticDir, "default.yaml"), []byte(defaultSemanticConfig()), 0o644); err != nil {
		return err
	}
	customPath := filepath.Join(semanticDir, "custom.yaml")
	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		return os.WriteFile(customPath, []byte(customSemanticConfig()), 0o644)
	} else if err != nil {
		return err
	}
	return nil
}

func defaultConfig() string {
	return `schema_version: "1.0"
tools:
  go_review: "generated"
  adapters:
    go.format: "system-gofmt"
    go.test: "go"
    go.semantic: "builtin"
defaults:
  timeout: 30s
  workdir: ..
artifacts:
  dir: ".go-review/artifacts/latest"
adapters:
  - id: go.format
    type: go.format
    capabilities: [check, fix]
    timeout: 30s
    fix_safety: safe
  - id: go.test
    type: cmd
    command: go
    args: [test, ./...]
    capabilities: [test]
    timeout: 2m
    parser: go-test
  - id: go.semantic
    type: go.semantic
    capabilities: [check]
    timeout: 30s
    fix_safety: review
steps:
  - id: format-check
    adapter: go.format
    on_fail: stop
    allow_fix: true
  - id: test
    adapter: go.test
    depends_on: [format-check]
    on_fail: stop
  - id: semantic
    adapter: go.semantic
    depends_on: [test]
    on_fail: stop
profiles:
  - name: fast
    steps: [format-check, test]
  - name: ci
    steps: [format-check, test, semantic]
  - name: nightly
    steps: [format-check, test, semantic]
`
}

func defaultSemanticConfig() string {
	return `rules:
  - no-direct-os-getenv
exclude:
  - vendor
  - testdata
`
}

func customSemanticConfig() string {
	return `rules:
exclude:
`
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
  go-review init [--workdir <dir>]
  go-review version

Commands:
  check    run configured adapters without applying edits (default; initializes missing config)
  fix      run configured adapters in fix mode when adapters support safe fixes such as gofmt
  init     create .go-review/go-review.yaml without running checks
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
