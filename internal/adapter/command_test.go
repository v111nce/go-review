package adapter

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"go-code-reviewer/internal/result"
)

func TestCommandAdapterSuccessCapturesOutputAndEnv(t *testing.T) {
	adapter := NewCommandAdapter()
	res, err := adapter.Run(context.Background(), ExecutionRequest{
		AdapterID: "cmd",
		StepID:    "echo",
		Command:   "sh",
		Args:      []string{"-c", "printf $LANE_B_TEST"},
		Env:       map[string]string{"LANE_B_TEST": "ok"},
	})
	if err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	if res.GateStatus != result.GatePass || res.ExitCode != 0 {
		t.Fatalf("unexpected result: %#v", res)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].Inline != "ok" {
		t.Fatalf("stdout not captured: %#v", res.Artifacts)
	}
}

func TestCommandAdapterFailureMapsGateFail(t *testing.T) {
	adapter := NewCommandAdapter()
	res, err := adapter.Run(context.Background(), ExecutionRequest{AdapterID: "cmd", StepID: "fail", Command: "sh", Args: []string{"-c", "echo boom >&2; exit 7"}})
	if err != nil {
		t.Fatalf("Run should return normalized failure without hard error: %v", err)
	}
	if res.GateStatus != result.GateFail || res.ExitCode != 7 {
		t.Fatalf("unexpected failure mapping: %#v", res)
	}
	if len(res.Artifacts) != 1 || !strings.Contains(res.Artifacts[0].Inline, "boom") {
		t.Fatalf("stderr not captured: %#v", res.Artifacts)
	}
}

func TestCommandAdapterTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	adapter := NewCommandAdapter()
	res, err := adapter.Run(context.Background(), ExecutionRequest{AdapterID: "cmd", StepID: "timeout", Command: "sh", Args: []string{"-c", "sleep 2"}, Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run should return timeout as normalized result: %v", err)
	}
	if !res.TimedOut || res.GateStatus != result.GateFail {
		t.Fatalf("expected timeout gate fail: %#v", res)
	}
}
