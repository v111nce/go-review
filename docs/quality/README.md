# Quality Overview

质量文档定义通用 Go code-review 编排平台自身和被治理项目的验证承诺。当前重点是把 adapter、pipeline、格式检查、静态检查、架构约束、语义规则、自动修复和回归门禁做成可验证基线。

## 文档索引

| 文档 | 内容 | 状态 |
| --- | --- | --- |
| [go-code-quality-baseline.md](go-code-quality-baseline.md) | Go 代码质量检查基线和工具矩阵 | draft |
| [go-rule-catalog.md](go-rule-catalog.md) | Go 官方 / Google / Uber 规则 catalog、自动化分流和落地优先级 | draft |
| [e2e-coverage-baseline.md](e2e-coverage-baseline.md) | 当前无 UI 面时的 E2E 覆盖占位基线 | draft |
| [definition-of-done.md](definition-of-done.md) | 规约治理能力完成定义 | draft |

## 质量原则

- 确定性工具先于 AI 判断。
- 可自动修复的问题必须能被测试证明。
- 架构违规以阻断和解释为主，不默认自动搬迁代码。
- 规则收紧应渐进执行，避免一次性引入大量不可处理失败。
