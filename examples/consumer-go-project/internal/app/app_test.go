package app

import "testing"

func TestMessage(t *testing.T) {
	if Message() != "consumer project ready" {
		t.Fatalf("unexpected message: %q", Message())
	}
}
