package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-code-reviewer/internal/config"
)

type fixTransaction struct {
	command   Command
	root      string
	snapshot  map[string][]byte
	protected bool
	applied   bool
	closed    bool
}

func newFixTransaction(command Command, root string) *fixTransaction {
	return &fixTransaction{command: command, root: root}
}

func (tx *fixTransaction) shouldProtect(ctx StepContext) bool {
	return tx.command == CommandFix &&
		ctx.Step.AllowFix &&
		ctx.Adapter.FixSafety == config.FixSafe &&
		ctx.Adapter.Type == "go.format"
}

func (tx *fixTransaction) snapshotProject() error {
	if tx.protected {
		return nil
	}
	files, err := goFiles(tx.root)
	if err != nil {
		return err
	}
	tx.snapshot = map[string][]byte{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		tx.snapshot[file] = append([]byte(nil), data...)
	}
	tx.protected = true
	return nil
}

func (tx *fixTransaction) markApplied() {
	if tx.protected {
		tx.applied = true
	}
}

func (tx *fixTransaction) rollbackAfterFailure(cause Result) Result {
	if tx.closed || !tx.protected || !tx.applied || cause.GateStatus != config.GateFail {
		return Result{}
	}
	tx.closed = true
	var failures []string
	for file, data := range tx.snapshot {
		if err := os.WriteFile(file, data, 0o644); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return Result{
			AdapterID:  "fix.transaction",
			StepID:     "fix-rollback",
			RuleID:     "fix.rollback",
			Kind:       ResultViolation,
			Message:    fmt.Sprintf("fix validation failed after %s and rollback had errors: %s", cause.StepID, strings.Join(failures, "; ")),
			FixSafety:  config.FixNone,
			GateStatus: config.GateFail,
		}
	}
	return Result{
		AdapterID:  "fix.transaction",
		StepID:     "fix-rollback",
		RuleID:     "fix.rollback",
		Kind:       ResultArtifact,
		Message:    fmt.Sprintf("rolled back safe fixes because validation step %s failed", cause.StepID),
		FixSafety:  config.FixNone,
		GateStatus: config.GateFail,
		Artifacts:  []Artifact{{Name: "changed-files", Content: strings.Join(tx.changedFiles(), "\n")}},
	}
}

func (tx *fixTransaction) changedFiles() []string {
	var files []string
	for file, before := range tx.snapshot {
		after, err := os.ReadFile(file)
		if err != nil || string(after) != string(before) {
			if rel, relErr := filepath.Rel(tx.root, file); relErr == nil {
				files = append(files, rel)
			} else {
				files = append(files, file)
			}
		}
	}
	return files
}
