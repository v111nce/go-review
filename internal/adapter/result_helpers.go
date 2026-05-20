package adapter

import "go-code-reviewer/internal/result"

func adapterID(req ExecutionRequest, fallback string) string {
	if req.AdapterID != "" {
		return req.AdapterID
	}
	return fallback
}

func basicResult(adapterID, stepID string, gate result.GateStatus, message string, artifacts []result.ArtifactRef) result.Result {
	return basicResultWithCategory(adapterID, stepID, result.CategoryCommand, gate, message, artifacts)
}

func basicResultWithCategory(adapterID, stepID string, category result.Category, gate result.GateStatus, message string, artifacts []result.ArtifactRef) result.Result {
	severity := result.SeverityInfo
	if gate == result.GateFail {
		severity = result.SeverityHigh
	}
	return result.Result{
		AdapterID:  adapterID,
		StepID:     stepID,
		RuleID:     adapterID,
		Kind:       result.KindArtifact,
		Category:   category,
		Severity:   severity,
		Scope:      result.ScopeRepository,
		Message:    message,
		Fix:        result.Fix{Available: false, Safety: result.FixNone},
		GateStatus: gate,
		Artifacts:  artifacts,
	}.Normalize()
}
