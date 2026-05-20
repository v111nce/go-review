package main

import "testing"

func TestMessage(t *testing.T) {
	if Message() != "expected compliant message" {
		t.Fatalf("intentional failure: %q", Message())
	}
}
