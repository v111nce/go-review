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

	"go-code-reviewer/internal/config"
)

type Command string

const (
	CommandCheck Command = "check"
	CommandFix   Command = "fix"
)

type Options struct {
	Command Command
	Config  string
	Profile string
	Workdir string
	Stdout  *os.File
	Stderr  *os.File
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
	Message      string
	FixAvailable bool
	FixSafety    config.FixSafety
	GateStatus   config.GateStatus
	Artifacts    []Artifact
}

type Artifact struct {
	Name    string
	Path    string
	Content string
}

type RunSummary struct {
	Profile string
	Results []Result
}

func (s RunSummary) GateStatus() config.GateStatus {
	status := config.GatePass
	for _, result := range s.Results {
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
	ProjectRoot string
}

func NewRegistry() *Registry {
	r := &Registry{adapters: map[string]AdapterFactory{}}
	r.Register("cmd", func(cfg config.Adapter) (Adapter, error) { return CommandAdapter{cfg: cfg}, nil })
	r.Register("go.format", func(cfg config.Adapter) (Adapter, error) { return GoFormatAdapter{cfg: cfg}, nil })
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
	projectRoot := opts.Workdir
	if projectRoot == "" {
		projectRoot = cfg.Defaults.Workdir
	}
	if projectRoot == "" {
		projectRoot = "."
	}
	projectRoot = resolveProjectRoot(opts.Config, projectRoot, opts.Workdir != "")
	summary := RunSummary{Profile: profile.Name}
	for _, stepID := range profile.Steps {
		step, ok := cfg.Step(stepID)
		if !ok {
			return summary, fmt.Errorf("profile %q references unknown step %q", profile.Name, stepID)
		}
		adapterCfg, ok := cfg.Adapter(step.AdapterID)
		if !ok {
			return summary, fmt.Errorf("step %q references unknown adapter %q", step.ID, step.AdapterID)
		}
		adapter, err := r.Registry.Resolve(*adapterCfg)
		if err != nil {
			return summary, err
		}
		stepCtx := StepContext{Command: opts.Command, Step: *step, Adapter: *adapterCfg, Config: cfg, ProjectRoot: projectRoot}
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
		summary.Results = append(summary.Results, result)
		if result.GateStatus == config.GateFail && step.OnFail == config.OnFailStop {
			break
		}
	}
	return summary, nil
}

func PrintSummary(summary RunSummary, stdout *os.File) {
	if stdout == nil {
		stdout = os.Stdout
	}
	fmt.Fprintf(stdout, "profile=%s gate=%s\n", summary.Profile, summary.GateStatus())
	for _, result := range summary.Results {
		fmt.Fprintf(stdout, "step=%s adapter=%s gate=%s message=%s\n", result.StepID, result.AdapterID, result.GateStatus, result.Message)
		for _, artifact := range result.Artifacts {
			if artifact.Path != "" {
				fmt.Fprintf(stdout, "artifact=%s path=%s\n", artifact.Name, artifact.Path)
			}
		}
	}
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
		written, writeErr := writeArtifacts(resolveWorkdir(stepCtx.ProjectRoot, dir), stepCtx.Step.ID, artifacts)
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

type GoFormatAdapter struct {
	cfg config.Adapter
}

func (a GoFormatAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "go.format", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

func (a GoFormatAdapter) Run(ctx context.Context, stepCtx StepContext) (Result, error) {
	args := a.cfg.Args
	if len(args) == 0 {
		if stepCtx.Command == CommandFix {
			args = []string{"-w", "."}
		} else {
			args = []string{"-l", "."}
		}
	}
	cmdCfg := a.cfg
	cmdCfg.Command = "gofmt"
	cmdCfg.Args = args
	cmdCfg.Type = "cmd"
	result, err := CommandAdapter{cfg: cmdCfg}.Run(ctx, stepCtx)
	result.RuleID = "go.format"
	result.FixAvailable = stepCtx.Command == CommandCheck
	if stepCtx.Command == CommandCheck {
		for _, artifact := range result.Artifacts {
			if artifact.Name == "stdout" && strings.TrimSpace(artifact.Content) != "" {
				result.Kind = ResultViolation
				result.Message = "gofmt would change files"
				result.GateStatus = config.GateFail
				return result, err
			}
		}
		if result.GateStatus == config.GatePass {
			result.Message = "gofmt clean"
		}
	} else if result.GateStatus == config.GatePass {
		result.Message = "gofmt applied"
	}
	return result, err
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
