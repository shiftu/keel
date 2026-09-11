---
schema: 1
id: D-8dba8b14-7075-43a9-ab48-f8fd5fdc0f60
title: M7 优雅退出：keel deinit 只带走自己写过的东西
status: accepted
date: "2026-09-10"
by: agent:claude
tags:
  - cli
  - render
  - gitx
scope:
  - internal/cli/deinit.go
  - internal/render/**
  - internal/gitx/hooks.go
supersedes: []
superseded_by: []
rule_migration: []
confidence: null
review_after: "2026-12-09"
evidence: []
---

## 背景

keel 往仓库里写的东西散在四处：git hook、CLAUDE.md / AGENTS.md 标记块、settings.json / hooks.json 里的 hook 条目、.mcp.json 的键、两份技能副本、.keel/ 本身。没有退出命令时，手工退出要对着 generated.yaml 一处处找，漏一处就留下一个指向不存在的二进制的 hook。一个工具能不能被放心装进仓库，取决于卸掉它有多干净。

## 决定

新增 `keel deinit [--purge] [--dry-run]`，作为 `init` 的严格逆操作，只带走 keel 自己写过的东西：

- 所有权唯一来源是 generated.yaml（digest + owned_json）和 hook 里的 `# keel:begin` 标记，不猜。
- 顺序：产物 → git hook → .keel/cache/ → .keel/。先撤依赖清单的部分，最后才碰清单所在目录，保证可重入。
- 默认保留 .keel/：决策历史是项目的记录，不是工具的产物。`--purge` 才删，且要求 .keel/ 在 git 里干净，没有跳过开关。
- 托管内容被人改过就报冲突、一个文件都不写，与 sync 同一条规则。
- 不动 keel.yaml、CI 配置、hook 管理器配置、git 历史。

## 备选与理由

- 叫 `uninstall`：拒绝。那个词在 CLI 里指卸二进制，二进制归 `keel update` 和包管理器管；`deinit` 对 `init`，语义同 `git submodule deinit`。
- 默认删除 .keel/：拒绝。「不想再跑 pre-commit」不该顺带变成「删掉三年架构决策」，§8.1「不删除失败历史」在退出时同样成立。
- 交互 y/N 确认：拒绝。keel 每个命令都是先规划再写入，`--dry-run` 就是确认；加 y/N 只会让脚本里多一个 `--yes`。
- 退出时顺手把 keel.yaml 的 tools 清空以防 sync 复活产物：拒绝。keel.yaml 是人写的策略；留着它，`init` 接回来时 tools 和 mcp 都还在。

## 后果与验证方式

- `render.removeOwned` 补齐 JSON hook 条目与 JSON 对象键两种模式的逆操作，并修掉标记块拼接会把两行粘成一行的老 bug；`render.Apply` 删文件后收走空目录。
- `gitx.PlanRemoveHooks` / `RemoveHooks`：restore / delete / trim / keep 四种去向；keel 段外有手写内容且 .keel-local 也在时报出来，不替人合并。
- 验证：`go test ./...` 通过；四个 testscript 夹具——deinit-keeps-keel（块外内容与用户 hook 条目原样、幂等、init 能接回）、deinit-purge（未提交拒绝、提交后退干净）、deinit-restores-chained-hook（原脚本一字不差回位、别人的 hook 不动）、deinit-conflict-writes-nothing（退出码 1 且零写入）。
