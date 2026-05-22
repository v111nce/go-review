package adapter

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/v111nce/go-review/internal/result"
)

const (
	GoLintAdapterID   = "go.lint"
	GoLintAdapterKind = "go.lint"
)

// GoLintAdapter delegates format/lint execution to golangci-lint.
type GoLintAdapter struct{}

func NewGoLintAdapter() *GoLintAdapter { return &GoLintAdapter{} }

func (a *GoLintAdapter) Metadata() Metadata {
	return Metadata{
		ID:           GoLintAdapterID,
		Kind:         GoLintAdapterKind,
		Description:  "runs golangci-lint formatters/checks",
		Capabilities: []Capability{CapabilityCheck, CapabilityFix},
		ToolVersions: map[string]string{"golangci-lint": "system"},
	}
}

func (a *GoLintAdapter) Run(ctx context.Context, req ExecutionRequest) (result.StepResult, error) {
	started := time.Now()
	step := req.StepID
	if step == "" {
		step = req.AdapterID
	}
	res := result.StepResult{AdapterID: adapterID(req, GoLintAdapterID), StepID: step, StartedAt: started}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "check"
	}
	args := append([]string{}, req.Args...)
	if len(args) == 0 {
		args = []string{"fmt", "--no-config", "--enable", "gofmt"}
	}
	if mode == "fix" {
		args = removeExecutionArg(args, "--diff")
	} else if containsExecutionArg(args, "fmt") && !containsExecutionArg(args, "--diff") {
		args = append(args, "--diff")
	}
	args = append(args, req.Inputs...)

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	command := req.Command
	if strings.TrimSpace(command) == "" {
		command = "golangci-lint"
	}
	cmd := exec.CommandContext(runCtx, command, args...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	out, err := cmd.CombinedOutput()
	res.FinishedAt = time.Now()
	res.Duration = res.FinishedAt.Sub(started)
	res.TimedOut = runCtx.Err() == context.DeadlineExceeded
	res.ExitCode = exitCode(err)
	if len(out) > 0 {
		res.Artifacts = []result.ArtifactRef{{Name: "golangci-lint-output", Kind: "stdout", ContentType: "text/plain", Inline: string(out)}}
	}

	gate := result.GatePass
	message := "golangci-lint fmt passed"
	if res.TimedOut {
		gate = result.GateFail
		message = "golangci-lint timed out"
		res.FailureReason = fmt.Sprintf("timeout after %s", req.Timeout)
	} else if err != nil {
		gate = result.GateFail
		message = fmt.Sprintf("golangci-lint failed with exit code %d", res.ExitCode)
		res.FailureReason = err.Error()
	} else if mode == "fix" {
		message = "golangci-lint fmt applied"
	}
	res.GateStatus = gate
	res.Results = []result.Result{basicResultWithCategory(res.AdapterID, res.StepID, result.CategoryFormat, gate, message, res.Artifacts)}
	return res, nil
}

func containsExecutionArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func removeExecutionArg(args []string, remove string) []string {
	out := args[:0]
	for _, arg := range args {
		if arg != remove {
			out = append(out, arg)
		}
	}
	return out
}
