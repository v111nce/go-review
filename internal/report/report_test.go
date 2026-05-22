package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func sampleReport() RunReport {
	return RunReport{
		Profile:    "ci",
		GateStatus: GateFail,
		Steps: []Step{
			{ID: "test", AdapterID: "go.test", Status: GatePass},
			{ID: "lint", AdapterID: "go.lint", Status: GateFail, FailureReason: "lint failed"},
		},
		Findings: []Finding{
			{AdapterID: "go.lint", StepID: "lint", RuleID: "SA1000", File: "main.go", Line: 3, Message: "bad thing", FixAvailable: true, FixSafety: "review", GateStatus: GateFail},
		},
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, sampleReport()); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	var decoded RunReport
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if decoded.SchemaVersion != "go-review.report.v1" || decoded.GateStatus != GateFail {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestWriteTerminal(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteTerminal(&buf, sampleReport()); err != nil {
		t.Fatalf("WriteTerminal() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"profile=ci", "gate=fail", "SA1000", "main.go:3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("terminal output missing %q:\n%s", want, out)
		}
	}
}

func TestWriteMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, sampleReport()); err != nil {
		t.Fatalf("WriteMarkdown() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"# go-review 报告", "## 失败项", "| lint | SA1000 | main.go:3 | bad thing | review |", "- 重新运行 `go-review check --profile ci`。"} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown output missing %q:\n%s", want, out)
		}
	}
}

func TestWriteLLMMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteLLMMarkdown(&buf, sampleReport()); err != nil {
		t.Fatalf("WriteLLMMarkdown() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"# go-review LLM 修复上下文", "## 重要约束", "### 失败项 1", "规则：SA1000", "预期完成标准"} {
		if !strings.Contains(out, want) {
			t.Fatalf("llm markdown output missing %q:\n%s", want, out)
		}
	}
}
