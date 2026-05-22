package adapter

import "testing"

func TestRegistryResolveByIDAndKind(t *testing.T) {
	registry := NewRegistry()
	cmd := NewCommandAdapter()
	if err := registry.Register(cmd); err != nil {
		t.Fatalf("Register command: %v", err)
	}
	if got, ok := registry.Resolve(CommandAdapterID); !ok || got != cmd {
		t.Fatalf("Resolve by ID failed: %v %v", got, ok)
	}
	if got, ok := registry.Resolve(CommandAdapterKind); !ok || got != cmd {
		t.Fatalf("Resolve by kind failed: %v %v", got, ok)
	}
	if _, err := registry.MustResolve("missing"); err == nil {
		t.Fatal("MustResolve missing expected error")
	}
}

func TestRegistryRejectsDuplicateIDs(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewCommandAdapter()); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := registry.Register(NewCommandAdapter()); err == nil {
		t.Fatal("duplicate register expected error")
	}
}

func TestNewDefaultRegistryIncludesBuiltins(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
	for _, id := range []string{CommandAdapterID, GoLintAdapterID} {
		if _, ok := registry.Resolve(id); !ok {
			t.Fatalf("expected builtin %s to resolve", id)
		}
	}
}
