package pipeline

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// GateStatus 是调度器和报告共用的标准化 pipeline 门禁结果。
type GateStatus string

const (
	GatePass    GateStatus = "pass"
	GateWarn    GateStatus = "warn"
	GateFail    GateStatus = "fail"
	GateSkipped GateStatus = "skipped"
)

// ParseGateStatus 校验序列化后的门禁状态。
func ParseGateStatus(value string) (GateStatus, error) {
	switch GateStatus(value) {
	case GatePass, GateWarn, GateFail, GateSkipped:
		return GateStatus(value), nil
	default:
		return "", fmt.Errorf("unknown gate status %q", value)
	}
}

// MergeGateStatus 把多个 step 状态聚合成一个 pipeline 门禁状态。
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
			// pass 和空值不会降低聚合结果
		default:
			return GateFail
		}
	}
	return merged
}

// OnFailPolicy 控制 step 失败后的下游行为。
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

// Step 是 review pipeline 使用的稳定契约 DAG 节点。
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

// StepResult 记录一次 step 执行的可观测结果。
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

// Execution 是调度器使用的最小 adapter 无关执行回调。
type Execution func(step Step) StepResult

// Graph 是已校验的 review step DAG。
type Graph struct {
	steps      map[string]Step
	order      []string
	dependents map[string][]string
}

// NewGraph 校验 step、依赖关系和环。
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

// Step 按 ID 返回一个已校验 step。
func (g *Graph) Step(id string) (Step, bool) {
	step, ok := g.steps[id]
	return step, ok
}

// Steps 按稳定拓扑顺序返回 step。
func (g *Graph) Steps() []Step {
	out := make([]Step, 0, len(g.order))
	for _, id := range g.order {
		out = append(out, g.steps[id])
	}
	return out
}

// Dependents 返回某个 step 的稳定直接下游列表。
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
