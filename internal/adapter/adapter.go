// Package adapter defines the tool-agnostic execution contract for review tools.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/v111nce/go-review/internal/result"
)

// Capability declares what an adapter can do.
type Capability string

const (
	CapabilityCheck  Capability = "check"
	CapabilityFix    Capability = "fix"
	CapabilityTest   Capability = "test"
	CapabilityScan   Capability = "scan"
	CapabilityReport Capability = "report"
)

// Metadata describes an adapter without exposing concrete implementation details.
type Metadata struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Description  string            `json:"description,omitempty"`
	Capabilities []Capability      `json:"capabilities"`
	Version      string            `json:"version,omitempty"`
	ToolVersions map[string]string `json:"tool_versions,omitempty"`
}

// ExecutionRequest is the normalized invocation payload for any adapter.
type ExecutionRequest struct {
	AdapterID string            `json:"adapter_id"`
	StepID    string            `json:"step_id"`
	Mode      string            `json:"mode,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	WorkDir   string            `json:"workdir,omitempty"`
	Timeout   time.Duration     `json:"timeout,omitempty"`
	Inputs    []string          `json:"inputs,omitempty"`
	Parser    string            `json:"parser,omitempty"`
	Options   map[string]string `json:"options,omitempty"`
}

// Adapter runs one review tool or built-in wrapper behind a common contract.
type Adapter interface {
	Metadata() Metadata
	Run(context.Context, ExecutionRequest) (result.StepResult, error)
}

// Registry resolves configured adapters by ID or kind.
type Registry struct {
	byID   map[string]Adapter
	byKind map[string]Adapter
}

// NewRegistry creates an empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{byID: map[string]Adapter{}, byKind: map[string]Adapter{}}
}

// Register adds an adapter. IDs must be unique. Kinds are optional aliases; the
// first adapter for a kind wins so explicit IDs remain deterministic.
func (r *Registry) Register(a Adapter) error {
	if a == nil {
		return errors.New("adapter is nil")
	}
	meta := a.Metadata()
	id := strings.TrimSpace(meta.ID)
	if id == "" {
		return errors.New("adapter id is required")
	}
	if _, exists := r.byID[id]; exists {
		return fmt.Errorf("adapter %q already registered", id)
	}
	r.byID[id] = a
	kind := strings.TrimSpace(meta.Kind)
	if kind != "" {
		if _, exists := r.byKind[kind]; !exists {
			r.byKind[kind] = a
		}
	}
	return nil
}

// Resolve returns an adapter by explicit ID first, then kind alias.
func (r *Registry) Resolve(idOrKind string) (Adapter, bool) {
	key := strings.TrimSpace(idOrKind)
	if key == "" {
		return nil, false
	}
	if a, ok := r.byID[key]; ok {
		return a, true
	}
	a, ok := r.byKind[key]
	return a, ok
}

// MustResolve returns an error with known adapters when resolution fails.
func (r *Registry) MustResolve(idOrKind string) (Adapter, error) {
	if a, ok := r.Resolve(idOrKind); ok {
		return a, nil
	}
	return nil, fmt.Errorf("adapter %q not registered; known adapters: %s", idOrKind, strings.Join(r.IDs(), ", "))
}

// IDs returns registered adapter IDs in stable order.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.byID))
	for id := range r.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// HasCapability reports whether metadata includes a capability.
func HasCapability(meta Metadata, capability Capability) bool {
	for _, c := range meta.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// RegisterBuiltins installs the built-in adapters owned by this package.
func RegisterBuiltins(r *Registry) error {
	if r == nil {
		return errors.New("registry is nil")
	}
	for _, builtin := range []Adapter{NewCommandAdapter(), NewGoLintAdapter()} {
		if err := r.Register(builtin); err != nil {
			return err
		}
	}
	return nil
}

// NewDefaultRegistry returns a registry with the first-version built-ins installed.
func NewDefaultRegistry() (*Registry, error) {
	r := NewRegistry()
	if err := RegisterBuiltins(r); err != nil {
		return nil, err
	}
	return r, nil
}
