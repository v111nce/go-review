package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/v111nce/go-review/internal/result"
)

func TestGoFormatAdapterCheckDetectsUnformattedFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main(){println(1)}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewGoFormatAdapter()
	res, err := adapter.Run(context.Background(), ExecutionRequest{AdapterID: "go.format", StepID: "format", Inputs: []string{file}})
	if err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	if res.GateStatus != result.GateFail {
		t.Fatalf("expected fail for unformatted file: %#v", res)
	}
	if len(res.Artifacts) == 0 || !strings.Contains(res.Artifacts[0].Inline, file) {
		t.Fatalf("expected gofmt output artifact with file path: %#v", res.Artifacts)
	}
}

func TestGoFormatAdapterFixFormatsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main(){println(1)}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewGoFormatAdapter()
	res, err := adapter.Run(context.Background(), ExecutionRequest{AdapterID: "go.format", StepID: "format", Mode: "fix", Inputs: []string{file}})
	if err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	if res.GateStatus != result.GatePass {
		t.Fatalf("expected pass after fix: %#v", res)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "func main()") {
		t.Fatalf("file was not formatted: %s", content)
	}
}
