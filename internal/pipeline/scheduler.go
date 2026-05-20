package pipeline

import (
	"sort"
	"time"
)

// Run executes a validated graph in deterministic dependency order.
// The implementation is intentionally sequential for the contract skeleton; the graph
// exposes ready batches so a bounded parallel scheduler can be added without changing callers.
func Run(g *Graph, exec Execution) []StepResult {
	results := make([]StepResult, 0, len(g.order))
	resultByID := map[string]StepResult{}
	skipped := map[string]string{}
	stopReason := ""

	for _, step := range g.Steps() {
		if stopReason != "" {
			results = append(results, skippedResult(step, stopReason))
			continue
		}
		if reason := skipped[step.ID]; reason != "" {
			results = append(results, skippedResult(step, reason))
			continue
		}
		if dependencyFailureReason(step, resultByID) != "" {
			results = append(results, skippedResult(step, "dependency failed"))
			continue
		}

		started := time.Now()
		result := exec(step)
		if result.StepID == "" {
			result.StepID = step.ID
		}
		if result.AdapterID == "" {
			result.AdapterID = step.AdapterID
		}
		if result.StartedAt.IsZero() {
			result.StartedAt = started
		}
		if result.EndedAt.IsZero() {
			result.EndedAt = time.Now()
		}
		if result.Duration == 0 && !result.StartedAt.IsZero() && !result.EndedAt.IsZero() {
			result.Duration = result.EndedAt.Sub(result.StartedAt)
		}
		if result.Status == "" {
			result.Status = GatePass
		}
		resultByID[step.ID] = result
		results = append(results, result)

		if result.Status == GateFail {
			switch normalizeOnFail(step.OnFail) {
			case OnFailStop:
				stopReason = "pipeline stopped after failed step " + step.ID
			case OnFailSkipDependents:
				markDependentsSkipped(g, skipped, step.ID, "skipped after failed dependency "+step.ID)
			case OnFailContinue:
				// keep scheduling independent/downstream steps
			}
		}
	}
	return results
}

// ReadyBatches returns deterministic dependency-ready layers for bounded parallel execution.
func ReadyBatches(g *Graph) [][]Step {
	remainingDeps := map[string]int{}
	for _, step := range g.Steps() {
		remainingDeps[step.ID] = len(step.DependsOn)
	}
	emitted := map[string]bool{}
	batches := [][]Step{}

	for len(emitted) < len(g.order) {
		ids := []string{}
		for id, count := range remainingDeps {
			if count == 0 && !emitted[id] {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		batch := make([]Step, 0, len(ids))
		for _, id := range ids {
			emitted[id] = true
			batch = append(batch, g.steps[id])
			for _, dependent := range g.dependents[id] {
				remainingDeps[dependent]--
			}
		}
		batches = append(batches, batch)
	}
	return batches
}

func skippedResult(step Step, reason string) StepResult {
	now := time.Now()
	return StepResult{StepID: step.ID, AdapterID: step.AdapterID, Status: GateSkipped, StartedAt: now, EndedAt: now, FailureReason: reason}
}

func dependencyFailureReason(step Step, results map[string]StepResult) string {
	for _, dep := range step.DependsOn {
		if results[dep].Status == GateFail || results[dep].Status == GateSkipped {
			return dep
		}
	}
	return ""
}

func markDependentsSkipped(g *Graph, skipped map[string]string, root string, reason string) {
	for _, dependent := range g.Dependents(root) {
		if _, exists := skipped[dependent]; !exists {
			skipped[dependent] = reason
		}
		markDependentsSkipped(g, skipped, dependent, reason)
	}
}

// AggregateGate returns the pipeline gate for a set of step results.
func AggregateGate(results []StepResult) GateStatus {
	statuses := make([]GateStatus, 0, len(results))
	for _, result := range results {
		statuses = append(statuses, result.Status)
	}
	return MergeGateStatus(statuses...)
}
