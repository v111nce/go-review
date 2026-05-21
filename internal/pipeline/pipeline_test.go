package pipeline

import (
	"reflect"
	"testing"
)

func TestNewGraphOrdersAndBatchesDAG(t *testing.T) {
	g, err := NewGraph([]Step{
		{ID: "test", AdapterID: "go.test", DependsOn: []string{"lint"}},
		{ID: "format", AdapterID: "go.format"},
		{ID: "security", AdapterID: "go.security"},
		{ID: "lint", AdapterID: "go.lint", DependsOn: []string{"format"}},
	})
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}
	got := stepIDs(g.Steps())
	want := []string{"format", "lint", "security", "test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	batches := ReadyBatches(g)
	gotBatches := make([][]string, len(batches))
	for i, batch := range batches {
		gotBatches[i] = stepIDs(batch)
	}
	wantBatches := [][]string{{"format", "security"}, {"lint"}, {"test"}}
	if !reflect.DeepEqual(gotBatches, wantBatches) {
		t.Fatalf("batches = %#v, want %#v", gotBatches, wantBatches)
	}
}

func TestNewGraphRejectsCyclesAndUnknownDependencies(t *testing.T) {
	if _, err := NewGraph([]Step{{ID: "a", DependsOn: []string{"b"}}}); err == nil {
		t.Fatal("expected unknown dependency error")
	}
	_, err := NewGraph([]Step{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	})
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestRunFailurePolicies(t *testing.T) {
	g, err := NewGraph([]Step{
		{ID: "format", OnFail: OnFailSkipDependents},
		{ID: "lint", DependsOn: []string{"format"}},
		{ID: "security", OnFail: OnFailContinue},
	})
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}
	results := Run(g, func(step Step) StepResult {
		if step.ID == "format" {
			return StepResult{Status: GateFail, FailureReason: "not formatted"}
		}
		return StepResult{Status: GatePass}
	})
	statuses := map[string]GateStatus{}
	for _, result := range results {
		statuses[result.StepID] = result.Status
	}
	if statuses["format"] != GateFail || statuses["lint"] != GateSkipped || statuses["security"] != GatePass {
		t.Fatalf("statuses = %#v", statuses)
	}
	if got := AggregateGate(results); got != GateFail {
		t.Fatalf("AggregateGate() = %s, want %s", got, GateFail)
	}
}

func TestSelectProfileRequiresIncludedDependencies(t *testing.T) {
	g, err := NewGraph([]Step{
		{ID: "format"},
		{ID: "lint", DependsOn: []string{"format"}},
		{ID: "nightly-security"},
	})
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}
	steps, err := SelectProfile(g, []Profile{{Name: "fast", Steps: []string{"format", "lint"}}}, "fast")
	if err != nil {
		t.Fatalf("SelectProfile() error = %v", err)
	}
	if got, want := stepIDs(steps), []string{"format", "lint"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("profile steps = %v, want %v", got, want)
	}
	if _, err := SelectProfile(g, []Profile{{Name: "ci", Steps: []string{"lint"}}}, "ci"); err == nil {
		t.Fatal("expected omitted dependency error")
	}
}

func TestMergeGateStatus(t *testing.T) {
	cases := []struct {
		name string
		in   []GateStatus
		want GateStatus
	}{
		{name: "pass", in: []GateStatus{GatePass, GatePass}, want: GatePass},
		{name: "warn", in: []GateStatus{GatePass, GateWarn}, want: GateWarn},
		{name: "fail", in: []GateStatus{GateWarn, GateFail}, want: GateFail},
		{name: "skipped only", in: []GateStatus{GateSkipped}, want: GateSkipped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MergeGateStatus(tc.in...); got != tc.want {
				t.Fatalf("MergeGateStatus() = %s, want %s", got, tc.want)
			}
		})
	}
}

func stepIDs(steps []Step) []string {
	out := make([]string, len(steps))
	for i, step := range steps {
		out[i] = step.ID
	}
	return out
}
