---
schema: 1
id: D-bfd5ee1e-6016-4b0d-8e0b-ddb8d9c87a0f
title: 规则要过对照验证才能生效；review 与自动触发的入口不执行规则
status: accepted
date: "2026-09-09"
by: agent:claude
tags:
  - evolution
  - rules
scope:
  - internal/review/**
  - internal/verify/promote.go
  - internal/check/objects.go
  - internal/cli/promote.go
supersedes: []
superseded_by: []
rule_migration: []
confidence: null
review_after: "2026-12-08"
evidence:
  - E-faf43b13-bf05-4bb5-b719-d9509002e8ba
---

## 背景

M2 之前规则只有一条路径：有人手写 `status: active`，从此它拦所有人。没有对照验证，
也没有撤回入口。审查里「自动进化必须回答：改什么、怎样验证、何时生效、失败怎么撤回」，
这四问在规则上一个都没答。

另外两个执行面的问题：Stop hook 每轮自动触发却会执行仓库里的规则脚本；
一条 scope 在 `internal/store/**` 的规则，改前端时照样跑。

## 决定

规则从候选到生效只有 `keel promote` 一条路，前置条件缺一不可：`candidate` 状态、
`check.argv`、指向有效决策的 `from`、以及 `cases` 里 **pass 和 fail 两个方向都有**的夹具。
逐条跑完全部符合预期才转 active，并写下证据；不符合就留在 candidate，**失败证据照样写进仓库**。

撤回只有 `keel retire --reason`：转 retired，原因写进正文，历史保留，
被它替代过的旧规则只提示不自动改。

规则执行只发生在人显式发起的验证里（worktree / index / range），
且只跑 scope 与本次变更集有交集的那些。`review`、`brief`、SessionStart、Stop 一律不执行。

## 备选与理由

- **只要求 `expect: pass` 的用例**：省一半夹具，但一条永远 `exit 0` 的检查也能满足，
  等于没验证。反例方向才是对照验证的全部意义。
- **允许没有 `from` 的规则 promote**：更灵活，但一条会拦住所有人提交的硬规则，
  如果没人能说出「为什么」，它迟早被人用 `--no-verify` 绕过去。
- **`review` 顺带跑一遍规则，报告里给出当前是否通过**：信息更全，
  但「看一眼有什么要维护的」就变成执行仓库里的代码——审查 §5 专门提过这条风险。
  规则过不过，让人显式跑 `keel check`。
- **`check` 发现规则失败就自动 retire**：省事，但那是把撤回权交给一次失败的运行。
  撤回要留原因，原因得有人写。

## 后果与验证方式

夹具是 `.keel/cases/` 下的真目录，可 review、可 diff、跨 clone 可重跑。
`expect: fail` 里放的是故意违规的代码，所以全仓库扫描型的检查脚本必须排除 `.keel/`——
否则规则会被自己的反例夹具绊倒。这一条在写第一个夹具时就撞上了。

技能版本与对照评估后置到 M4：它要求宿主在固定任务集和固定项目快照上执行任务，
在那套基础设施存在之前做，只会给出没有验证支撑的可信度。

验证：`go test ./...` 全绿，其中 `rule-promote`、`rule-promote-rejected`、
`rule-scope-gating`、`rule-retire-and-review`、`review-clusters`、
`stop-does-not-run-rules` 六个夹具锁定上述行为。
