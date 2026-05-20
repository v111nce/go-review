package config

import (
	"bufio"

	"errors"
	"fmt"
	"go-code-reviewer/internal/adapter"
	"go-code-reviewer/internal/result"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	CurrentSchemaMajor = 1
	CurrentSchemaMinor = 0
)

type GateStatus = result.GateStatus

const (
	GatePass GateStatus = result.GatePass
	GateWarn GateStatus = result.GateWarn
	GateFail GateStatus = result.GateFail
)

func ParseGateStatus(value string) (GateStatus, error) {
	return result.ParseGateStatus(value)
}

type FixSafety = result.FixSafety

const (
	FixSafe   FixSafety = result.FixSafe
	FixReview FixSafety = result.FixReview
	FixNone   FixSafety = result.FixNone
)

func ParseFixSafety(value string) (FixSafety, error) {
	return result.ParseFixSafety(value)
}

type Capability = adapter.Capability

const (
	CapabilityCheck  Capability = adapter.CapabilityCheck
	CapabilityFix    Capability = adapter.CapabilityFix
	CapabilityTest   Capability = adapter.CapabilityTest
	CapabilityScan   Capability = adapter.CapabilityScan
	CapabilityReport Capability = adapter.CapabilityReport
)

type OnFail string

const (
	OnFailStop           OnFail = "stop"
	OnFailContinue       OnFail = "continue"
	OnFailSkipDependents OnFail = "skip_dependents"
)

func ParseOnFail(value string) (OnFail, error) {
	switch OnFail(strings.ToLower(strings.TrimSpace(value))) {
	case "", OnFailStop:
		return OnFailStop, nil
	case OnFailContinue:
		return OnFailContinue, nil
	case OnFailSkipDependents:
		return OnFailSkipDependents, nil
	default:
		return "", fmt.Errorf("unsupported on_fail policy %q", value)
	}
}

type Config struct {
	SchemaVersion string
	Tools         ToolVersions
	Defaults      Defaults
	Adapters      []Adapter
	Steps         []Step
	Profiles      []Profile
	Artifacts     ArtifactConfig
}

type ToolVersions struct {
	GoReview string
	Adapters map[string]string
}

type Defaults struct {
	Timeout time.Duration
	Workdir string
}

type Adapter struct {
	ID           string
	Type         string
	Command      string
	Args         []string
	Workdir      string
	Env          map[string]string
	Parser       string
	Capabilities []Capability
	Timeout      time.Duration
	Version      string
	FixSafety    FixSafety
}

type Step struct {
	ID        string
	AdapterID string
	DependsOn []string
	OnFail    OnFail
	AllowFix  bool
	Timeout   time.Duration
	Artifacts []string
}

type Profile struct {
	Name  string
	Steps []string
}

type ArtifactConfig struct {
	Dir string
}

func LoadFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, err := Load(f)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	if cfg.Defaults.Workdir == "" {
		cfg.Defaults.Workdir = filepath.Dir(path)
	}
	return cfg, nil
}

func Load(r io.Reader) (*Config, error) {
	lines, err := readYAMLLines(r)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line.indent != 0 {
			continue
		}
		key, val, ok := splitKeyValue(line.text)
		if !ok {
			return nil, line.err("expected top-level key")
		}
		switch key {
		case "schema_version":
			cfg.SchemaVersion = unquote(val)
		case "tools":
			tools, next, err := parseTools(lines, i+1, line.indent)
			if err != nil {
				return nil, err
			}
			cfg.Tools = tools
			i = next - 1
		case "defaults":
			defaults, next, err := parseDefaults(lines, i+1, line.indent)
			if err != nil {
				return nil, err
			}
			cfg.Defaults = defaults
			i = next - 1
		case "adapters":
			adapters, next, err := parseAdapters(lines, i+1, line.indent)
			if err != nil {
				return nil, err
			}
			cfg.Adapters = adapters
			i = next - 1
		case "steps":
			steps, next, err := parseSteps(lines, i+1, line.indent)
			if err != nil {
				return nil, err
			}
			cfg.Steps = steps
			i = next - 1
		case "profiles":
			profiles, next, err := parseProfiles(lines, i+1, line.indent)
			if err != nil {
				return nil, err
			}
			cfg.Profiles = profiles
			i = next - 1
		case "artifacts":
			artifacts, next, err := parseArtifacts(lines, i+1, line.indent)
			if err != nil {
				return nil, err
			}
			cfg.Artifacts = artifacts
			i = next - 1
		default:
			return nil, line.err("unknown top-level field %q", key)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cfg *Config) Validate() error {
	if err := validateSchemaVersion(cfg.SchemaVersion); err != nil {
		return err
	}
	if cfg.Tools.Adapters == nil {
		cfg.Tools.Adapters = map[string]string{}
	}
	adapterIDs := map[string]struct{}{}
	for i := range cfg.Adapters {
		a := &cfg.Adapters[i]
		if a.ID == "" {
			return fmt.Errorf("adapter[%d] missing id", i)
		}
		if _, exists := adapterIDs[a.ID]; exists {
			return fmt.Errorf("duplicate adapter id %q", a.ID)
		}
		adapterIDs[a.ID] = struct{}{}
		if a.Type == "" {
			a.Type = a.ID
		}
		if a.Timeout == 0 {
			a.Timeout = cfg.Defaults.Timeout
		}
		if a.FixSafety == "" {
			a.FixSafety = FixNone
		}
		if a.Env == nil {
			a.Env = map[string]string{}
		}
	}
	stepIDs := map[string]struct{}{}
	for i := range cfg.Steps {
		s := &cfg.Steps[i]
		if s.ID == "" {
			return fmt.Errorf("step[%d] missing id", i)
		}
		if _, exists := stepIDs[s.ID]; exists {
			return fmt.Errorf("duplicate step id %q", s.ID)
		}
		stepIDs[s.ID] = struct{}{}
		if s.AdapterID == "" {
			return fmt.Errorf("step %q missing adapter", s.ID)
		}
		if _, ok := adapterIDs[s.AdapterID]; !ok {
			return fmt.Errorf("step %q references unknown adapter %q", s.ID, s.AdapterID)
		}
		if s.OnFail == "" {
			s.OnFail = OnFailStop
		}
	}
	for _, p := range cfg.Profiles {
		if p.Name == "" {
			return errors.New("profile missing name")
		}
		if len(p.Steps) == 0 {
			return fmt.Errorf("profile %q has no steps", p.Name)
		}
		for _, stepID := range p.Steps {
			if _, ok := stepIDs[stepID]; !ok {
				return fmt.Errorf("profile %q references unknown step %q", p.Name, stepID)
			}
		}
	}
	return nil
}

func (cfg *Config) Profile(name string) (*Profile, error) {
	if name == "" {
		name = "local"
	}
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == name {
			return &cfg.Profiles[i], nil
		}
	}
	return nil, fmt.Errorf("profile %q not found", name)
}

func (cfg *Config) Adapter(id string) (*Adapter, bool) {
	for i := range cfg.Adapters {
		if cfg.Adapters[i].ID == id {
			return &cfg.Adapters[i], true
		}
	}
	return nil, false
}

func (cfg *Config) Step(id string) (*Step, bool) {
	for i := range cfg.Steps {
		if cfg.Steps[i].ID == id {
			return &cfg.Steps[i], true
		}
	}
	return nil, false
}

func validateSchemaVersion(version string) error {
	if version == "" {
		return errors.New("schema_version is required")
	}
	parts := strings.Split(version, ".")
	if len(parts) < 1 || len(parts) > 2 {
		return fmt.Errorf("schema_version %q must be MAJOR or MAJOR.MINOR", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("schema_version %q has invalid major version", version)
	}
	minor := 0
	if len(parts) == 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("schema_version %q has invalid minor version", version)
		}
	}
	if major != CurrentSchemaMajor {
		return fmt.Errorf("unsupported schema major version %d", major)
	}
	if minor > CurrentSchemaMinor {
		return fmt.Errorf("unsupported schema minor version %d for major %d", minor, major)
	}
	return nil
}

type yamlLine struct {
	num    int
	indent int
	text   string
}

func (l yamlLine) err(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", l.num, fmt.Sprintf(format, args...))
}

func readYAMLLines(r io.Reader) ([]yamlLine, error) {
	scanner := bufio.NewScanner(r)
	var lines []yamlLine
	for num := 1; scanner.Scan(); num++ {
		raw := stripComment(scanner.Text())
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if strings.Contains(raw, "\t") {
			return nil, fmt.Errorf("line %d: tabs are not supported in YAML indentation", num)
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		lines = append(lines, yamlLine{num: num, indent: indent, text: strings.TrimSpace(raw)})
	}
	return lines, scanner.Err()
}

func stripComment(s string) string {
	inSingle, inDouble := false, false
	for i, r := range s {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return s[:i]
			}
		}
	}
	return s
}

func splitKeyValue(text string) (string, string, bool) {
	idx := strings.Index(text, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+1:]), true
}

func parseTools(lines []yamlLine, start, parentIndent int) (ToolVersions, int, error) {
	tools := ToolVersions{Adapters: map[string]string{}}
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		key, val, ok := splitKeyValue(lines[i].text)
		if !ok {
			return tools, i, lines[i].err("expected tools key")
		}
		switch key {
		case "go_review":
			tools.GoReview = unquote(val)
		case "adapters":
			m, next, err := parseStringMap(lines, i+1, lines[i].indent)
			if err != nil {
				return tools, i, err
			}
			tools.Adapters = m
			i = next - 1
		default:
			return tools, i, lines[i].err("unknown tools field %q", key)
		}
		i++
	}
	return tools, i, nil
}

func parseDefaults(lines []yamlLine, start, parentIndent int) (Defaults, int, error) {
	var defaults Defaults
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		key, val, ok := splitKeyValue(lines[i].text)
		if !ok {
			return defaults, i, lines[i].err("expected defaults key")
		}
		switch key {
		case "timeout":
			d, err := parseDuration(val)
			if err != nil {
				return defaults, i, lines[i].err("%v", err)
			}
			defaults.Timeout = d
		case "workdir":
			defaults.Workdir = unquote(val)
		default:
			return defaults, i, lines[i].err("unknown defaults field %q", key)
		}
		i++
	}
	return defaults, i, nil
}

func parseAdapters(lines []yamlLine, start, parentIndent int) ([]Adapter, int, error) {
	var adapters []Adapter
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		line := lines[i]
		if !strings.HasPrefix(line.text, "- ") {
			return adapters, i, line.err("expected adapter list item")
		}
		adapter := Adapter{Env: map[string]string{}}
		next, err := parseAdapterItem(lines, i, parentIndent, &adapter)
		if err != nil {
			return adapters, i, err
		}
		adapters = append(adapters, adapter)
		i = next
	}
	return adapters, i, nil
}

func parseAdapterItem(lines []yamlLine, start, parentIndent int, adapter *Adapter) (int, error) {
	itemIndent := lines[start].indent
	fields := []yamlLine{{num: lines[start].num, indent: itemIndent + 2, text: strings.TrimSpace(strings.TrimPrefix(lines[start].text, "- "))}}
	i := start + 1
	for i < len(lines) && lines[i].indent > itemIndent {
		fields = append(fields, lines[i])
		i++
	}
	for j := 0; j < len(fields); j++ {
		if fields[j].text == "" {
			continue
		}
		key, val, ok := splitKeyValue(fields[j].text)
		if !ok {
			return i, fields[j].err("expected adapter field")
		}
		switch key {
		case "id":
			adapter.ID = unquote(val)
		case "type":
			adapter.Type = unquote(val)
		case "command":
			adapter.Command = unquote(val)
		case "args":
			adapter.Args = parseStringListInline(val)
		case "workdir":
			adapter.Workdir = unquote(val)
		case "env":
			m, next, err := parseStringMap(fields, j+1, fields[j].indent)
			if err != nil {
				return i, err
			}
			adapter.Env = m
			j = next - 1
		case "parser":
			adapter.Parser = unquote(val)
		case "capabilities":
			for _, c := range parseStringListInline(val) {
				adapter.Capabilities = append(adapter.Capabilities, Capability(c))
			}
		case "timeout":
			d, err := parseDuration(val)
			if err != nil {
				return i, fields[j].err("%v", err)
			}
			adapter.Timeout = d
		case "version":
			adapter.Version = unquote(val)
		case "fix_safety":
			v, err := ParseFixSafety(val)
			if err != nil {
				return i, fields[j].err("%v", err)
			}
			adapter.FixSafety = v
		default:
			return i, fields[j].err("unknown adapter field %q", key)
		}
	}
	_ = parentIndent
	return i, nil
}

func parseSteps(lines []yamlLine, start, parentIndent int) ([]Step, int, error) {
	var steps []Step
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		line := lines[i]
		if !strings.HasPrefix(line.text, "- ") {
			return steps, i, line.err("expected step list item")
		}
		step := Step{OnFail: OnFailStop}
		next, err := parseStepItem(lines, i, &step)
		if err != nil {
			return steps, i, err
		}
		steps = append(steps, step)
		i = next
	}
	return steps, i, nil
}

func parseStepItem(lines []yamlLine, start int, step *Step) (int, error) {
	itemIndent := lines[start].indent
	fields := []yamlLine{{num: lines[start].num, indent: itemIndent + 2, text: strings.TrimSpace(strings.TrimPrefix(lines[start].text, "- "))}}
	i := start + 1
	for i < len(lines) && lines[i].indent > itemIndent {
		fields = append(fields, lines[i])
		i++
	}
	for j := 0; j < len(fields); j++ {
		key, val, ok := splitKeyValue(fields[j].text)
		if !ok {
			return i, fields[j].err("expected step field")
		}
		switch key {
		case "id":
			step.ID = unquote(val)
		case "adapter":
			step.AdapterID = unquote(val)
		case "depends_on":
			step.DependsOn = parseStringListInline(val)
		case "on_fail":
			v, err := ParseOnFail(val)
			if err != nil {
				return i, fields[j].err("%v", err)
			}
			step.OnFail = v
		case "allow_fix":
			v, err := strconv.ParseBool(unquote(val))
			if err != nil {
				return i, fields[j].err("invalid allow_fix %q", val)
			}
			step.AllowFix = v
		case "timeout":
			d, err := parseDuration(val)
			if err != nil {
				return i, fields[j].err("%v", err)
			}
			step.Timeout = d
		case "artifacts":
			step.Artifacts = parseStringListInline(val)
		default:
			return i, fields[j].err("unknown step field %q", key)
		}
	}
	return i, nil
}

func parseProfiles(lines []yamlLine, start, parentIndent int) ([]Profile, int, error) {
	var profiles []Profile
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		line := lines[i]
		if !strings.HasPrefix(line.text, "- ") {
			return profiles, i, line.err("expected profile list item")
		}
		profile := Profile{}
		next, err := parseProfileItem(lines, i, &profile)
		if err != nil {
			return profiles, i, err
		}
		profiles = append(profiles, profile)
		i = next
	}
	return profiles, i, nil
}

func parseProfileItem(lines []yamlLine, start int, profile *Profile) (int, error) {
	itemIndent := lines[start].indent
	fields := []yamlLine{{num: lines[start].num, indent: itemIndent + 2, text: strings.TrimSpace(strings.TrimPrefix(lines[start].text, "- "))}}
	i := start + 1
	for i < len(lines) && lines[i].indent > itemIndent {
		fields = append(fields, lines[i])
		i++
	}
	for _, field := range fields {
		key, val, ok := splitKeyValue(field.text)
		if !ok {
			return i, field.err("expected profile field")
		}
		switch key {
		case "name":
			profile.Name = unquote(val)
		case "steps":
			profile.Steps = parseStringListInline(val)
		default:
			return i, field.err("unknown profile field %q", key)
		}
	}
	return i, nil
}

func parseArtifacts(lines []yamlLine, start, parentIndent int) (ArtifactConfig, int, error) {
	var artifacts ArtifactConfig
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		key, val, ok := splitKeyValue(lines[i].text)
		if !ok {
			return artifacts, i, lines[i].err("expected artifacts key")
		}
		switch key {
		case "dir":
			artifacts.Dir = unquote(val)
		default:
			return artifacts, i, lines[i].err("unknown artifacts field %q", key)
		}
		i++
	}
	return artifacts, i, nil
}

func parseStringMap(lines []yamlLine, start, parentIndent int) (map[string]string, int, error) {
	m := map[string]string{}
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		key, val, ok := splitKeyValue(lines[i].text)
		if !ok {
			return m, i, lines[i].err("expected map key")
		}
		m[key] = unquote(val)
		i++
	}
	return m, i, nil
}

func parseStringListInline(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" {
		return nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	}
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = unquote(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseDuration(value string) (time.Duration, error) {
	value = unquote(value)
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	return d, nil
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
