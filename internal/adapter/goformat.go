package adapter

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go-code-reviewer/internal/result"
)

const (
	GoFormatAdapterID   = "go.format"
	GoFormatAdapterKind = "go.format"
)

// GoFormatAdapter is a minimal built-in wrapper around gofmt that still uses the adapter contract.
type GoFormatAdapter struct{}

func NewGoFormatAdapter() *GoFormatAdapter { return &GoFormatAdapter{} }

func (a *GoFormatAdapter) Metadata() Metadata {
	return Metadata{
		ID:           GoFormatAdapterID,
		Kind:         GoFormatAdapterKind,
		Description:  "checks or applies gofmt formatting",
		Capabilities: []Capability{CapabilityCheck, CapabilityFix},
		ToolVersions: map[string]string{"gofmt": "system"},
	}
}

func (a *GoFormatAdapter) Run(ctx context.Context, req ExecutionRequest) (result.StepResult, error) {
	started := time.Now()
	step := req.StepID
	if step == "" {
		step = req.AdapterID
	}
	res := result.StepResult{AdapterID: adapterID(req, GoFormatAdapterID), StepID: step, StartedAt: started}
	inputs := req.Inputs
	if len(inputs) == 0 {
		inputs = req.Args
	}
	if len(inputs) == 0 {
		res.FinishedAt = time.Now()
		res.Duration = res.FinishedAt.Sub(started)
		res.GateStatus = result.GatePass
		res.Results = []result.Result{basicResultWithCategory(res.AdapterID, res.StepID, result.CategoryFormat, result.GatePass, "no Go files supplied", nil)}
		return res, nil
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "check"
	}
	args := append([]string{}, inputs...)
	if mode == "fix" {
		args = append([]string{"-w"}, args...)
	} else {
		args = append([]string{"-l"}, args...)
	}

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, "gofmt", args...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	out, err := cmd.CombinedOutput()
	res.FinishedAt = time.Now()
	res.Duration = res.FinishedAt.Sub(started)
	res.TimedOut = runCtx.Err() == context.DeadlineExceeded
	res.ExitCode = exitCode(err)
	if len(out) > 0 {
		res.Artifacts = []result.ArtifactRef{{Name: "gofmt-output", Kind: "stdout", ContentType: "text/plain", Inline: string(out)}}
	}

	gate := result.GatePass
	message := "gofmt passed"
	if res.TimedOut {
		gate = result.GateFail
		message = "gofmt timed out"
		res.FailureReason = fmt.Sprintf("timeout after %s", req.Timeout)
	} else if err != nil {
		gate = result.GateFail
		message = fmt.Sprintf("gofmt failed with exit code %d", res.ExitCode)
		res.FailureReason = err.Error()
	} else if mode != "fix" && strings.TrimSpace(string(out)) != "" {
		gate = result.GateFail
		message = "gofmt reported unformatted files"
	}
	res.GateStatus = gate
	res.Results = []result.Result{basicResultWithCategory(res.AdapterID, res.StepID, result.CategoryFormat, gate, message, res.Artifacts)}
	return res, nil
}
