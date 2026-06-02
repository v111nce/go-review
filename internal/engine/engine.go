package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/v111nce/go-review/internal/config"
	"github.com/v111nce/go-review/internal/pipeline"
	"github.com/v111nce/go-review/internal/report"
)

type Command string

const (
	CommandCheck Command = "check"
	CommandFix   Command = "fix"
)

type Options struct {
	Command   Command
	Config    string
	Profile   string
	Workdir   string
	ReportDir string
	Stdout    *os.File
	Stderr    *os.File
}

type ResultKind string

const (
	ResultViolation ResultKind = "violation"
	ResultArtifact  ResultKind = "artifact"
)

type Result struct {
	AdapterID    string
	StepID       string
	RuleID       string
	Kind         ResultKind
	File         string
	Line         int
	Column       int
	Message      string
	Suggestion   string
	FixAvailable bool
	FixSafety    config.FixSafety
	FixApplied   bool
	GateStatus   config.GateStatus
	Artifacts    []Artifact
	Superseded   bool
}

type Artifact struct {
	Name    string
	Path    string
	Content string
}

type RunSummary struct {
	Command         Command
	ConfigPath      string
	Profile         string
	Workdir         string
	StartedAt       time.Time
	EndedAt         time.Time
	Results         []Result
	ProgressPrinted bool
}

func (s RunSummary) GateStatus() config.GateStatus {
	status := config.GatePass
	for _, result := range s.Results {
		if result.Superseded {
			continue
		}
		switch result.GateStatus {
		case config.GateFail:
			return config.GateFail
		case config.GateWarn:
			status = config.GateWarn
		}
	}
	return status
}

func (s RunSummary) ExitCode() int {
	if s.GateStatus() == config.GateFail {
		return 1
	}
	return 0
}

type Registry struct {
	adapters map[string]AdapterFactory
}

type AdapterFactory func(config.Adapter) (Adapter, error)

type Adapter interface {
	Metadata() AdapterMetadata
	Run(context.Context, StepContext) (Result, error)
}

type AdapterMetadata struct {
	ID           string
	Type         string
	Capabilities []config.Capability
	Version      string
}

type StepContext struct {
	Command     Command
	Step        config.Step
	Adapter     config.Adapter
	Config      *config.Config
	ConfigPath  string
	ProjectRoot string
	ReportDir   string
}

func NewRegistry() *Registry {
	r := &Registry{adapters: map[string]AdapterFactory{}}
	r.Register("cmd", func(cfg config.Adapter) (Adapter, error) { return CommandAdapter{cfg: cfg}, nil })
	r.Register("go.lint", func(cfg config.Adapter) (Adapter, error) { return GoLintAdapter{cfg: cfg}, nil })
	r.Register("go.semantic", func(cfg config.Adapter) (Adapter, error) { return SemanticAdapter{cfg: cfg}, nil })
	r.Register("llm.review", func(cfg config.Adapter) (Adapter, error) { return LLMReviewAdapter{cfg: cfg}, nil })
	r.Register("llm.claude", func(cfg config.Adapter) (Adapter, error) { return ClaudeReviewAdapter{cfg: cfg}, nil })
	return r
}

func (r *Registry) Register(adapterType string, factory AdapterFactory) {
	r.adapters[adapterType] = factory
}

func (r *Registry) Resolve(cfg config.Adapter) (Adapter, error) {
	adapterType := cfg.Type
	if adapterType == "" {
		adapterType = cfg.ID
	}
	factory, ok := r.adapters[adapterType]
	if !ok {
		return nil, fmt.Errorf("adapter %q uses unsupported type %q", cfg.ID, adapterType)
	}
	return factory(cfg)
}

type Runner struct {
	Registry *Registry
}

func NewRunner() Runner {
	return Runner{Registry: NewRegistry()}
}

func (r Runner) Run(ctx context.Context, opts Options) (RunSummary, error) {
	startedAt := time.Now()
	if opts.Config == "" {
		return RunSummary{}, errors.New("--config is required")
	}
	cfg, err := config.LoadFile(opts.Config)
	if err != nil {
		return RunSummary{}, err
	}
	profile, err := cfg.Profile(opts.Profile)
	if err != nil {
		return RunSummary{}, err
	}
	steps, err := orderedProfileSteps(cfg, profile, opts.Profile)
	if err != nil {
		return RunSummary{}, err
	}
	projectRoot := opts.Workdir
	if projectRoot == "" {
		projectRoot = cfg.Defaults.Workdir
	}
	if projectRoot == "" {
		projectRoot = "."
	}
	projectRoot = resolveProjectRoot(opts.Config, projectRoot, opts.Workdir != "")
	summary := RunSummary{Command: opts.Command, ConfigPath: opts.Config, Profile: profile.Name, Workdir: projectRoot, StartedAt: startedAt, ProgressPrinted: true}
	progress := newProgressLogger(opts.Stdout)
	progress.StartProfile(summary.Profile, summary.Command)
	for _, step := range steps {
		adapterCfg, ok := cfg.Adapter(step.AdapterID)
		if !ok {
			return summary, fmt.Errorf("step %q references unknown adapter %q", step.ID, step.AdapterID)
		}
		adapter, err := r.Registry.Resolve(*adapterCfg)
		if err != nil {
			return summary, err
		}
		stepCtx := StepContext{Command: opts.Command, Step: step, Adapter: *adapterCfg, Config: cfg, ConfigPath: opts.Config, ProjectRoot: projectRoot, ReportDir: opts.ReportDir}
		progress.StartStep(step, *adapterCfg)
		if adapterCfg.Type == "llm.review" && opts.ReportDir != "" {
			if err := report.WriteFiles(opts.ReportDir, summary.Report()); err != nil {
				return summary, err
			}
		}
		result, runErr := adapter.Run(ctx, stepCtx)
		if runErr != nil && result.Message == "" {
			result = Result{AdapterID: adapterCfg.ID, StepID: step.ID, RuleID: adapterCfg.ID, Kind: ResultViolation, Message: runErr.Error(), FixSafety: adapterCfg.FixSafety, GateStatus: config.GateFail}
		}
		if result.AdapterID == "" {
			result.AdapterID = adapterCfg.ID
		}
		if result.StepID == "" {
			result.StepID = step.ID
		}
		if result.FixSafety == "" {
			result.FixSafety = adapterCfg.FixSafety
		}
		if result.Kind == "" {
			result.Kind = ResultArtifact
		}
		progress.EndStep(result)
		summary.Results = append(summary.Results, result)
		if result.GateStatus == config.GateFail && step.OnFail == config.OnFailStop {
			break
		}
	}
	if opts.Command == CommandFix && shouldRevalidateAfterLLM(summary) {
		markSupersededFailures(&summary)
		for _, step := range steps {
			adapterCfg, ok := cfg.Adapter(step.AdapterID)
			if !ok {
				return summary, fmt.Errorf("step %q references unknown adapter %q", step.ID, step.AdapterID)
			}
			if adapterCfg.Type == "llm.review" {
				continue
			}
			adapter, err := r.Registry.Resolve(*adapterCfg)
			if err != nil {
				return summary, err
			}
			stepCtx := StepContext{Command: CommandCheck, Step: step, Adapter: *adapterCfg, Config: cfg, ConfigPath: opts.Config, ProjectRoot: projectRoot, ReportDir: opts.ReportDir}
			progress.StartStep(step, *adapterCfg)
			result, runErr := adapter.Run(ctx, stepCtx)
			if runErr != nil && result.Message == "" {
				result = Result{AdapterID: adapterCfg.ID, StepID: step.ID, RuleID: adapterCfg.ID, Kind: ResultViolation, Message: runErr.Error(), FixSafety: adapterCfg.FixSafety, GateStatus: config.GateFail}
			}
			if result.AdapterID == "" {
				result.AdapterID = adapterCfg.ID
			}
			if result.StepID == "" {
				result.StepID = step.ID
			}
			if result.FixSafety == "" {
				result.FixSafety = adapterCfg.FixSafety
			}
			if result.Kind == "" {
				result.Kind = ResultArtifact
			}
			progress.EndStep(result)
			summary.Results = append(summary.Results, result)
			if result.GateStatus == config.GateFail && step.OnFail == config.OnFailStop {
				break
			}
		}
	}
	summary.EndedAt = time.Now()
	if opts.ReportDir != "" {
		if err := report.WriteFiles(opts.ReportDir, summary.Report()); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

type progressLogger struct {
	out     *os.File
	enabled bool
}

func newProgressLogger(out *os.File) progressLogger {
	if out == nil {
		out = os.Stdout
	}
	return progressLogger{out: out, enabled: true}
}

func (l progressLogger) StartProfile(profile string, command Command) {
	if !l.enabled {
		return
	}
	fmt.Fprintf(l.out, "START profile=%s command=%s\n", profile, command)
}

func (l progressLogger) StartStep(step config.Step, adapter config.Adapter) {
	if !l.enabled {
		return
	}
	fmt.Fprintf(l.out, "START step=%s adapter=%s type=%s\n", step.ID, step.AdapterID, adapter.Type)
	if adapter.Type == "llm.review" {
		fmt.Fprintf(l.out, "INFO step=%s message=%q\n", step.ID, "starting codex exec; this may take a while")
	}
	if adapter.Type == "llm.claude" {
		fmt.Fprintf(l.out, "INFO step=%s message=%q\n", step.ID, "starting claude review; this may take a while")
	}
}

func (l progressLogger) EndStep(result Result) {
	if !l.enabled {
		return
	}
	fmt.Fprintf(l.out, "END step=%s status=%s\n", result.StepID, result.GateStatus)
}

func (s RunSummary) Report() report.RunReport {
	reportSteps := make([]report.Step, 0, len(s.Results))
	findings := make([]report.Finding, 0, len(s.Results))
	artifacts := make([]report.ArtifactRef, 0)
	for _, result := range s.Results {
		if result.Superseded {
			continue
		}
		step := report.Step{
			ID:            result.StepID,
			AdapterID:     result.AdapterID,
			Status:        report.GateStatus(result.GateStatus),
			Message:       result.Message,
			FixAvailable:  result.FixAvailable,
			FixSafety:     string(result.FixSafety),
			FixApplied:    result.FixApplied,
			ArtifactPaths: artifactPaths(result.Artifacts),
		}
		if result.GateStatus == config.GateFail {
			step.FailureReason = result.Message
		}
		reportSteps = append(reportSteps, step)
		for _, artifact := range result.Artifacts {
			if artifact.Path != "" {
				artifacts = append(artifacts, report.ArtifactRef{StepID: result.StepID, Name: artifact.Name, Path: artifact.Path, Summary: artifactSummary(artifact)})
			}
		}
		if result.GateStatus == config.GatePass || result.Kind == ResultArtifact {
			continue
		}
		findings = append(findings, report.Finding{
			AdapterID:    result.AdapterID,
			StepID:       result.StepID,
			RuleID:       result.RuleID,
			Kind:         string(result.Kind),
			File:         result.File,
			Line:         result.Line,
			Column:       result.Column,
			Message:      result.Message,
			Suggestion:   result.Suggestion,
			FixAvailable: result.FixAvailable,
			FixSafety:    string(result.FixSafety),
			FixApplied:   result.FixApplied,
			GateStatus:   report.GateStatus(result.GateStatus),
		})
	}
	return report.RunReport{
		Command:    string(s.Command),
		Profile:    s.Profile,
		GateStatus: report.GateStatus(s.GateStatus()),
		Workdir:    s.Workdir,
		ConfigPath: s.ConfigPath,
		StartedAt:  s.StartedAt,
		EndedAt:    s.EndedAt,
		Steps:      reportSteps,
		Findings:   findings,
		Artifacts:  artifacts,
	}
}

func shouldRevalidateAfterLLM(summary RunSummary) bool {
	hasSupersedableFailure := false
	hasSuccessfulLLM := false
	for _, result := range summary.Results {
		if result.AdapterID == "llm.review" && result.GateStatus == config.GatePass {
			hasSuccessfulLLM = true
		}
		if result.GateStatus == config.GateFail && result.AdapterID != "llm.review" {
			hasSupersedableFailure = true
		}
	}
	return hasSuccessfulLLM && hasSupersedableFailure
}

func markSupersededFailures(summary *RunSummary) {
	for i := range summary.Results {
		if summary.Results[i].GateStatus == config.GateFail {
			summary.Results[i].Superseded = true
		}
	}
}

func artifactPaths(artifacts []Artifact) []string {
	paths := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Path != "" {
			paths = append(paths, artifact.Path)
		}
	}
	return paths
}

func artifactSummary(artifact Artifact) string {
	content := strings.TrimSpace(artifact.Content)
	switch artifact.Name {
	case "prompt":
		if content == "" {
			return "模型输入 prompt。"
		}
		return "模型输入 prompt；完整内容见产物。"
	case "stdout":
		if content == "" {
			return "无 stdout 输出。"
		}
		return truncateSingleLine(content, 240)
	case "stderr":
		if content == "" {
			return "无 stderr 输出。"
		}
		return truncateSingleLine(content, 240)
	case "process", "review":
		if content == "" {
			return "模型过程输出已写入统一过程文档。"
		}
		return truncateSingleLine(content, 240)
	default:
		if content == "" {
			return ""
		}
		return truncateSingleLine(content, 240)
	}
}

func truncateSingleLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func orderedProfileSteps(cfg *config.Config, profile *config.Profile, requested string) ([]config.Step, error) {
	graphSteps := make([]pipeline.Step, 0, len(cfg.Steps))
	for _, step := range cfg.Steps {
		graphSteps = append(graphSteps, pipeline.Step{
			ID:        step.ID,
			AdapterID: step.AdapterID,
			DependsOn: step.DependsOn,
			OnFail:    pipeline.OnFailPolicy(step.OnFail),
			AllowFix:  step.AllowFix,
			Timeout:   step.Timeout,
			Artifacts: step.Artifacts,
		})
	}
	graph, err := pipeline.NewGraph(graphSteps)
	if err != nil {
		return nil, err
	}
	profiles := make([]pipeline.Profile, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		profiles = append(profiles, pipeline.Profile{Name: p.Name, Steps: p.Steps})
	}
	selected, err := pipeline.SelectProfile(graph, profiles, requested)
	if err != nil {
		return nil, err
	}
	steps := make([]config.Step, 0, len(selected))
	for _, selectedStep := range selected {
		step, ok := cfg.Step(selectedStep.ID)
		if !ok {
			return nil, fmt.Errorf("profile %q references unknown step %q", profile.Name, selectedStep.ID)
		}
		if !step.Enabled {
			continue
		}
		steps = append(steps, *step)
	}
	return steps, nil
}

func PrintSummary(summary RunSummary, stdout *os.File) {
	if stdout == nil {
		stdout = os.Stdout
	}
	if !summary.ProgressPrinted {
		fmt.Fprintf(stdout, "START profile=%s command=%s\n", summary.Profile, summary.Command)
		for _, result := range summary.Results {
			fmt.Fprintf(stdout, "START step=%s\n", result.StepID)
			fmt.Fprintf(stdout, "END step=%s status=%s\n", result.StepID, result.GateStatus)
		}
	}
	if summary.GateStatus() == config.GatePass {
		fmt.Fprintf(stdout, "SUCCESS profile=%s\n", summary.Profile)
	} else {
		fmt.Fprintf(stdout, "FAILED profile=%s\n", summary.Profile)
		if failed := firstFailedResult(summary); failed != nil {
			fmt.Fprintf(stdout, "reason=%s\n", conciseReason(*failed))
			if artifact := firstArtifactPath(*failed); artifact != "" {
				fmt.Fprintf(stdout, "details=%s\n", artifact)
			}
		}
	}
	if reportPath := latestReportPath(summary); reportPath != "" {
		fmt.Fprintf(stdout, "report=%s\n", reportPath)
	}
}

func firstFailedResult(summary RunSummary) *Result {
	for i := range summary.Results {
		if summary.Results[i].Superseded {
			continue
		}
		if summary.Results[i].GateStatus == config.GateFail {
			return &summary.Results[i]
		}
	}
	return nil
}

func conciseReason(result Result) string {
	var b strings.Builder
	if result.StepID != "" {
		b.WriteString(result.StepID)
		b.WriteString(": ")
	}
	if result.File != "" {
		b.WriteString(formatLocation(result))
		b.WriteString(": ")
	}
	if result.RuleID != "" {
		b.WriteString(result.RuleID)
		b.WriteString(": ")
	}
	b.WriteString(result.Message)
	return b.String()
}

func firstArtifactPath(result Result) string {
	for _, artifact := range result.Artifacts {
		if artifact.Path != "" {
			return artifact.Path
		}
	}
	return ""
}

func latestReportPath(summary RunSummary) string {
	if summary.ConfigPath == "" {
		return ""
	}
	dir := filepath.Dir(summary.ConfigPath)
	if filepath.Base(dir) == ".go-review" {
		return filepath.Join(dir, "reports", "latest.md")
	}
	return filepath.Join(dir, ".go-review", "reports", "latest.md")
}

type CommandAdapter struct {
	cfg config.Adapter
}

func (a CommandAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "cmd", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

func (a CommandAdapter) Run(ctx context.Context, stepCtx StepContext) (Result, error) {
	if a.cfg.Command == "" {
		return Result{}, fmt.Errorf("cmd adapter %q missing command", a.cfg.ID)
	}
	timeout := stepCtx.Step.Timeout
	if timeout == 0 {
		timeout = a.cfg.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, a.cfg.Command, a.cfg.Args...)
	cmd.Dir = resolveWorkdir(stepCtx.ProjectRoot, a.cfg.Workdir)
	cmd.Env = mergeEnv(os.Environ(), a.cfg.Env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)
	status := config.GatePass
	message := "command passed"
	if ctx.Err() == context.DeadlineExceeded {
		status = config.GateFail
		message = fmt.Sprintf("command timed out after %s", timeout)
	} else if err != nil {
		status = config.GateFail
		message = err.Error()
	}
	artifacts := []Artifact{
		{Name: "stdout", Content: stdout.String()},
		{Name: "stderr", Content: stderr.String()},
	}
	if dir := stepCtx.Config.Artifacts.Dir; dir != "" {
		written, writeErr := writeArtifacts(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, dir), stepCtx.Step.ID, artifacts)
		if writeErr == nil {
			artifacts = written
		}
	}
	if status == config.GatePass && strings.TrimSpace(stdout.String()) != "" {
		message = firstLine(stdout.String())
	}
	if status == config.GateFail && strings.TrimSpace(stderr.String()) != "" {
		message = firstLine(stderr.String())
	}
	_ = duration
	return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: a.cfg.ID, Kind: ResultArtifact, Message: message, FixSafety: a.cfg.FixSafety, GateStatus: status, Artifacts: artifacts}, err
}

func formatLocation(result Result) string {
	location := result.File
	if result.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, result.Line)
		if result.Column > 0 {
			location = fmt.Sprintf("%s:%d", location, result.Column)
		}
	}
	return location
}

func resolveProjectRoot(configPath, projectRoot string, explicit bool) string {
	if filepath.IsAbs(projectRoot) {
		return filepath.Clean(projectRoot)
	}
	if explicit {
		return filepath.Clean(projectRoot)
	}
	base := filepath.Dir(configPath)
	return filepath.Clean(filepath.Join(base, projectRoot))
}

func resolveWorkdir(base, workdir string) string {
	if workdir == "" {
		return base
	}
	if filepath.IsAbs(workdir) {
		return workdir
	}
	return filepath.Clean(filepath.Join(base, workdir))
}

func mergeEnv(base []string, extra map[string]string) []string {
	out := append([]string{}, base...)
	for key, value := range extra {
		prefix := key + "="
		replaced := false
		for i := range out {
			if strings.HasPrefix(out[i], prefix) {
				out[i] = prefix + value
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, prefix+value)
		}
	}
	return out
}

func writeArtifacts(dir, stepID string, artifacts []Artifact) ([]Artifact, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return artifacts, err
	}
	written := make([]Artifact, len(artifacts))
	for i, artifact := range artifacts {
		artifact.Path = filepath.Join(dir, sanitize(stepID)+"-"+artifact.Name+".txt")
		if err := os.WriteFile(artifact.Path, []byte(artifact.Content), 0o644); err != nil {
			return artifacts, err
		}
		written[i] = artifact
	}
	return written, nil
}

func sanitize(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	if value == "" {
		return "step"
	}
	return value
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return value[:idx]
	}
	return value
}
