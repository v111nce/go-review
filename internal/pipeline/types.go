package pipeline

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// GateStatus is the normalized pipeline gate outcome used by schedulers and reports.
type GateStatus string

const (
	GatePass    GateStatus = "pass"
	GateWarn    GateStatus = "warn"
	GateFail    GateStatus = "fail"
	GateSkipped GateStatus = "skipped"
)

// ParseGateStatus validates a serialized gate status.
func ParseGateStatus(value string) (GateStatus, error) {
	switch GateStatus(value) {
	case GatePass, GateWarn, GateFail, GateSkipped:
		return GateStatus(value), nil
	default:
		return "", fmt.Errorf("unknown gate status %q", value)
	}
}

// MergeGateStatus aggregates step statuses into one pipeline gate.
func MergeGateStatus(statuses ...GateStatus) GateStatus {
	merged := GatePass
	for _, status := range statuses {
		switch status {
		case GateFail:
			return GateFail
		case GateWarn:
			if merged == GatePass || merged == GateSkipped {
				merged = GateWarn
			}
		case GateSkipped:
			if merged == GatePass {
				merged = GateSkipped
			}
		case GatePass, "":
			// pass and unset do not worsen the aggregate
		default:
			return GateFail
		}
	}
	return merged
}

// OnFailPolicy controls downstream behavior after a step fails.
type OnFailPolicy string

const (
	OnFailStop           OnFailPolicy = "stop"
	OnFailContinue       OnFailPolicy = "continue"
	OnFailSkipDependents OnFailPolicy = "skip_dependents"
)

func normalizeOnFail(policy OnFailPolicy) OnFailPolicy {
	if policy == "" {
		return OnFailStop
	}
	return policy
}

// Step is the contract-stable DAG node used by the review pipeline.
type Step struct {
	ID          string
	AdapterID   string
	DependsOn   []string
	OnFail      OnFailPolicy
	AllowFix    bool
	Artifacts   []string
	Timeout     time.Duration
	Description string
}

// StepResult captures the observable result of one step execution.
type StepResult struct {
	StepID        string        `json:"step_id"`
	AdapterID     string        `json:"adapter_id,omitempty"`
	Status        GateStatus    `json:"gate_status"`
	StartedAt     time.Time     `json:"started_at,omitempty"`
	EndedAt       time.Time     `json:"ended_at,omitempty"`
	Duration      time.Duration `json:"duration,omitempty"`
	ArtifactPaths []string      `json:"artifact_paths,omitempty"`
	FailureReason string        `json:"failure_reason,omitempty"`
}

// Execution is the minimal adapter-agnostic callback used by the scheduler.
type Execution func(step Step) StepResult

// Graph is a validated DAG of review steps.
type Graph struct {
	steps      map[string]Step
	order      []string
	dependents map[string][]string
}

// NewGraph validates steps, dependencies, and cycles.
func NewGraph(steps []Step) (*Graph, error) {
	if len(steps) == 0 {
		return nil, errors.New("pipeline must contain at least one step")
	}

	g := &Graph{steps: make(map[string]Step, len(steps)), dependents: map[string][]string{}}
	for _, step := range steps {
		if step.ID == "" {
			return nil, errors.New("pipeline step id is required")
		}
		if _, exists := g.steps[step.ID]; exists {
			return nil, fmt.Errorf("duplicate pipeline step %q", step.ID)
		}
		step.OnFail = normalizeOnFail(step.OnFail)
		switch step.OnFail {
		case OnFailStop, OnFailContinue, OnFailSkipDependents:
		default:
			return nil, fmt.Errorf("step %q has unsupported on_fail policy %q", step.ID, step.OnFail)
		}
		g.steps[step.ID] = step
		g.order = append(g.order, step.ID)
	}

	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if dep == "" {
				return nil, fmt.Errorf("step %q has empty dependency", step.ID)
			}
			if _, ok := g.steps[dep]; !ok {
				return nil, fmt.Errorf("step %q depends on unknown step %q", step.ID, dep)
			}
			g.dependents[dep] = append(g.dependents[dep], step.ID)
		}
	}

	order, err := g.topologicalOrder()
	if err != nil {
		return nil, err
	}
	g.order = order
	for id := range g.dependents {
		sort.Strings(g.dependents[id])
	}
	return g, nil
}

// Step returns one validated step by id.
func (g *Graph) Step(id string) (Step, bool) {
	step, ok := g.steps[id]
	return step, ok
}

// Steps returns steps in deterministic topological order.
func (g *Graph) Steps() []Step {
	out := make([]Step, 0, len(g.order))
	for _, id := range g.order {
		out = append(out, g.steps[id])
	}
	return out
}

// Dependents returns deterministic direct dependents for a step.
func (g *Graph) Dependents(id string) []string {
	deps := append([]string(nil), g.dependents[id]...)
	return deps
}

func (g *Graph) topologicalOrder() ([]string, error) {
	indegree := map[string]int{}
	for id := range g.steps {
		indegree[id] = 0
	}
	for _, step := range g.steps {
		for range step.DependsOn {
			indegree[step.ID]++
		}
	}

	ready := make([]string, 0)
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	order := make([]string, 0, len(g.steps))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, dependent := range g.dependents[id] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(g.steps) {
		return nil, errors.New("pipeline contains a dependency cycle")
	}
	return order, nil
}
