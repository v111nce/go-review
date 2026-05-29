package rulecatalog

import (
	"bytes"
	"strings"
	"testing"
)

// TestCatalogCRUDValidateAndRender 覆盖规则 catalog 的基础 CRUD、校验和 Markdown 渲染。
// 这样 rules/go-rules.json 作为源数据时，命令行维护和文档生成都有回归保护。
func TestCatalogCRUDValidateAndRender(t *testing.T) {
	catalog := Empty()
	rule := Rule{
		ID:             "team.semantic.max-params",
		Description:    "函数/方法入参个数不得超过配置阈值。",
		Source:         Source{Name: "Team semantic rules", Section: "max-params"},
		Handling:       "tool-semantic",
		Adapter:        "go.semantic",
		ToolRules:      []string{"max-params"},
		DefaultProfile: "strict",
		Autofix:        Autofix{Supported: false, Safety: "none"},
		Status:         "active",
		Implemented:    true,
		Notes:          "确定性 AST 子规则。",
	}
	if err := catalog.Add(rule); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := catalog.Add(rule); err == nil {
		t.Fatal("duplicate Add should fail")
	}
	got, ok := catalog.Get(rule.ID)
	if !ok || got.Description != rule.Description {
		t.Fatalf("Get() = %#v, %v", got, ok)
	}
	rule.Description = "更新后的说明。"
	if err := catalog.Upsert(rule); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, _ = catalog.Get(rule.ID)
	if got.Description != "更新后的说明。" {
		t.Fatalf("Upsert description = %q", got.Description)
	}
	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, catalog); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(buf.String(), "由 JSON catalog 生成") || !strings.Contains(buf.String(), "team.semantic.max-params") || !strings.Contains(buf.String(), "yes") {
		t.Fatalf("rendered markdown missing content:\n%s", buf.String())
	}
	if err := catalog.Delete(rule.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := catalog.Get(rule.ID); ok {
		t.Fatal("deleted rule still exists")
	}
}

func TestRuleValidationRejectsAmbiguousHandling(t *testing.T) {
	rule := Rule{
		ID:          "x",
		Description: "说明",
		Source:      Source{Name: "source"},
		Handling:    "mixed",
		Autofix:     Autofix{Safety: "none"},
		Status:      "active",
	}
	if err := rule.Validate(); err == nil || !strings.Contains(err.Error(), "invalid handling") {
		t.Fatalf("Validate err = %v, want invalid handling", err)
	}
}

// TestRepositoryCatalogImplementsAllNonCandidateRules 锁定当前分类承诺：
// 除 D 类 candidate 外，其余规则必须标记 implemented，并指向明确 adapter/tool_rules。
func TestRepositoryCatalogImplementsAllNonCandidateRules(t *testing.T) {
	catalog, err := LoadFile("../../rules/go-rules.json")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	counts := map[string]int{}
	for _, rule := range catalog.Rules {
		counts[rule.Handling]++
		if rule.Handling == "candidate" {
			if rule.Implemented {
				t.Fatalf("candidate rule %s should not be implemented", rule.ID)
			}
			continue
		}
		if !rule.Implemented {
			t.Fatalf("non-candidate rule %s handling=%s should be implemented", rule.ID, rule.Handling)
		}
		switch rule.Handling {
		case "tool-golangci", "tool-golangci-config", "tool-go-test", "tool-semantic":
			if rule.Adapter == "" || len(rule.ToolRules) == 0 {
				t.Fatalf("tool rule %s missing adapter/tool_rules: %#v", rule.ID, rule)
			}
		case "llm-review":
			if rule.Adapter != "llm.review" || len(rule.ToolRules) != 1 || rule.ToolRules[0] != "AGENTS.md" || !strings.Contains(rule.Notes, "AGENTS.md") {
				t.Fatalf("llm rule %s not linked to AGENTS.md: %#v", rule.ID, rule)
			}
		default:
			t.Fatalf("unexpected non-candidate handling %s for %s", rule.Handling, rule.ID)
		}
	}
	want := map[string]int{"tool-golangci": 35, "tool-golangci-config": 47, "tool-go-test": 3, "tool-semantic": 8, "llm-review": 122, "candidate": 8}
	for handling, n := range want {
		if counts[handling] != n {
			t.Fatalf("handling %s count=%d, want %d", handling, counts[handling], n)
		}
	}
}
