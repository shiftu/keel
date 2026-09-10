# 夹具：Git hook 不得吞掉 keel check 失败

对应 P1-01。旧规格 `command -v keel && keel check || true` 在 keel 已安装但检查失败时仍返回 0。

本目录用 POSIX sh 复现，不依赖已实现的 keel 二进制。

## 文件

- `keel-fail.sh` — 模拟已安装、检查失败的 keel（exit 1）
- `hook-old.sh` — formats.md 修订前的追加段
- `hook-new.sh` — 修订后的追加段
- `run.sh` — 断言：旧脚本对失败 keel 返回 0；新脚本返回 1；keel 不在 PATH 时两者都返回 0

## 期望

```
./run.sh   # exit 0
```

实现 `init` 安装 hook 时，生成文本必须与 `hook-new.sh` 同构（if + `exit $?`，无尾部 `|| true`）。
另外五种安装情形（已有 hook 失败、非 shell hook、hooksPath、worktree、未安装）见 formats.md §8.1 / §9.5，实现阶段用 testscript 覆盖。

## 已迁移

M1 实现后，这些断言已经变成 Go 测试：`internal/gitx/hooks_test.go`
（`TestHookPropagatesFailure` 覆盖失败传播、未安装放行、执行位；
`TestForeignHookIsNotClobbered` 覆盖已有 shell hook 与 `--adopt-hooks` 串联；
`TestNonShellHookIsReported` 覆盖非 shell hook；
`TestHooksPathIsRespected` 覆盖 `core.hooksPath`）。
本目录保留为审查原始复现记录。
