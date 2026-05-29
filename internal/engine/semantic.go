package engine

import (
	"context"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/v111nce/go-review/internal/config"
)

const noDirectEnvRuleID = "semantic.no-direct-os-getenv"

// SemanticAdapter 负责运行 go.semantic 规则。
//
// go.semantic 的定位是“基于 AST / type info 的确定性语义检查”：通用格式化和静态检查
// 继续交给 golangci-lint，团队/项目特有但能用代码稳定判断的规则放到这里，并统一用
// go/analysis.Analyzer 表达。
type SemanticAdapter struct {
	cfg config.Adapter
}

type semanticAnalyzerFactory func() (*analysis.Analyzer, semanticRuleMeta)
type semanticCustomAnalyzerFactory func(semanticCustomRule) (*analysis.Analyzer, semanticRuleMeta)

// semanticRuleMeta 保存 analyzer 之外的报告元数据。
// Analyzer 负责发现问题；meta 负责把发现的问题挂到稳定 rule_id、pass message 和修复提示。
type semanticRuleMeta struct {
	RuleID        string
	PassMessage   string
	FixSafety     config.FixSafety
	FixAvailable  bool
	SuggestionFor func(analysis.Diagnostic) string
}

// semanticBuiltInAnalyzers 是 go-review 随默认配置提供的内置语义规则集合。
//
// 内置规则来自 rules/go-rules.json 中已确定能由 AST/type info 自动判断的规范项，用户只需要
// 在 .go-review/semantic/default.yaml 的 rules 列表中启用/禁用名称，不需要写 Go 代码。
var semanticBuiltInAnalyzers = map[string]semanticAnalyzerFactory{
	// channel-size：限制显式 channel buffer 大小，覆盖 Uber 对大 buffer 需要设计说明的要求。
	"channel-size": func() (*analysis.Analyzer, semanticRuleMeta) {
		rule := semanticCustomRule{
			ID:         "uber.guideline.channel-size",
			Max:        1,
			Message:    "channel buffer size greater than 1 requires explicit design justification",
			Suggestion: "prefer an unbuffered channel, size 1, or document why a larger buffer is required",
		}
		return channelSizeAnalyzer(rule, "channel_size"), semanticMeta(rule, "semantic channel-size passed")
	},
	// custom-contexts：禁止自定义 context-like interface，鼓励直接使用 context.Context。
	"custom-contexts": func() (*analysis.Analyzer, semanticRuleMeta) {
		rule := semanticCustomRule{
			ID:         "google.libs.custom-contexts",
			Message:    "custom context-like interface should use context.Context directly",
			Suggestion: "accept context.Context instead of defining a custom Context interface/type",
		}
		return customContextAnalyzer(rule, "custom_contexts"), semanticMeta(rule, "semantic custom-contexts passed")
	},
	// enum-start-one：要求 iota 枚举预留 0 值，避免零值被误认为有效业务枚举。
	"enum-start-one": func() (*analysis.Analyzer, semanticRuleMeta) {
		rule := semanticCustomRule{
			ID:         "uber.guideline.enum-start-one",
			Message:    "iota enum should reserve zero as unknown or invalid and start valid values at one",
			Suggestion: "add an explicit zero value such as Unknown or Invalid before iota values",
		}
		return enumStartOneAnalyzer(rule, "enum_start_one"), semanticMeta(rule, "semantic enum-start-one passed")
	},
	// exit-in-main：把 os.Exit 收敛到 package main 的 main 函数，库代码应返回 error。
	"exit-in-main": func() (*analysis.Analyzer, semanticRuleMeta) {
		rule := semanticCustomRule{
			ID:         "uber.guideline.exit-in-main",
			Message:    "os.Exit should be centralized in main",
			Suggestion: "return errors from library code and call os.Exit only from main",
		}
		return exitInMainAnalyzer(rule, "exit_in_main"), semanticMeta(rule, "semantic exit-in-main passed")
	},
	// import-blank：限制空白导入，只允许 main/test 文件显式做副作用注册。
	"import-blank": func() (*analysis.Analyzer, semanticRuleMeta) {
		rule := semanticCustomRule{
			ID:         "go.official.import-blank",
			Message:    "blank import is only allowed in main or test files for explicit side effects",
			Suggestion: "move side-effect registration to main/test scope or replace the blank import with a normal dependency",
		}
		return blankImportAnalyzer(rule, "import_blank"), semanticMeta(rule, "semantic import-blank passed")
	},
	// no-tfatal-goroutine：禁止在 goroutine 内直接调用 t.Fatal/t.FailNow。
	"no-tfatal-goroutine": func() (*analysis.Analyzer, semanticRuleMeta) {
		rule := semanticCustomRule{
			ID:         "google.bp.no-tfatal-goroutine",
			Message:    "do not call t.Fatal from a goroutine",
			Suggestion: "send the error to the test goroutine and call t.Fatal there",
		}
		return noTFatalGoroutineAnalyzer(rule, "no_tfatal_goroutine"), semanticMeta(rule, "semantic no-tfatal-goroutine passed")
	},
	// no-direct-os-getenv：内置 no-direct-call 示例，要求通过可注入配置读取环境变量。
	"no-direct-os-getenv": func() (*analysis.Analyzer, semanticRuleMeta) {
		rule := semanticCustomRule{
			ID:         noDirectEnvRuleID,
			Package:    "os",
			Function:   "Getenv",
			Message:    "direct os.Getenv access bypasses injectable configuration",
			Suggestion: "read environment values through an injected config/env provider; package main startup code and *_test.go are ignored by the default rule",
		}
		return noDirectOSGetenvAnalyzer(rule, "no_direct_os_getenv"), semanticMeta(rule, "semantic no-direct-os-getenv passed")
	},
}

// semanticCustomAnalyzers 是无需重新编译 go-review 就能通过 YAML 配置的规则种类。
//
// 它不是任意脚本/插件机制，而是“参数化 analyzer”：框架预先实现 kind，用户在
// .go-review/semantic/custom.yaml 中填写 id、阈值、目标包函数等参数。
var semanticCustomAnalyzers = map[string]semanticCustomAnalyzerFactory{
	// no-direct-call：配置 package + function，禁止直接调用某个包级函数。
	"no-direct-call": func(rule semanticCustomRule) (*analysis.Analyzer, semanticRuleMeta) {
		message := rule.Message
		if message == "" {
			message = fmt.Sprintf("direct %s.%s call is disallowed", rule.Package, rule.Function)
		}
		meta := semanticRuleMeta{
			RuleID:      semanticRuleID(rule.ID),
			PassMessage: fmt.Sprintf("semantic custom rule %s passed", rule.ID),
			SuggestionFor: func(analysis.Diagnostic) string {
				return rule.Suggestion
			},
		}
		rule.Message = message
		return noDirectCallAnalyzer(rule, analyzerName(rule.ID)), meta
	},
	// max-params：配置 max，限制函数/方法入参数量。
	"max-params": func(rule semanticCustomRule) (*analysis.Analyzer, semanticRuleMeta) {
		return maxParamsAnalyzer(rule, analyzerName(rule.ID)), semanticMeta(rule, fmt.Sprintf("semantic custom rule %s passed", rule.ID))
	},
}

// semanticMeta 为常见规则生成统一报告元数据。
func semanticMeta(rule semanticCustomRule, passMessage string) semanticRuleMeta {
	return semanticRuleMeta{
		RuleID:      semanticRuleID(rule.ID),
		PassMessage: passMessage,
		SuggestionFor: func(analysis.Diagnostic) string {
			return rule.Suggestion
		},
	}
}

func (a SemanticAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "go.semantic", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

// Run 按配置顺序运行内置规则和自定义规则。
//
// 每条规则失败时只返回当前规则的第一条诊断，但 adapter 自身不会让其它 pipeline step
// 隐式中断；是否继续由 step.on_fail 控制。默认配置里每个检查部分都是 continue，保证
// semantic 失败不会影响 format/lint/test 等其它部分继续运行。
func (a SemanticAdapter) Run(_ context.Context, stepCtx StepContext) (Result, error) {
	cfg, err := a.semanticConfig(stepCtx)
	if err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.config", Kind: ResultViolation, Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}
	for _, rule := range cfg.BuiltInRules {
		analyzer, meta, err := a.builtInAnalyzer(rule)
		if err != nil {
			return a.semanticConfigFailure(stepCtx, err.Error()), nil
		}
		result, err := a.runAnalyzer(analyzer, meta, stepCtx)
		if err != nil || result.GateStatus == config.GateFail {
			return result, err
		}
	}
	for _, rule := range cfg.CustomRules {
		analyzer, meta, err := a.customAnalyzer(rule)
		if err != nil {
			return a.semanticConfigFailure(stepCtx, err.Error()), nil
		}
		result, err := a.runAnalyzer(analyzer, meta, stepCtx)
		if err != nil || result.GateStatus == config.GateFail {
			return result, err
		}
	}
	ruleCount := len(cfg.BuiltInRules) + len(cfg.CustomRules)
	return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.rules", Kind: ResultArtifact, Message: fmt.Sprintf("semantic analyzers passed (%d)", ruleCount), FixSafety: a.cfg.FixSafety, GateStatus: config.GatePass}, nil
}

// builtInAnalyzer 根据 default.yaml 中的规则名创建内置 analyzer。
func (a SemanticAdapter) builtInAnalyzer(rule string) (*analysis.Analyzer, semanticRuleMeta, error) {
	rule = semanticBuiltInRuleName(rule)
	factory, ok := semanticBuiltInAnalyzers[rule]
	if !ok {
		return nil, semanticRuleMeta{}, fmt.Errorf("unsupported semantic rule %q", rule)
	}
	analyzer, meta := factory()
	return analyzer, meta, nil
}

// customAnalyzer 根据 custom.yaml 中的 kind 和参数创建参数化 analyzer。
func (a SemanticAdapter) customAnalyzer(rule semanticCustomRule) (*analysis.Analyzer, semanticRuleMeta, error) {
	kind := semanticCustomKind(rule.Kind)
	factory, ok := semanticCustomAnalyzers[kind]
	if !ok {
		return nil, semanticRuleMeta{}, fmt.Errorf("unsupported semantic custom rule kind %q", kind)
	}
	rule.Kind = kind
	analyzer, meta := factory(rule)
	return analyzer, meta, nil
}

// maxParamsAnalyzer 检查函数/方法入参数量是否超过 rule.Max。
//
// Go AST 会把 `func f(a, b int)` 表达为一个 Field、两个 Names，因此不能只数
// FieldList.List 的长度；fieldListNameCount 会按实际命名参数个数计算，未命名参数按 1 个算。
func maxParamsAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              fmt.Sprintf("reports functions with more than %d parameters", rule.Max),
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				ast.Inspect(file, func(node ast.Node) bool {
					decl, ok := node.(*ast.FuncDecl)
					if !ok || decl.Type == nil {
						return true
					}
					count := fieldListNameCount(decl.Type.Params)
					if count <= rule.Max {
						return false
					}
					message := rule.Message
					if message == "" {
						message = fmt.Sprintf("function %s has %d parameters, maximum is %d", decl.Name.Name, count, rule.Max)
					}
					diagnostic := analysis.Diagnostic{Pos: decl.Pos(), Category: semanticRuleID(rule.ID), Message: message}
					if rule.Suggestion != "" {
						diagnostic.SuggestedFixes = []analysis.SuggestedFix{{Message: rule.Suggestion}}
					}
					pass.Report(diagnostic)
					return false
				})
			}
			return nil, nil
		},
	}
}

// fieldListNameCount 计算参数列表中的实际参数个数。
func fieldListNameCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
			continue
		}
		count += len(field.Names)
	}
	return count
}

// blankImportAnalyzer 检查空白导入位置。
// main 包和 _test.go 文件常用于显式注册副作用，默认放行；普通业务包中的 `_` import
// 更容易隐藏依赖和初始化副作用，因此报告违规。
func blankImportAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              "reports blank imports outside main and test files",
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				filename := pass.Fset.Position(file.Package).Filename
				if (file.Name != nil && file.Name.Name == "main") || strings.HasSuffix(filename, "_test.go") {
					continue
				}
				for _, spec := range file.Imports {
					if spec.Name != nil && spec.Name.Name == "_" {
						pass.Report(analysis.Diagnostic{Pos: spec.Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message})
					}
				}
			}
			return nil, nil
		},
	}
}

// customContextAnalyzer 检查自定义 context-like interface/type。
//
// 判定边界故意保持保守：名称里包含 context 且底层是 interface 才报告；直接 alias/包装
// context.Context 的普通 `Context` 名称放行，减少误报。
func customContextAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              "reports custom context-like interfaces",
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				ast.Inspect(file, func(node ast.Node) bool {
					spec, ok := node.(*ast.TypeSpec)
					if !ok || spec.Name == nil {
						return true
					}
					if strings.EqualFold(spec.Name.Name, "Context") && selectorExprString(spec.Type) == "context.Context" {
						return true
					}
					if !strings.Contains(strings.ToLower(spec.Name.Name), "context") {
						return true
					}
					if _, ok := spec.Type.(*ast.InterfaceType); ok {
						pass.Report(analysis.Diagnostic{Pos: spec.Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message})
						return false
					}
					return true
				})
			}
			return nil, nil
		},
	}
}

// channelSizeAnalyzer 检查 make(chan T, N) 的静态整数字面量缓冲大小。
//
// 只处理编译期可见的整数 literal；变量/常量表达式暂不展开，避免在轻量 analyzer 中引入
// 复杂求值导致误报。
func channelSizeAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	max := rule.Max
	if max <= 0 {
		max = 1
	}
	return &analysis.Analyzer{
		Name:             name,
		Doc:              "reports channel buffers larger than the configured maximum",
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				ast.Inspect(file, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok || !isIdent(call.Fun, "make") || len(call.Args) < 2 {
						return true
					}
					if _, ok := call.Args[0].(*ast.ChanType); !ok {
						return true
					}
					size, ok := intLiteralValue(call.Args[1])
					if !ok || size <= max {
						return true
					}
					pass.Report(analysis.Diagnostic{Pos: call.Args[1].Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message})
					return true
				})
			}
			return nil, nil
		},
	}
}

// enumStartOneAnalyzer 检查 iota 枚举组是否预留零值。
//
// 规则只看 const 组第一项：如果第一项使用 iota 且名称没有 Unknown/Invalid/None/Zero
// 等零值语义，就认为有效枚举从 0 开始，报告违规。
func enumStartOneAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              "reports const iota groups that start valid enum values at zero",
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				for _, decl := range file.Decls {
					gen, ok := decl.(*ast.GenDecl)
					if !ok || gen.Tok != token.CONST || len(gen.Specs) == 0 {
						continue
					}
					first, ok := gen.Specs[0].(*ast.ValueSpec)
					if !ok || !valueSpecUsesIota(first) || firstConstNameAllowsZero(first) {
						continue
					}
					pass.Report(analysis.Diagnostic{Pos: first.Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message})
				}
			}
			return nil, nil
		},
	}
}

// exitInMainAnalyzer 检查 os.Exit 是否只出现在 package main 的 main 函数中。
//
// 它结合 import 名称和 types.Info 判断 selector 确实指向 os.Exit；当类型信息因 fixture
// 不完整不可用时，再回退到 import 别名匹配。
func exitInMainAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              "reports os.Exit outside package main main()",
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				packageNames := importedNames(file, "os")
				ast.Inspect(file, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel == nil || sel.Sel.Name != "Exit" || !isPackageFunctionSelector(pass.TypesInfo, packageNames, "os", "Exit", sel) {
						return true
					}
					if enclosingFunctionName(file, call.Pos()) == "main" && file.Name != nil && file.Name.Name == "main" {
						return true
					}
					pass.Report(analysis.Diagnostic{Pos: call.Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message})
					return true
				})
			}
			return nil, nil
		},
	}
}

// noTFatalGoroutineAnalyzer 检查 go statement 内部是否直接调用 Fatal/Fatalf/FailNow。
// 该规则用 AST 子树扫描即可稳定发现，暂不推断变量是否一定是 *testing.T，以免引入误报。
func noTFatalGoroutineAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              "reports t.Fatal calls inside go statements",
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				ast.Inspect(file, func(node ast.Node) bool {
					stmt, ok := node.(*ast.GoStmt)
					if !ok || stmt.Call == nil {
						return true
					}
					ast.Inspect(stmt.Call, func(inner ast.Node) bool {
						call, ok := inner.(*ast.CallExpr)
						if !ok {
							return true
						}
						sel, ok := call.Fun.(*ast.SelectorExpr)
						if ok && sel.Sel != nil && (sel.Sel.Name == "Fatal" || sel.Sel.Name == "Fatalf" || sel.Sel.Name == "FailNow") {
							pass.Report(analysis.Diagnostic{Pos: call.Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message})
						}
						return true
					})
					return true
				})
			}
			return nil, nil
		},
	}
}

// noDirectCallAnalyzer 检查是否直接调用配置指定的 package.function。
//
// 这是自定义规则中最通用的一类：例如禁止 os.Getenv、fmt.Println、time.Now 等直接调用，
// 转而要求走可注入的封装层。它优先用 types.Info 精确确认包路径，类型信息不足时回退
// 到 import 名称匹配。
func noDirectCallAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              fmt.Sprintf("reports direct %s.%s calls", rule.Package, rule.Function),
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				packageNames := importedNames(file, rule.Package)
				ast.Inspect(file, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel == nil || sel.Sel.Name != rule.Function {
						return true
					}
					if !isPackageFunctionSelector(pass.TypesInfo, packageNames, rule.Package, rule.Function, sel) {
						return true
					}
					diagnostic := analysis.Diagnostic{Pos: call.Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message}
					if rule.Suggestion != "" {
						diagnostic.SuggestedFixes = []analysis.SuggestedFix{{Message: rule.Suggestion}}
					}
					pass.Report(diagnostic)
					return true
				})
			}
			return nil, nil
		},
	}
}

// noDirectOSGetenvAnalyzer 是内置 no-direct-os-getenv 的低噪音版本。
//
// 普通业务包直接读取环境变量会绕过配置注入，导致测试和部署行为难以控制，因此默认报告。
// 但启动入口 package main 往往负责把部署环境变量转换成配置，集成测试也常用环境变量接入
// 外部资源；这两个场景在 ailx-agent 这类 go-zero 项目里是合理用法，所以默认放行。
func noDirectOSGetenvAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              "reports direct os.Getenv calls outside startup and test files",
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				filename := pass.Fset.Position(file.Package).Filename
				if (file.Name != nil && file.Name.Name == "main") || strings.HasSuffix(filename, "_test.go") {
					continue
				}
				packageNames := importedNames(file, rule.Package)
				ast.Inspect(file, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel == nil || sel.Sel.Name != rule.Function {
						return true
					}
					if !isPackageFunctionSelector(pass.TypesInfo, packageNames, rule.Package, rule.Function, sel) {
						return true
					}
					diagnostic := analysis.Diagnostic{Pos: call.Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message}
					if rule.Suggestion != "" {
						diagnostic.SuggestedFixes = []analysis.SuggestedFix{{Message: rule.Suggestion}}
					}
					pass.Report(diagnostic)
					return true
				})
			}
			return nil, nil
		},
	}
}

// isIdent 判断表达式是否为指定标识符。
func isIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

// intLiteralValue 提取整数字面量值；非字面量返回 false。
func intLiteralValue(expr ast.Expr) (int, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	value, err := strconv.Atoi(lit.Value)
	return value, err == nil
}

// valueSpecUsesIota 判断 const value spec 中是否直接或间接出现 iota。
func valueSpecUsesIota(spec *ast.ValueSpec) bool {
	for _, value := range spec.Values {
		if exprUsesIota(value) {
			return true
		}
	}
	return false
}

// exprUsesIota 在表达式子树里查找 iota 标识符。
func exprUsesIota(expr ast.Expr) bool {
	uses := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && ident.Name == "iota" {
			uses = true
			return false
		}
		return true
	})
	return uses
}

// firstConstNameAllowsZero 判断 const 组第一项名称是否表达“无效/未知/零值”语义。
// 这是 enum-start-one 的低误报白名单，不要求团队固定使用某一个命名。
func firstConstNameAllowsZero(spec *ast.ValueSpec) bool {
	if len(spec.Names) == 0 {
		return false
	}
	name := strings.ToLower(spec.Names[0].Name)
	return strings.Contains(name, "unknown") || strings.Contains(name, "invalid") || strings.Contains(name, "unspecified") || strings.Contains(name, "none") || strings.Contains(name, "zero")
}

// enclosingFunctionName 返回某个位置所在的函数名，用于判断 os.Exit 是否在 main() 内。
func enclosingFunctionName(file *ast.File, pos token.Pos) string {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil {
			continue
		}
		if fn.Pos() <= pos && pos <= fn.End() {
			return fn.Name.Name
		}
	}
	return ""
}

// selectorExprString 把简单 SelectorExpr 转成 `pkg.Name` 形式，供保守白名单判断使用。
func selectorExprString(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return ""
	}
	if ident, ok := sel.X.(*ast.Ident); ok {
		return ident.Name + "." + sel.Sel.Name
	}
	return sel.Sel.Name
}

// runAnalyzer 以 go/analysis.Pass 形式运行单个 analyzer，并把诊断归一为 Result。
//
// 当前实现按 workdir 收集非排除 Go 文件，一次性解析并做轻量 type-check。types.Config 的
// Error 回调吞掉类型错误，是为了让语义规则在存在局部编译错误的 fixture/工作区里仍能尽量
// 运行；具体语法解析错误仍会作为当前 semantic step 的失败返回。
func (a SemanticAdapter) runAnalyzer(analyzer *analysis.Analyzer, meta semanticRuleMeta, stepCtx StepContext) (Result, error) {
	files, err := goFiles(resolveWorkdir(stepCtx.ProjectRoot, a.cfg.Workdir), projectExcludes(stepCtx.Config))
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: meta.RuleID, Kind: ResultArtifact, Message: fmt.Sprintf("%s skipped; no non-excluded Go files", analyzer.Name), FixSafety: a.cfg.FixSafety, GateStatus: config.GatePass}, nil
	}
	fset := token.NewFileSet()
	parsedFiles := make([]*ast.File, 0, len(files))
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: meta.RuleID, Kind: ResultViolation, File: relPath(stepCtx.ProjectRoot, file), Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
		}
		parsedFiles = append(parsedFiles, parsed)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	typesCfg := types.Config{Importer: importer.Default(), Error: func(error) {}}
	pkg, _ := typesCfg.Check("semanticfixture", fset, parsedFiles, info)

	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{
		Analyzer:  analyzer,
		Fset:      fset,
		Files:     parsedFiles,
		Pkg:       pkg,
		TypesInfo: info,
		Report: func(diagnostic analysis.Diagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
		ReadFile: os.ReadFile,
	}
	if _, err := analyzer.Run(pass); err != nil {
		return Result{}, err
	}
	if len(diagnostics) == 0 {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: meta.RuleID, Kind: ResultArtifact, Message: meta.PassMessage, FixSafety: a.cfg.FixSafety, GateStatus: config.GatePass}, nil
	}
	diagnostic := diagnostics[0]
	pos := fset.Position(diagnostic.Pos)
	return Result{
		AdapterID:    a.cfg.ID,
		StepID:       stepCtx.Step.ID,
		RuleID:       diagnosticRuleID(diagnostic, meta.RuleID),
		Kind:         ResultViolation,
		File:         relPath(stepCtx.ProjectRoot, pos.Filename),
		Line:         pos.Line,
		Column:       pos.Column,
		Message:      diagnostic.Message,
		Suggestion:   diagnosticSuggestion(diagnostic, meta),
		FixAvailable: meta.FixAvailable,
		FixSafety:    a.cfg.FixSafety,
		GateStatus:   config.GateFail,
	}, nil
}

// diagnosticRuleID 优先使用 analyzer 诊断 Category 中携带的 rule_id，缺省时回退到 meta。
func diagnosticRuleID(diagnostic analysis.Diagnostic, fallback string) string {
	if strings.TrimSpace(diagnostic.Category) != "" {
		return semanticRuleID(diagnostic.Category)
	}
	return fallback
}

// diagnosticSuggestion 归一修复建议文本。
// meta.SuggestionFor 允许规则按诊断动态生成建议；否则使用 analysis.SuggestedFix.Message。
func diagnosticSuggestion(diagnostic analysis.Diagnostic, meta semanticRuleMeta) string {
	if meta.SuggestionFor != nil {
		if suggestion := meta.SuggestionFor(diagnostic); suggestion != "" {
			return suggestion
		}
	}
	for _, fix := range diagnostic.SuggestedFixes {
		if strings.TrimSpace(fix.Message) != "" {
			return fix.Message
		}
	}
	return ""
}

// semanticBuiltInRuleName 规范化内置规则名，空值兼容为历史默认 no-direct-os-getenv。
func semanticBuiltInRuleName(rule string) string {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "no-direct-os-getenv"
	}
	return rule
}

// semanticConfig 是 default.yaml 与 custom.yaml 合并后的运行配置。
type semanticConfig struct {
	BuiltInRules []string
	CustomRules  []semanticCustomRule
}

// semanticCustomRule 表示 custom.yaml 中一条参数化自定义规则。
// 不同 kind 会使用不同字段，例如 no-direct-call 使用 Package/Function，max-params 使用 Max。
type semanticCustomRule struct {
	ID         string
	Kind       string
	Package    string
	Function   string
	Max        int
	Message    string
	Suggestion string
}

// semanticConfig 加载 .go-review/semantic/default.yaml 与 custom.yaml。
//
// 文件名已经区分默认规则和自定义规则，所以两个文件都统一使用 `rules:` 顶层字段：
// default.yaml 的 rules 是字符串列表；custom.yaml 的 rules 是对象块列表。
func (a SemanticAdapter) semanticConfig(stepCtx StepContext) (semanticConfig, error) {
	if strings.TrimSpace(a.cfg.Parser) != "" {
		return semanticConfig{}, fmt.Errorf("go.semantic does not support adapter parser; configure built-in rules in semantic/default.yaml")
	}
	configDir := filepath.Dir(stepCtx.ConfigPath)
	paths := []struct {
		path string
		kind semanticConfigKind
	}{
		{path: filepath.Join(configDir, "semantic", "default.yaml"), kind: semanticConfigBuiltInRules},
		{path: filepath.Join(configDir, "semantic", "custom.yaml"), kind: semanticConfigCustomRules},
	}
	var merged semanticConfig
	for _, source := range paths {
		loaded, err := loadSemanticConfig(source.path, source.kind)
		if err != nil {
			return semanticConfig{}, err
		}
		merged.BuiltInRules = append(merged.BuiltInRules, loaded.BuiltInRules...)
		merged.CustomRules = append(merged.CustomRules, loaded.CustomRules...)
	}
	if len(merged.BuiltInRules) == 0 && len(merged.CustomRules) == 0 {
		merged.BuiltInRules = []string{"no-direct-os-getenv"}
	}
	return merged, nil
}

// semanticConfigKind 标记当前解析的是内置规则文件还是自定义规则文件。
type semanticConfigKind int

const (
	semanticConfigBuiltInRules semanticConfigKind = iota
	semanticConfigCustomRules
)

// loadSemanticConfig 读取一个 semantic YAML 文件。
//
// 这里没有引入完整 YAML 依赖，而是实现项目需要的轻量子集：注释、缩进、顶层 rules、
// 字符串列表和对象块列表。这样默认配置可读，同时避免把配置解析复杂度扩大到通用 YAML。
func loadSemanticConfig(path string, kind semanticConfigKind) (semanticConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return semanticConfig{}, nil
	}
	if err != nil {
		return semanticConfig{}, err
	}
	lines, err := readSemanticConfigLines(path, data)
	if err != nil {
		return semanticConfig{}, err
	}
	var cfg semanticConfig
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line.indent != 0 {
			return semanticConfig{}, fmt.Errorf("%s:%d: expected top-level semantic config field", path, line.num)
		}
		key, values, ok := parseSemanticListHeader(line.text)
		if !ok {
			return semanticConfig{}, fmt.Errorf("%s:%d: expected semantic config field", path, line.num)
		}
		switch key {
		case "rules":
			if kind == semanticConfigBuiltInRules {
				cfg.BuiltInRules = append(cfg.BuiltInRules, values...)
				if len(values) == 0 {
					more, next, err := parseSemanticBuiltInRuleItems(path, lines, i+1, line.indent)
					if err != nil {
						return semanticConfig{}, err
					}
					cfg.BuiltInRules = append(cfg.BuiltInRules, more...)
					i = next - 1
				}
				continue
			}
			if len(values) != 0 {
				return semanticConfig{}, fmt.Errorf("%s:%d: custom semantic rules must be a block list", path, line.num)
			}
			rules, next, err := parseSemanticCustomRuleItems(path, lines, i+1, line.indent)
			if err != nil {
				return semanticConfig{}, err
			}
			cfg.CustomRules = append(cfg.CustomRules, rules...)
			i = next - 1
		default:
			return semanticConfig{}, fmt.Errorf("%s:%d: unsupported semantic config field %q", path, line.num, key)
		}
	}
	return cfg, nil
}

// semanticConfigLine 保存去注释后的有效配置行和缩进，用于轻量 YAML 子集解析。
type semanticConfigLine struct {
	num    int
	indent int
	text   string
}

// readSemanticConfigLines 去掉空行和注释，并拒绝 tab 缩进。
func readSemanticConfigLines(path string, data []byte) ([]semanticConfigLine, error) {
	var lines []semanticConfigLine
	for lineNo, raw := range strings.Split(string(data), "\n") {
		raw = stripSemanticComment(raw)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if strings.Contains(raw, "\t") {
			return nil, fmt.Errorf("%s:%d: tabs are not supported in YAML indentation", path, lineNo+1)
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		lines = append(lines, semanticConfigLine{num: lineNo + 1, indent: indent, text: strings.TrimSpace(raw)})
	}
	return lines, nil
}

// parseSemanticBuiltInRuleItems 解析 default.yaml 中的块状规则名列表。
func parseSemanticBuiltInRuleItems(path string, lines []semanticConfigLine, start, parentIndent int) ([]string, int, error) {
	var rules []string
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		line := lines[i]
		if !strings.HasPrefix(line.text, "- ") {
			return nil, i, fmt.Errorf("%s:%d: expected built-in rules list item", path, line.num)
		}
		value := cleanSemanticValue(strings.TrimSpace(strings.TrimPrefix(line.text, "- ")))
		if value != "" {
			rules = append(rules, value)
		}
		i++
	}
	return rules, i, nil
}

// parseSemanticCustomRuleItems 解析 custom.yaml 中的规则对象列表，并逐条校验必填字段。
func parseSemanticCustomRuleItems(path string, lines []semanticConfigLine, start, parentIndent int) ([]semanticCustomRule, int, error) {
	var rules []semanticCustomRule
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		line := lines[i]
		if !strings.HasPrefix(line.text, "- ") {
			return nil, i, fmt.Errorf("%s:%d: expected custom rules list item", path, line.num)
		}
		rule := semanticCustomRule{}
		next, err := parseSemanticCustomRuleItem(path, lines, i, &rule)
		if err != nil {
			return nil, i, err
		}
		if err := validateSemanticCustomRule(path, line.num, rule); err != nil {
			return nil, i, err
		}
		rules = append(rules, rule)
		i = next
	}
	return rules, i, nil
}

// parseSemanticCustomRuleItem 解析一条自定义规则对象。
// 支持 `pkg`/`func` 作为 package/function 的短别名，便于用户书写。
func parseSemanticCustomRuleItem(path string, lines []semanticConfigLine, start int, rule *semanticCustomRule) (int, error) {
	itemIndent := lines[start].indent
	fields := []semanticConfigLine{{num: lines[start].num, indent: itemIndent + 2, text: strings.TrimSpace(strings.TrimPrefix(lines[start].text, "- "))}}
	i := start + 1
	for i < len(lines) && lines[i].indent > itemIndent {
		fields = append(fields, lines[i])
		i++
	}
	for _, field := range fields {
		if field.text == "" {
			continue
		}
		key, val, ok := parseSemanticField(field.text)
		if !ok {
			return i, fmt.Errorf("%s:%d: expected custom rule field", path, field.num)
		}
		switch key {
		case "id":
			rule.ID = cleanSemanticValue(val)
		case "kind":
			rule.Kind = cleanSemanticValue(val)
		case "package", "pkg":
			rule.Package = cleanSemanticValue(val)
		case "function", "func":
			rule.Function = cleanSemanticValue(val)
		case "max":
			parsed, err := parseSemanticPositiveInt(val)
			if err != nil {
				return i, fmt.Errorf("%s:%d: max must be a positive integer", path, field.num)
			}
			rule.Max = parsed
		case "message":
			rule.Message = cleanSemanticValue(val)
		case "suggestion":
			rule.Suggestion = cleanSemanticValue(val)
		default:
			return i, fmt.Errorf("%s:%d: unsupported custom rule field %q", path, field.num, key)
		}
	}
	return i, nil
}

// validateSemanticCustomRule 按 kind 校验自定义规则必需参数。
func validateSemanticCustomRule(path string, lineNo int, rule semanticCustomRule) error {
	if strings.TrimSpace(rule.ID) == "" {
		return fmt.Errorf("%s:%d: custom rule missing id", path, lineNo)
	}
	kind := semanticCustomKind(rule.Kind)
	if _, ok := semanticCustomAnalyzers[kind]; !ok {
		return fmt.Errorf("%s:%d: unsupported custom rule kind %q", path, lineNo, kind)
	}
	switch kind {
	case "no-direct-call":
		if strings.TrimSpace(rule.Package) == "" || strings.TrimSpace(rule.Function) == "" {
			return fmt.Errorf("%s:%d: custom no-direct-call rule requires package and function", path, lineNo)
		}
	case "max-params":
		if rule.Max <= 0 {
			return fmt.Errorf("%s:%d: custom max-params rule requires positive max", path, lineNo)
		}
	}
	return nil
}

// parseSemanticPositiveInt 解析 max 等必须为正整数的配置字段。
func parseSemanticPositiveInt(value string) (int, error) {
	value = cleanSemanticValue(value)
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid positive integer %q", value)
	}
	return n, nil
}

// parseSemanticField 解析 `key: value` 字段行。
func parseSemanticField(text string) (string, string, bool) {
	idx := strings.Index(text, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+1:]), true
}

// parseSemanticListHeader 解析顶层 `rules:`、`rules: [a, b]` 或 `rules: a`。
func parseSemanticListHeader(line string) (string, []string, bool) {
	key, rest, ok := strings.Cut(line, ":")
	if !ok {
		return "", nil, false
	}
	key = strings.TrimSpace(key)
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return key, nil, true
	}
	if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(rest, "["), "]"))
		if inner == "" {
			return key, nil, true
		}
		parts := strings.Split(inner, ",")
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			if value := cleanSemanticValue(part); value != "" {
				values = append(values, value)
			}
		}
		return key, values, true
	}
	if value := cleanSemanticValue(rest); value != "" {
		return key, []string{value}, true
	}
	return key, nil, true
}

// cleanSemanticValue 去掉配置值两侧空白和简单引号。
func cleanSemanticValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"'")
}

// stripSemanticComment 删除引号外的 # 注释，保留字符串字面量中的 #。
func stripSemanticComment(s string) string {
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

// semanticCustomKind 规范化自定义规则 kind；空值兼容为 no-direct-call。
func semanticCustomKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "no-direct-call"
	}
	return kind
}

// semanticConfigFailure 生成配置错误结果，统一挂到 semantic.config。
func (a SemanticAdapter) semanticConfigFailure(stepCtx StepContext, message string) Result {
	return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.config", Kind: ResultViolation, Message: message, FixSafety: config.FixNone, GateStatus: config.GateFail}
}

// semanticRuleID 规范化语义规则 ID。
// 已包含命名空间的 ID 原样保留；简单 ID 自动加 semantic. 前缀，保证报告稳定可追踪。
func semanticRuleID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "semantic.custom"
	}
	if strings.Contains(id, ".") {
		return id
	}
	return "semantic." + id
}

// analyzerName 把 rule_id 转成 go/analysis 合法 Analyzer.Name。
func analyzerName(id string) string {
	id = strings.TrimPrefix(semanticRuleID(id), "semantic.")
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "semantic_" + name
	}
	return name
}

// isPackageFunctionSelector 判断 selector 是否指向指定 package.function。
// 优先使用 type checker 的对象信息；当类型信息不可用时，回退到当前文件 import 名称表。
func isPackageFunctionSelector(info *types.Info, packageNames map[string]struct{}, packagePath, function string, sel *ast.SelectorExpr) bool {
	if info != nil {
		obj := info.Uses[sel.Sel]
		if obj != nil && obj.Pkg() != nil {
			return obj.Pkg().Path() == packagePath && obj.Name() == function
		}
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = packageNames[ident.Name]
	return ok
}

// importedNames 返回某个 import path 在当前文件中可使用的包名前缀。
// 点导入和空白导入没有 selector 前缀，不能用于 package.function 匹配，因此跳过。
func importedNames(file *ast.File, importPath string) map[string]struct{} {
	names := map[string]struct{}{}
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, "\"") != importPath {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "." || spec.Name.Name == "_" {
				continue
			}
			names[spec.Name.Name] = struct{}{}
			continue
		}
		names[path.Base(importPath)] = struct{}{}
	}
	return names
}
