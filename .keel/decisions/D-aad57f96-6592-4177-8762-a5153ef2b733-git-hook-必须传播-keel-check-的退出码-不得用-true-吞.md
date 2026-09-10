---
schema: 1
id: D-aad57f96-6592-4177-8762-a5153ef2b733
title: git hook 必须传播 keel check 的退出码，不得用 || true 吞掉
status: accepted
date: "2026-09-09"
by: agent:claude
tags:
  - hooks
  - gate
scope:
  - internal/gitx/hooks.go
  - templates/**
supersedes: []
superseded_by: []
rule_migration: []
confidence: null
review_after: "2026-12-08"
evidence: []
---

## 背景

设计 v1 的 pre-commit hook 写的是 `keel check --gate || true`。外部审查复现了它：
门禁形同虚设，任何检查失败都被 `|| true` 吞掉，hook 永远 exit 0。
这是审查列出的第一条 P1，也是整个工具最容易悄悄失效的一处。

## 决定

keel 生成的任何 git hook 都必须把 `keel check` 的退出码原样传出去。
正确形态只有一种：

```sh
if command -v keel >/dev/null 2>&1; then
  keel check --quiet --target index || exit $?
fi
```

`command -v` 判断让没装 keel 的同事不被拦住；`|| exit $?` 保证装了的人真的被拦。
这两件事必须分开表达——用 `|| true` 同时"解决"它们，等于两件都没做。

## 备选与理由

- **`keel check || true`**：审查复现的原始写法。它想表达「keel 没装就别报错」，
  实际表达的是「无论如何都放行」。
- **`set -e` 加裸调用**：hook 脚本里 `set -e` 的行为随 shell 和上下文变化，
  尤其在函数和条件语句里。显式 `|| exit $?` 不依赖任何 shell 选项。
- **不装 hook，只靠 CI**：CI 确实是合并门禁的最终保证，但本地 hook 能在提交时就给出反馈。
  两者都要，不互相替代。

## 后果与验证方式

`docs/design/fixtures/git-hook-exit/` 保留了坏与好两个版本作为审查留痕，
Go 测试 `internal/gitx/hooks_test.go` 的 `TestHookPropagatesFailure` 锁定行为。

M3 起额外由硬规则 `scripts/check/hook-exit-code.sh` 扫描：`internal/` 和 `templates/` 下
不得出现 `|| true`。扫描要排除 `.keel/`（反例夹具）与 `docs/`（审查留痕里就有坏版本）。
