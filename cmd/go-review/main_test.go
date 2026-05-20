package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunVersionCommands(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := captureRun(args)
			if code != 0 {
				t.Fatalf("run(%v) code=%d stderr=%q", args, code, stderr)
			}
			if !strings.Contains(stdout, "go-review version") || !strings.Contains(stdout, "commit") {
				t.Fatalf("version output missing fields: %q", stdout)
			}
			if stderr != "" {
				t.Fatalf("stderr=%q", stderr)
			}
		})
	}
}

func TestRunHelpMentionsVersion(t *testing.T) {
	stdout, stderr, code := captureRun([]string{"--help"})
	if code != 0 {
		t.Fatalf("help code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "version") {
		t.Fatalf("help output missing version command:\n%s", stdout)
	}
}

func captureRun(args []string) (stdout string, stderr string, code int) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	os.Stdout = outW
	os.Stderr = errW
	code = run(args)
	_ = outW.Close()
	_ = errW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	var outBuf, errBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, outR)
	_, _ = io.Copy(&errBuf, errR)
	return outBuf.String(), errBuf.String(), code
}
