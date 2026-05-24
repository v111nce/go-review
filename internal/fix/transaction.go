package fix

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Formatter 在编辑写入后校验或格式化变更文件。
type Formatter func(paths []string) error

// Validator 在格式化成功后重跑受影响检查。
type Validator func(paths []string) error

// Transaction 应用安全编辑，并在写入、格式化或验证失败时回滚。
type Transaction struct {
	Root      string
	Formatter Formatter
	Validator Validator
}

// Result 是一次修复事务尝试的持久证据。
type Result struct {
	Applied       bool     `json:"applied"`
	RolledBack    bool     `json:"rolled_back"`
	ChangedFiles  []string `json:"changed_files,omitempty"`
	FailureReason string   `json:"failure_reason,omitempty"`
}

// Apply 只校验并应用 safe 编辑；非 safe 输入会作为 no-op 结果返回。
func (tx Transaction) Apply(safety Safety, edits []TextEdit) Result {
	if safety != SafetySafe {
		return Result{Applied: false, FailureReason: "fix safety is not safe"}
	}
	if err := ValidateEdits(edits); err != nil {
		return Result{FailureReason: err.Error()}
	}
	files := filesForEdits(edits)
	backups := map[string][]byte{}

	for _, file := range files {
		path := tx.path(file)
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{ChangedFiles: files, FailureReason: err.Error()}
		}
		backups[file] = append([]byte(nil), data...)
		fileEdits := filterEdits(edits, file)
		next, err := ApplyToBytes(data, fileEdits)
		if err != nil {
			return Result{ChangedFiles: files, FailureReason: err.Error()}
		}
		if err := os.WriteFile(path, next, 0o644); err != nil {
			rollback(tx.Root, backups)
			return Result{ChangedFiles: files, RolledBack: true, FailureReason: err.Error()}
		}
	}

	if tx.Formatter != nil {
		if err := tx.Formatter(files); err != nil {
			rollback(tx.Root, backups)
			return Result{ChangedFiles: files, RolledBack: true, FailureReason: fmt.Sprintf("formatter failed: %v", err)}
		}
	}
	if tx.Validator != nil {
		if err := tx.Validator(files); err != nil {
			rollback(tx.Root, backups)
			return Result{ChangedFiles: files, RolledBack: true, FailureReason: fmt.Sprintf("validation failed: %v", err)}
		}
	}
	return Result{Applied: true, ChangedFiles: files}
}

func (tx Transaction) path(file string) string {
	if tx.Root == "" || filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(tx.Root, file)
}

func filesForEdits(edits []TextEdit) []string {
	seen := map[string]struct{}{}
	files := []string{}
	for _, edit := range edits {
		if _, ok := seen[edit.File]; ok {
			continue
		}
		seen[edit.File] = struct{}{}
		files = append(files, edit.File)
	}
	sort.Strings(files)
	return files
}

func filterEdits(edits []TextEdit, file string) []TextEdit {
	out := []TextEdit{}
	for _, edit := range edits {
		if edit.File == file {
			out = append(out, edit)
		}
	}
	return out
}

func rollback(root string, backups map[string][]byte) {
	for file, data := range backups {
		path := file
		if root != "" && !filepath.IsAbs(file) {
			path = filepath.Join(root, file)
		}
		_ = os.WriteFile(path, data, 0o644)
	}
}
