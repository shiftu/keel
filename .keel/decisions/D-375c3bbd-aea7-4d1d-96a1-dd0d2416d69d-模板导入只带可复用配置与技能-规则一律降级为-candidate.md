---
schema: 1
id: D-375c3bbd-aea7-4d1d-96a1-dd0d2416d69d
title: 模板导入只带可复用配置与技能；规则一律降级为 candidate
status: accepted
date: "2026-09-09"
by: agent:claude
tags:
  - template
  - security
scope:
  - internal/template/surface.go
  - internal/cli/init.go
  - internal/cli/template.go
supersedes: []
superseded_by: []
rule_migration: []
confidence: null
review_after: "2026-12-08"
evidence: []
---

## 背景

M4 要做跨仓库复用。「复制一份 .keel/」听起来简单，但 .keel/ 里的东西性质不同：
技能和策略配置是可复用的建议，决策/记忆/证据是**源仓库的项目事实**，
规则则是**会被执行的代码入口**。

## 决定

导入面只有四样：`keel.yaml`（`tools` 除外，它是本机探测结果）、`skills/**`、`rules/*.md`、`cases/**`。

`decisions/` `memory/` `evidence/` `intent.md` 一律不导入。

导入的规则强制改写成候选形态：`status: candidate`、`evidence: []`、`verifier_digest: ""`、
`from: null`、`from_template: <source>@<commit>`。

## 备选与理由

- **整个 .keel/ 原样复制**：等于把别人的项目历史当成自己的。证据的 `target.content_digest`
  是对着源仓库的代码算的，在这边永远对不上，等于凭空多出一堆「验证过但对不上」的噪音。否决。
- **保留模板里的 `status: active`**：装个模板就等于让别人的仓库决定我这边执行什么代码，
  直接绕开 M3 定的「promote 不能自我批准」。否决。
- **保留指向模板决策的 `from`**：实现时试过，`keel check` 报 `ref_missing`（error），
  刚导完模板就满仓库红字、pre-commit 过不去。本地没有依据就是没有依据；
  出处由 `from_template` 钉住的 commit 保留，去源仓库那一版一看就有。

## 后果与验证方式

要让导入的规则生效，本地必须走完整条路：`keel decide` 记依据 → 改 `from` → `keel promote` 跑对照验证。
这是有意的摩擦。

验证：`internal/cli/testdata/script/template-import.txtar` 断言导入后决策/记忆/证据都不存在、
规则是 candidate 且 evidence 为空、`keel check` 通过、`keel promote` 拒绝。
