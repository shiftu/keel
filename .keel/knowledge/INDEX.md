# 知识索引

由 `keel sync` 生成，勿手改；改 `.keel/` 下的源对象。
状态含义与验证方式见 `docs/design/formats.md`。

## 决策

| ID | 状态 | 标题 | scope | 证据 |
|---|---|---|---|---|
| D-6401b385 | accepted | 证据只能由 keel 真的跑完一次验证器产生；check 只推导不改写状态 | internal/check/memory.go、internal/store/digest.go、internal/store/trust.go、internal/verify/** | 1 条（有对得上的 pass） |
| D-febf95c3 | accepted | 用 GitHub Release 发预编译单二进制，安装脚本从 Releases 拉 | docs/release.md | 无 |

## 规则

（还没有规则。）

## 记忆

（还没有记忆。）
