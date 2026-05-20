package fix

import (
	"fmt"
	"strings"
)

// Safety is the canonical automatic-fix safety level.
type Safety string

const (
	SafetySafe   Safety = "safe"
	SafetyReview Safety = "review"
	SafetyNone   Safety = "none"
)

// ParseSafety normalizes canonical values and documented aliases.
func ParseSafety(value string) (Safety, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none", "report-only", "report_only", "off":
		return SafetyNone, nil
	case "safe", "auto", "automatic":
		return SafetySafe, nil
	case "review", "manual-review", "manual_review", "suggest":
		return SafetyReview, nil
	default:
		return "", fmt.Errorf("unknown fix safety %q", value)
	}
}

// Policy applies documented precedence: result, rule, adapter, profile/default.
type Policy struct {
	Default Safety
	Profile Safety
	Adapter map[string]Safety
	Rule    map[string]Safety
}

// Decision resolves the safety for one candidate fix.
func (p Policy) Decision(adapterID, ruleID string, resultLevel Safety) Safety {
	if resultLevel != "" {
		return resultLevel
	}
	if p.Rule != nil {
		if safety, ok := p.Rule[ruleID]; ok {
			return safety
		}
	}
	if p.Adapter != nil {
		if safety, ok := p.Adapter[adapterID]; ok {
			return safety
		}
	}
	if p.Profile != "" {
		return p.Profile
	}
	if p.Default != "" {
		return p.Default
	}
	return SafetyNone
}

// CanAutoApply reports whether the resolved level is eligible for fix command application.
func CanAutoApply(level Safety) bool { return level == SafetySafe }
