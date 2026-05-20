#!/usr/bin/env sh
set -eu
mode="${1:-pass}"
case "$mode" in
  pass)
    echo "fake-tool: pass"
    exit 0
    ;;
  warn)
    echo "testdata/fixtures/regression-gates/violating-project/main.go:5:1: fixture warning" >&2
    exit 0
    ;;
  fail)
    echo "testdata/fixtures/regression-gates/violating-project/main.go:5:1: fixture failure" >&2
    exit 1
    ;;
  sleep)
    sleep "${2:-2}"
    echo "fake-tool: slept"
    exit 0
    ;;
  *)
    echo "fake-tool: unknown mode $mode" >&2
    exit 2
    ;;
esac
