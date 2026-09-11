# 知识索引

由 `keel sync` 生成，勿手改；改 `.keel/` 下的源对象。
状态含义与验证方式见 `docs/design/formats.md`。

现行：决策 10 · 规则 1 · 记忆 0。历史 0 条见文末。

## 决策

| ID | 状态 | 标题 | scope | 证据 |
|---|---|---|---|---|
| D-0a66f368 | accepted | scripts/check/ 放规则的检查脚本，argv 直接指它 | scripts/** | 无 |
| D-375c3bbd | accepted | 模板导入只带可复用配置与技能；规则一律降级为 candidate | internal/template/surface.go、internal/cli/init.go、internal/cli/template.go | 无 |
| D-38a4f854 | accepted | 模板更新用三方比较：两边都改过的文件一个字节都不写 | internal/template/plan.go、internal/template/state.go、internal/template/compare.go | 无 |
| D-4cf3bac4 | accepted | 知识索引分现行/历史两层，淘汰走显式命令 | internal/render/knowledge.go、internal/store/state.go、internal/store/config.go、internal/cli/archive.go、internal/review/review.go | 无 |
| D-6401b385 | accepted | 证据只能由 keel 真的跑完一次验证器产生；check 只推导不改写状态 | internal/check/memory.go、internal/store/digest.go、internal/store/trust.go、internal/verify/** | 1 条（有对得上的 pass） |
| D-8dba8b14 | accepted | M7 优雅退出：keel deinit 只带走自己写过的东西 | internal/cli/deinit.go、internal/render/**、internal/gitx/hooks.go | 无 |
| D-aad57f96 | accepted | git hook 必须传播 keel check 的退出码，不得用 \|\| true 吞掉 | internal/gitx/hooks.go、templates/** | 无 |
| D-bfd5ee1e | accepted | 规则要过对照验证才能生效；review 与自动触发的入口不执行规则 | internal/review/**、internal/verify/promote.go、internal/check/objects.go、internal/cli/promote.go | 1 条（有对得上的 pass） |
| D-d123fcee | accepted | 补全与自更新：spec 一处来源 + 校验和必对 | internal/cli/**、internal/selfupdate/**、internal/ui/progress.go、install.sh、install.ps1、Makefile | 2 条（有对得上的 pass） |
| D-febf95c3 | accepted | 用 GitHub Release 发预编译单二进制，安装脚本从 Releases 拉 | docs/release.md | 无 |

## 规则

| ID | 状态 | 标题 | scope | 依据 | 对照验证 |
|---|---|---|---|---|---|
| R-c1d0e5a7 | active | 生成的 git hook 不得用 \|\| true 吞掉退出码 | internal/gitx/hooks.go、templates/** | D-aad57f96 | pass×1 fail×1 · 已通过 |

## 记忆

（还没有现行记忆。）

## 历史

（还没有被替代、撤回或归档的结论。）
