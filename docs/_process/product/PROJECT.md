# Product Process Project

## 背景

用户希望为 Go 项目建立类似阿里巴巴 Java 编码规约的质量治理能力。初始想法包含 code-review agent、定时全量回归和提交触发回归；经过讨论后，当前阶段先忽略 agent，优先落地工具化规约、检测和安全自动修复。

## 当前产品定位

Go code-review 编排平台：通过常用 adapter、通用 `cmd` adapter 和团队自定义 adapter 接入任意工具，检测项目架构、工程目录和代码规范问题，并在安全场景下自动修复。

## 已确认方向

- 先文档化产品能力和质量基线。
- 工具化规约优先于 AI agent。
- 核心能力升级为工具无关 adapter 和 review pipeline 编排。
- 通用规则通过常用 adapter 复用成熟工具。
- 团队特定规则通过自定义 adapter 承载。
- 自动修复只覆盖语义安全子集。
