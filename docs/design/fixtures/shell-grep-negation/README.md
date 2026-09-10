# 夹具：`! grep` 在目录不存在时会变成成功

对应 P1-07。旧规则示例：

```
check: "! grep -rlnE 'pgx|lib/pq' internal/store"
```

当 `internal/store` 不存在时，GNU/BSD grep 返回非 0（错误），`!` 将其翻转成 0，硬规则「通过」。

## 文件

- `run.sh` — 在空临时目录跑旧写法与修正写法

修正原则（与 formats.md §4 一致）：

- 优先 `argv` 调已有检查工具
- 若用 grep：先确认路径存在；grep exit 2（或无法打开路径）计为 error，不得 pass
- 无匹配（grep exit 1）才是这条「不得出现导入」规则的 pass

## 期望

```
./run.sh   # exit 0
```

## 已迁移

M0 实现后，这条契约已经变成 Go 测试与 testscript 夹具：
`internal/check/exec_test.go` 的 `TestRunRuleOutcomes`，
以及 `internal/cli/testdata/script/rule-exec-outcomes.txtar`。
规则只接受 `argv`，退出码 0=pass、1=fail、其余=error。
本目录保留为审查原始复现记录。
