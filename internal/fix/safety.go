package fix

import (
	"fmt"
	"strings"
)

// Safety 是自动修复安全等级的标准枚举。
type Safety string

const (
	SafetySafe   Safety = "safe"
	SafetyReview Safety = "review"
	SafetyNone   Safety = "none"
)

// ParseSafety 规范化标准值及文档化别名。
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

// Policy 按文档化优先级解析安全等级：result、rule、adapter、profile/default。
type Policy struct {
	Default Safety
	Profile Safety
	Adapter map[string]Safety
	Rule    map[string]Safety
}

// Decision 解析单个候选修复的最终安全等级。
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

// CanAutoApply 判断解析后的等级是否允许 fix 命令自动应用。
func CanAutoApply(level Safety) bool { return level == SafetySafe }
