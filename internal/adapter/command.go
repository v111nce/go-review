package adapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"go-code-reviewer/internal/result"
)

const (
	CommandAdapterID   = "cmd"
	CommandAdapterKind = "command"
)

// CommandAdapter executes arbitrary local commands and maps process state to a gate result.
type CommandAdapter struct{}

// NewCommandAdapter creates the generic command adapter.
func NewCommandAdapter() *CommandAdapter { return &CommandAdapter{} }

func (a *CommandAdapter) Metadata() Metadata {
	return Metadata{
		ID:           CommandAdapterID,
		Kind:         CommandAdapterKind,
		Description:  "runs an external command and captures stdout/stderr/exit status",
		Capabilities: []Capability{CapabilityCheck, CapabilityTest, CapabilityScan},
	}
}

func (a *CommandAdapter) Run(ctx context.Context, req ExecutionRequest) (result.StepResult, error) {
	started := time.Now()
	step := req.StepID
	if step == "" {
		step = req.AdapterID
	}
	res := result.StepResult{AdapterID: adapterID(req, CommandAdapterID), StepID: step, StartedAt: started}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		res.FinishedAt = time.Now()
		res.Duration = res.FinishedAt.Sub(started)
		res.GateStatus = result.GateFail
		res.FailureReason = "command is required"
		res.Results = []result.Result{basicResult(res.AdapterID, res.StepID, result.GateFail, "command is required", nil)}
		return res, errors.New("command is required")
	}

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, command, req.Args...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	cmd.Env = mergeEnv(os.Environ(), req.Env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res.FinishedAt = time.Now()
	res.Duration = res.FinishedAt.Sub(started)
	res.TimedOut = errors.Is(runCtx.Err(), context.DeadlineExceeded)
	res.ExitCode = exitCode(err)
	res.Artifacts = commandArtifacts(stdout.String(), stderr.String())

	gate := result.GatePass
	message := "command completed successfully"
	if res.TimedOut {
		gate = result.GateFail
		message = "command timed out"
		res.FailureReason = fmt.Sprintf("timeout after %s", req.Timeout)
	} else if err != nil {
		gate = result.GateFail
		message = fmt.Sprintf("command failed with exit code %d", res.ExitCode)
		res.FailureReason = err.Error()
	}
	res.GateStatus = gate
	res.Results = []result.Result{basicResult(res.AdapterID, res.StepID, gate, message, res.Artifacts)}
	return res, nil
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	values := map[string]string{}
	for _, entry := range base {
		key, val, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = val
		}
	}
	for key, val := range overrides {
		values[key] = val
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	merged := make([]string, 0, len(keys))
	for _, key := range keys {
		merged = append(merged, key+"="+values[key])
	}
	return merged
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func commandArtifacts(stdout, stderr string) []result.ArtifactRef {
	artifacts := make([]result.ArtifactRef, 0, 2)
	if stdout != "" {
		artifacts = append(artifacts, result.ArtifactRef{Name: "stdout", Kind: "stdout", ContentType: "text/plain", Inline: stdout})
	}
	if stderr != "" {
		artifacts = append(artifacts, result.ArtifactRef{Name: "stderr", Kind: "stderr", ContentType: "text/plain", Inline: stderr})
	}
	return artifacts
}
