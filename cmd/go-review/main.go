package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/v111nce/go-review/internal/engine"
	"github.com/v111nce/go-review/internal/rulecatalog"
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
		case "rules":
			return runRules(args[1:])
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
	if err := writeDefaultRuleCatalog(filepath.Dir(configPath)); err != nil {
		return "", err
	}
	if err := writeDefaultSemanticConfig(filepath.Dir(configPath)); err != nil {
		return "", err
	}
	return configPath, nil
}

func writeDefaultRuleCatalog(configDir string) error {
	path := filepath.Join(configDir, "rules.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return rulecatalog.SaveFile(path, defaultRuleCatalog())
}

func defaultRuleCatalog() rulecatalog.Catalog {
	return rulecatalog.Catalog{
		SchemaVersion: rulecatalog.SchemaVersion,
		Rules: []rulecatalog.Rule{
			{
				ID:             "go.official.gofmt",
				Title:          "Go 代码格式化",
				Description:    "代码必须使用 gofmt 统一格式化，避免人工风格争论。",
				Source:         rulecatalog.Source{Name: "Go Code Review Comments", URL: "https://go.dev/wiki/CodeReviewComments", Section: "Gofmt"},
				Handling:       "tool-golangci",
				Adapter:        "go.lint",
				ToolRules:      []string{"gofmt"},
				DefaultProfile: "default",
				Severity:       "error",
				Autofix:        rulecatalog.Autofix{Supported: true, Safety: "safe"},
				Status:         "active",
				Implemented:    true,
				Notes:          "机械格式化，可 safe fix；报告 rule_id 输出 go.official.gofmt。",
			},
			{
				ID:             "go.official.imports",
				Title:          "Import 整理",
				Description:    "import 应自动整理、删除未用项并按 Go 习惯分组。",
				Source:         rulecatalog.Source{Name: "Go Code Review Comments", URL: "https://go.dev/wiki/CodeReviewComments", Section: "Imports"},
				Handling:       "tool-golangci",
				Adapter:        "go.lint",
				ToolRules:      []string{"goimports", "gci"},
				DefaultProfile: "default",
				Severity:       "error",
				Autofix:        rulecatalog.Autofix{Supported: true, Safety: "safe"},
				Status:         "active",
				Implemented:    true,
				Notes:          "配置 goimports/gci formatter 时，报告 rule_id 输出 go.official.imports。",
			},
			{
				ID:             "team.semantic.max-params",
				Title:          "函数参数上限",
				Description:    "函数/方法入参个数不得超过配置阈值。",
				Source:         rulecatalog.Source{Name: "Team semantic rules", Section: "max-params"},
				Handling:       "tool-semantic",
				Adapter:        "go.semantic",
				ToolRules:      []string{"max-params"},
				DefaultProfile: "strict",
				Severity:       "medium",
				Autofix:        rulecatalog.Autofix{Supported: false, Safety: "none"},
				Status:         "active",
				Implemented:    true,
				Notes:          "确定性 AST 子规则；配置使用该 catalog id 时，报告 rule_id 保持一致。",
			},
		},
	}
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
    go.lint: "system-golangci-lint"       # install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
    go.test: "go"
    go.semantic: "builtin"
    # Optional adapters. Uncomment after installing the named tools.
    # go.vet: "go"                       # built in with Go; uncomment the adapter/step/profile entries below to enable.
    # staticcheck: "staticcheck"          # install: go install honnef.co/go/tools/cmd/staticcheck@latest
    # govulncheck: "govulncheck"          # install: go install golang.org/x/vuln/cmd/govulncheck@latest
    # gosec: "gosec"                      # install: go install github.com/securego/gosec/v2/cmd/gosec@latest
defaults:
  timeout: 30s
  workdir: ..
# Project-owned excludes. Add entries only when this repository intentionally ignores them.
# exclude:
#   - generated
#   - third_party
artifacts:
  dir: ".go-review/artifacts/latest"
adapters:
  - id: go.lint.format
    type: go.lint
    args: [fmt, --no-config, --enable, gofmt, --enable, goimports]
    capabilities: [check, fix]
    timeout: 30s
    fix_safety: safe
  - id: go.lint.static
    type: go.lint
    args:
      - run
      - --no-config
      - --enable-only=errcheck,govet,staticcheck,unused,ineffassign,revive,thelper,errorlint,errname,forcetypeassert,predeclared,bodyclose,perfsprint,gocritic,godot,godoclint,nonamedreturns,nakedret,testifylint,importas,forbidigo,varnamelen,gochecknoglobals,gochecknoinits,paralleltest,makezero,prealloc,gosec
      - --output.text.print-issued-lines=false
      - --show-stats=false
    capabilities: [check]
    timeout: 2m
    fix_safety: none
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
  # Optional: go vet (no extra install; ships with Go).
  # - id: go.vet
  #   type: cmd
  #   command: go
  #   args: [vet, ./...]
  #   capabilities: [check]
  #   timeout: 2m
  #   parser: text
  # Optional: staticcheck. Install first:
  #   go install honnef.co/go/tools/cmd/staticcheck@latest
  # - id: staticcheck
  #   type: cmd
  #   command: staticcheck
  #   args: [./...]
  #   capabilities: [check]
  #   timeout: 2m
  #   parser: text
  # Optional: govulncheck. Install first:
  #   go install golang.org/x/vuln/cmd/govulncheck@latest
  # - id: govulncheck
  #   type: cmd
  #   command: govulncheck
  #   args: [./...]
  #   capabilities: [scan]
  #   timeout: 5m
  #   parser: text
  # Optional: gosec. Install first:
  #   go install github.com/securego/gosec/v2/cmd/gosec@latest
  # - id: gosec
  #   type: cmd
  #   command: gosec
  #   args: [./...]
  #   capabilities: [scan]
  #   timeout: 5m
  #   parser: text
steps:
  - id: format-check
    adapter: go.lint.format
    on_fail: continue
    allow_fix: true
  - id: lint
    adapter: go.lint.static
    on_fail: continue
  - id: test
    adapter: go.test
    on_fail: continue
  - id: semantic
    adapter: go.semantic
    on_fail: continue
  # Optional steps. Uncomment the matching adapter above before enabling.
  # Each step uses on_fail: continue so one review area does not prevent others from running.
  # - id: vet
  #   adapter: go.vet
  #   on_fail: continue
  # - id: staticcheck
  #   adapter: staticcheck
  #   on_fail: continue
  # - id: vulncheck
  #   adapter: govulncheck
  #   on_fail: continue
  # - id: security
  #   adapter: gosec
  #   on_fail: continue
profiles:
  - name: fast
    steps: [format-check, test]
  - name: ci
    steps: [format-check, lint, test, semantic]
  - name: nightly
    steps: [format-check, lint, test, semantic]
  # Optional after enabling the matching steps above:
  # - name: full
  #   steps: [format-check, test, semantic, vet, staticcheck, vulncheck, security]
`
}

func defaultSemanticConfig() string {
	return `# Framework-owned semantic rules live here.
# This file's rules: entries are built-in rule names registered by go.semantic.
rules:
  - import-blank
  - custom-contexts
  - no-tfatal-goroutine
  - channel-size
  - enum-start-one
  - exit-in-main
  - no-direct-os-getenv
`
}

func customSemanticConfig() string {
	return `# Team-owned semantic rules live here.
# This file's rules: entries are rule objects supported by go.semantic.
# Currently supported custom kinds:
# - no-direct-call: reports calls to an imported package function, including aliased imports.
# - max-params: reports functions/methods whose parameter count is greater than max.
# Built-in semantic/default.yaml supports: import-blank, custom-contexts, no-tfatal-goroutine,
# channel-size, enum-start-one, exit-in-main, and no-direct-os-getenv.
# Example: ban direct fmt.Println and require injected logging instead.
rules:
#   - id: no-direct-fmt-println
#     kind: no-direct-call
#     package: fmt
#     function: Println
#     message: "不要直接使用 fmt.Println"
#     suggestion: "改用注入的 logger"
#   - id: team.semantic.max-params
#     kind: max-params
#     max: 4
#     message: "方法入参不能超过 4 个"
#     suggestion: "拆分参数对象或引入配置结构"
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

func runRules(args []string) int {
	if len(args) == 0 {
		printRulesHelp()
		return 0
	}
	switch args[0] {
	case "list":
		return runRulesList(args[1:])
	case "get":
		return runRulesGet(args[1:])
	case "add":
		return runRulesAdd(args[1:], false)
	case "upsert":
		return runRulesAdd(args[1:], true)
	case "delete", "rm":
		return runRulesDelete(args[1:])
	case "validate":
		return runRulesValidate(args[1:])
	case "render-doc":
		return runRulesRenderDoc(args[1:])
	case "--help", "-h", "help":
		printRulesHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown rules command %q\n\n", args[0])
		printRulesHelp()
		return 2
	}
}

func runRulesList(args []string) int {
	fs := rulesFlagSet("rules list")
	catalogPath := fs.String("catalog", defaultRulesCatalogPath(), "path to rules JSON catalog")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	catalog, err := loadRulesCatalog(*catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules list: %v\n", err)
		return 2
	}
	for _, rule := range catalog.Rules {
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\timplemented=%t\t%s\n", rule.ID, rule.Handling, emptyValue(rule.DefaultProfile), rule.Implemented, rule.Description)
	}
	return 0
}

func runRulesGet(args []string) int {
	fs := rulesFlagSet("rules get")
	catalogPath := fs.String("catalog", defaultRulesCatalogPath(), "path to rules JSON catalog")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "go-review rules get: expected rule id")
		return 2
	}
	catalog, err := loadRulesCatalog(*catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules get: %v\n", err)
		return 2
	}
	rule, ok := catalog.Get(fs.Arg(0))
	if !ok {
		fmt.Fprintf(os.Stderr, "go-review rules get: rule %q not found\n", fs.Arg(0))
		return 1
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(rule); err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules get: %v\n", err)
		return 2
	}
	return 0
}

func runRulesAdd(args []string, upsert bool) int {
	name := "add"
	if upsert {
		name = "upsert"
	}
	fs := rulesFlagSet("rules " + name)
	catalogPath := fs.String("catalog", defaultRulesCatalogPath(), "path to rules JSON catalog")
	filePath := fs.String("file", "", "path to JSON rule object or catalog")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rules, err := readRulesInput(*filePath, fs.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules %s: %v\n", name, err)
		return 2
	}
	catalog, err := loadRulesCatalogOrEmpty(*catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules %s: %v\n", name, err)
		return 2
	}
	for _, rule := range rules {
		if upsert {
			err = catalog.Upsert(rule)
		} else {
			err = catalog.Add(rule)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "go-review rules %s: %v\n", name, err)
			return 2
		}
	}
	if err := rulecatalog.SaveFile(*catalogPath, catalog); err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules %s: %v\n", name, err)
		return 2
	}
	fmt.Fprintf(os.Stdout, "%s %d rule(s) catalog=%s\n", name, len(rules), *catalogPath)
	return 0
}

func runRulesDelete(args []string) int {
	fs := rulesFlagSet("rules delete")
	catalogPath := fs.String("catalog", defaultRulesCatalogPath(), "path to rules JSON catalog")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "go-review rules delete: expected rule id")
		return 2
	}
	catalog, err := loadRulesCatalog(*catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules delete: %v\n", err)
		return 2
	}
	if err := catalog.Delete(fs.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules delete: %v\n", err)
		return 1
	}
	if err := rulecatalog.SaveFile(*catalogPath, catalog); err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules delete: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stdout, "deleted %s catalog=%s\n", fs.Arg(0), *catalogPath)
	return 0
}

func runRulesValidate(args []string) int {
	fs := rulesFlagSet("rules validate")
	catalogPath := fs.String("catalog", defaultRulesCatalogPath(), "path to rules JSON catalog")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	catalog, err := loadRulesCatalog(*catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules validate: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stdout, "valid catalog=%s rules=%d\n", *catalogPath, len(catalog.Rules))
	return 0
}

func runRulesRenderDoc(args []string) int {
	fs := rulesFlagSet("rules render-doc")
	catalogPath := fs.String("catalog", defaultRulesCatalogPath(), "path to rules JSON catalog")
	outPath := fs.String("out", "", "path to write Markdown; defaults to stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	catalog, err := loadRulesCatalog(*catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules render-doc: %v\n", err)
		return 2
	}
	if strings.TrimSpace(*outPath) == "" {
		if err := rulecatalog.RenderMarkdown(os.Stdout, catalog); err != nil {
			fmt.Fprintf(os.Stderr, "go-review rules render-doc: %v\n", err)
			return 2
		}
		return 0
	}
	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules render-doc: %v\n", err)
		return 2
	}
	defer f.Close()
	if err := rulecatalog.RenderMarkdown(f, catalog); err != nil {
		fmt.Fprintf(os.Stderr, "go-review rules render-doc: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stdout, "rendered %s\n", *outPath)
	return 0
}

func rulesFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func defaultRulesCatalogPath() string {
	return filepath.Join("rules", "go-rules.json")
}

func loadRulesCatalog(path string) (rulecatalog.Catalog, error) {
	return rulecatalog.LoadFile(path)
}

func loadRulesCatalogOrEmpty(path string) (rulecatalog.Catalog, error) {
	catalog, err := rulecatalog.LoadFile(path)
	if os.IsNotExist(err) {
		return rulecatalog.Empty(), nil
	}
	return catalog, err
}

func readRulesInput(filePath string, args []string) ([]rulecatalog.Rule, error) {
	if strings.TrimSpace(filePath) != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		return parseRulesJSON(data)
	}
	if len(args) == 0 {
		return nil, errors.New("expected --file or inline JSON rule")
	}
	return parseRulesJSON([]byte(strings.Join(args, " ")))
}

func parseRulesJSON(data []byte) ([]rulecatalog.Rule, error) {
	var catalog rulecatalog.Catalog
	if err := json.Unmarshal(data, &catalog); err == nil && len(catalog.Rules) > 0 {
		catalog.Normalize()
		if err := catalog.Validate(); err != nil {
			return nil, err
		}
		return catalog.Rules, nil
	}
	var rule rulecatalog.Rule
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, err
	}
	rule.Normalize()
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	return []rulecatalog.Rule{rule}, nil
}

func emptyValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func printRulesHelp() {
	fmt.Fprintln(os.Stdout, `go-review rules manages the JSON rule catalog.

Usage:
  go-review rules list [--catalog rules/go-rules.json]
  go-review rules get <rule-id> [--catalog rules/go-rules.json]
  go-review rules add --file <rule.json> [--catalog rules/go-rules.json]
  go-review rules upsert --file <rule-or-catalog.json> [--catalog rules/go-rules.json]
  go-review rules delete <rule-id> [--catalog rules/go-rules.json]
  go-review rules validate [--catalog rules/go-rules.json]
  go-review rules render-doc [--catalog rules/go-rules.json] [--out docs/quality/go-rule-catalog.md]

Catalog JSON is the source of truth; Markdown docs are generated views.`)
}

func printHelp() {
	fmt.Fprintln(os.Stdout, `go-review runs configured Go code-review quality gates.

Usage:
  go-review [check] [--config <path>] [--profile fast]
  go-review fix [--config <path>] [--profile fast]
  go-review init [--workdir <dir>]
  go-review rules <list|get|add|upsert|delete|validate|render-doc>
  go-review version

Commands:
  check    run configured adapters without applying edits (default; initializes missing config)
  fix      run configured adapters in fix mode when adapters support safe fixes such as golangci-lint fmt
  init     create .go-review/go-review.yaml without running checks
  rules    manage JSON rule catalog and render Markdown docs
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
