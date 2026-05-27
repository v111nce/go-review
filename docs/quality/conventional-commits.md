# Conventional Commits 中文提交规范

## 核心规则

本项目采用 [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) 格式，但提交说明必须使用中文。

提交标题格式：

```text
<type>[optional scope]: <中文说明>
```

允许英文保留项：

- `type` 关键字，例如 `feat`、`fix`、`docs`
- 可选 `scope`，例如 `agent`、`sidecar`、`web`
- `BREAKING CHANGE` footer 关键字
- 项目约定的 git trailer key，例如 `Constraint`、`Rejected`、`Confidence`、`Scope-risk`、`Directive`、`Tested`、`Not-tested`
- 代码标识符、命令、文件路径、API 名称和专有名词

除此之外，标题、正文和 trailer 内容默认使用中文。

## 常用 type

| type | 使用场景 | 示例 |
| --- | --- | --- |
| `feat` | 新功能 | `feat(agent): 支持按 Agent 绑定模型配置` |
| `fix` | 修复缺陷 | `fix(sidecar): 修复自定义模型 provider 推断失败` |
| `docs` | 只改文档 | `docs(deepagent): 更新 sidecar 启动说明` |
| `test` | 增加或调整测试 | `test(agent): 覆盖模型配置写入 BrainInput` |
| `refactor` | 重构但不改变行为 | `refactor(api): 简化 provider 请求解析` |
| `style` | 格式、排版、命名等不影响逻辑的调整 | `style(web): 统一详情页 select 样式` |
| `chore` | 杂项维护 | `chore(deps): 更新本地工具配置` |
| `build` | 构建系统或依赖管理 | `build(web): 调整 Vite 构建配置` |
| `ci` | CI 配置 | `ci(test): 增加前端类型检查步骤` |
| `perf` | 性能优化 | `perf(agent): 减少 provider 重建次数` |
| `revert` | 回滚提交 | `revert: 回滚 Agent 详情页布局调整` |

## scope 规则

scope 可选。改动范围清晰时建议写 scope：

```text
feat(agent): 支持按 Agent 绑定模型配置
fix(sidecar): 修复 provider 测试失败提示
```

scope 使用英文短词，优先选择模块名、包名或目录名，例如：

- `agent`
- `brain`
- `sidecar`
- `web`
- `api`
- `docs`
- `storage`
- `config`

## 正文和 trailers

复杂提交应保留决策记录正文和 trailers。trailer key 使用项目既有英文 key，内容使用中文：

```text
feat(agent): 支持按 Agent 绑定模型配置

允许每个 AgentInstance 选择自己的模型 provider，同时保持 DeepAgent 作为固定 AgentBrain。

Constraint: API key 只保存在 sidecar 本地 provider store。
Rejected: 所有 Agent 共用一个全局 provider | 会让 provider 切换影响无关 Agent。
Confidence: high
Scope-risk: moderate
Directive: BrainInput 只能携带 modelProviderId，禁止包含 api_key。
Tested: make check-fast；npm --prefix web/control-plane test -- --run。
Not-tested: 两个 Agent 并发连接不同第三方 provider 的真实运行。
```

## 禁止示例

```text
feat: bind model providers per agent
fix: update sidecar provider inference
```

原因：冒号后的说明是英文，不符合“提交说明必须中文”。

## 推荐示例

```text
feat(agent): 支持按 Agent 绑定模型配置
fix(sidecar): 修复自定义模型 provider 推断失败
docs(runtime): 补充 DeepAgent 必填启动步骤
test(api): 覆盖 Agent 编辑 modelProviderId
```
