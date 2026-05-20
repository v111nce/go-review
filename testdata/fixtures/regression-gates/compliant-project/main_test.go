package main

import "testing"

func TestMessage(t *testing.T) {
	if Message() != "compliant fixture" {
		t.Fatalf("unexpected message: %q", Message())
	}
}
