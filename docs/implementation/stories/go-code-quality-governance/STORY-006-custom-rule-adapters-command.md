# STORY-006 Custom Rule Adapters Command

## 来源

- Product: [Go 代码质量规约治理 / 自定义规约 Adapter](../../../product/go-code-quality-governance.md#自定义规约-adapter)
- Module Key: `custom-rule-adapters`
- Story Key: `custom-rule-adapters-command`
- Backend: [通用工具接入契约](../../../backend/rule-engine-and-autofix.md#通用工具接入契约)
- Quality: [Go 代码质量基线](../../../quality/go-code-quality-baseline.md)

## 目标

团队可以通过通用 `cmd` adapter 接入任意已有工具，让外部工具进入 review pipeline 并影响门禁结果。

## 范围

包含：

- 命令、参数、工作目录、环境变量配置。
- 超时配置。
- 退出码到 gate status 的映射。
- 基础输出 artifact 保存。

不包含：

- 每个工具的专用解析器。
- 远程工具安装。
- 沙箱隔离策略。

## 依赖

- Backend: 通用工具接入契约。
- Quality: 任意工具接入基线。

## 任务

- 支持 `cmd` adapter 配置 command、args、workdir、env。
- 支持 timeout。
- 支持 stdout、stderr 保存为 artifact。
- 支持退出码映射为 pass / fail。
- 支持可选 parser 配置。

## 验收标准

- 一个非内置命令可以进入 pipeline。
- 命令成功时 step 通过。
- 命令失败时 step 失败并保留输出。
- 超时时 step 失败并给出清晰原因。

## 测试

- 使用成功命令、失败命令、超时命令 fixture。
- 验证 stdout、stderr artifact。
- 验证失败 gate status。

## 完成定义

- 通用 `cmd` adapter 能作为非内置工具接入口使用。
