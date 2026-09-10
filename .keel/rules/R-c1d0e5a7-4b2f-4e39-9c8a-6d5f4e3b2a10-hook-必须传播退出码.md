---
schema: 1
id: R-c1d0e5a7-4b2f-4e39-9c8a-6d5f4e3b2a10
title: 生成的 git hook 不得用 || true 吞掉退出码
status: active
scope:
  - internal/gitx/hooks.go
  - templates/**
severity: error
check:
  argv:
    - scripts/check/hook-exit-code.sh
  timeout_seconds: 30
from: D-aad57f96-6592-4177-8762-a5153ef2b733
from_template: null
verifier_digest: ""
evidence:
  - E-60d6336b-a428-4cca-8fb4-5935d5e56134
cases:
  - dir: .keel/cases/R-hookexit/pass
    expect: pass
  - dir: .keel/cases/R-hookexit/fail
    expect: fail
---

`|| true` 会把提交门禁变成永远放行。正确写法把「没装 keel」和「检查失败」分开表达：

```sh
if command -v keel >/dev/null 2>&1; then
  keel check --quiet --target index || exit $?
fi
```
