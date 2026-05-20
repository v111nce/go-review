package main

import "testing"

func TestMessage(t *testing.T) {
	if Message() != "autofix fixture" {
		t.Fatalf("unexpected message: %q", Message())
	}
}
