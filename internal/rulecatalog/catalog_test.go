package rulecatalog

import (
	"bytes"
	"strings"
	"testing"
)

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
