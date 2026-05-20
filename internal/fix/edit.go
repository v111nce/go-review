package fix

import (
	"errors"
	"fmt"
	"sort"
)

// TextEdit is a byte-offset replacement within one file.
type TextEdit struct {
	File    string
	Start   int
	End     int
	NewText string
}

// ValidateEdits checks range validity and overlapping edits.
func ValidateEdits(edits []TextEdit) error {
	byFile := map[string][]TextEdit{}
	for _, edit := range edits {
		if edit.File == "" {
			return errors.New("text edit file is required")
		}
		if edit.Start < 0 || edit.End < edit.Start {
			return fmt.Errorf("invalid edit range for %s: %d..%d", edit.File, edit.Start, edit.End)
		}
		byFile[edit.File] = append(byFile[edit.File], edit)
	}
	for file, fileEdits := range byFile {
		sort.SliceStable(fileEdits, func(i, j int) bool {
			if fileEdits[i].Start == fileEdits[j].Start {
				return fileEdits[i].End < fileEdits[j].End
			}
			return fileEdits[i].Start < fileEdits[j].Start
		})
		for i := 1; i < len(fileEdits); i++ {
			if fileEdits[i].Start < fileEdits[i-1].End {
				return fmt.Errorf("overlapping edits in %s: %d..%d overlaps %d..%d", file, fileEdits[i-1].Start, fileEdits[i-1].End, fileEdits[i].Start, fileEdits[i].End)
			}
		}
	}
	return nil
}

// ApplyToBytes applies non-overlapping edits to file content.
func ApplyToBytes(content []byte, edits []TextEdit) ([]byte, error) {
	if err := ValidateEdits(edits); err != nil {
		return nil, err
	}
	sorted := append([]TextEdit(nil), edits...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start > sorted[j].Start })
	out := append([]byte(nil), content...)
	for _, edit := range sorted {
		if edit.End > len(out) {
			return nil, fmt.Errorf("edit range %d..%d exceeds file size %d", edit.Start, edit.End, len(out))
		}
		next := make([]byte, 0, len(out)-edit.End+edit.Start+len(edit.NewText))
		next = append(next, out[:edit.Start]...)
		next = append(next, edit.NewText...)
		next = append(next, out[edit.End:]...)
		out = next
	}
	return out, nil
}
