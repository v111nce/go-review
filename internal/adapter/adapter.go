// Package adapter 定义 review 工具无关的 adapter 执行契约。
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

// Capability 声明 adapter 支持的能力。
type Capability string

const (
	CapabilityCheck  Capability = "check"
	CapabilityFix    Capability = "fix"
	CapabilityTest   Capability = "test"
	CapabilityScan   Capability = "scan"
	CapabilityReport Capability = "report"
)

// Metadata 描述 adapter 元信息，不暴露具体实现细节。
type Metadata struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Description  string            `json:"description,omitempty"`
	Capabilities []Capability      `json:"capabilities"`
	Version      string            `json:"version,omitempty"`
	ToolVersions map[string]string `json:"tool_versions,omitempty"`
}

// ExecutionRequest 是所有 adapter 共用的标准化调用参数。
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

// Adapter 用统一契约包装一个 review 工具或内置执行器。
type Adapter interface {
	Metadata() Metadata
	Run(context.Context, ExecutionRequest) (result.StepResult, error)
}

// Registry 根据显式 ID 或 kind 别名解析 adapter。
type Registry struct {
	byID   map[string]Adapter
	byKind map[string]Adapter
}

// NewRegistry 创建空 adapter 注册表。
func NewRegistry() *Registry {
	return &Registry{byID: map[string]Adapter{}, byKind: map[string]Adapter{}}
}

// Register 注册一个 adapter。ID 必须唯一；kind 是可选别名，
// 同一个 kind 的第一个 adapter 生效，从而保证显式 ID 的解析结果稳定。
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

// Resolve 优先按显式 ID 查找 adapter，其次按 kind 别名查找。
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

// MustResolve 在解析失败时返回包含已知 adapter 列表的错误。
func (r *Registry) MustResolve(idOrKind string) (Adapter, error) {
	if a, ok := r.Resolve(idOrKind); ok {
		return a, nil
	}
	return nil, fmt.Errorf("adapter %q not registered; known adapters: %s", idOrKind, strings.Join(r.IDs(), ", "))
}

// IDs 按稳定顺序返回已注册 adapter ID。
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.byID))
	for id := range r.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// HasCapability 判断元信息中是否包含指定能力。
func HasCapability(meta Metadata, capability Capability) bool {
	for _, c := range meta.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// RegisterBuiltins 注册本包内置 adapter。
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

// NewDefaultRegistry 返回已安装首版内置 adapter 的注册表。
func NewDefaultRegistry() (*Registry, error) {
	r := NewRegistry()
	if err := RegisterBuiltins(r); err != nil {
		return nil, err
	}
	return r, nil
}
