# 项目意图

keel 把项目目标、开发约定、任务经验和验证证据保存在 git 仓库里，为 Claude Code / Codex
提供一致、可追溯的工作上下文，并用可验证、可撤回的规则更新减少重复犯错。

## 目标

- 一个静态二进制、零服务、零账号、零数据库；核心命令离线可用。
- 长期状态只以 git 里的 Markdown + YAML 存在，可 review、可 diff、可回滚。
- 确定性内核：keel 内部不调 LLM，判断交给宿主 agent；工具只做能验证的事。
- 同一份事实同时供给 Claude Code 与 Codex，跨工具接续任务不丢上下文。
- 进化闭环可验证、可撤回：任务经验 → 学习候选 → 对照验证 → 按既有策略生效 → 回归时撤回。

## 非目标

- 不做服务端、不做多租户、不做账号体系、不存任何密钥明文（MCP 配置只声明环境变量名）。
- 不替 agent 做语义判断：不做相似度检索、不做自动总结、不猜项目目标。
- 不按记录条数自动扩大自决范围。`auto_promote` 恒为 false，手写的 `proven` 不提高上限。
- 不接管用户已有的 git hook、CLAUDE.md 段落、settings.json 字段：冲突就报错，不静默覆盖。

## 约束

- Go 单二进制；运行时依赖只有 `gopkg.in/yaml.v3`，测试期可用 `go-internal/testscript`。
- ID 是 `<Kind>-<UUID v4>`，一经产生不重编号；短前缀只能作 CLI 输入，不能进 git trailer。
- 四种检查目标不可互相替代：`worktree` 只诊断、`index` 验证暂存快照、`commit-msg` 验证 trailer、`range` 供 CI。
- 规则执行只走 `check.argv`，不接 shell 字符串；退出码 0=通过、1=失败、其余=错误；超时按进程组杀。
- 业务退出码 0/1/2 与宿主 hook 协议分开：`keel hook` 永远 exit 0 且输出合法 JSON。

## 完成标准

- `go test ./...` 全绿：单元测试 + `internal/cli/testdata/script/*.txtar` 夹具 + git hook 集成测试。
- 新增依赖而没有覆盖它的有效决策时，`keel check --target index` 退出码为 1；空 ADR 与不相关 ID 不算覆盖。
- 暂存坏代码、工作树已修好但未暂存时，`--target worktree` 通过而 `--target index` 失败。
- `keel sync` 幂等；手改托管块产生冲突且一个文件都不写。
- 一侧工具写下的记忆，另一侧的 SessionStart 能拿到，且未验证的标注为「候选」。

## 引用

- [README.md](../README.md) · [docs/usage.md](../docs/usage.md)
- 设计：[docs/design/design.md](../docs/design/design.md) · 规格：[docs/design/formats.md](../docs/design/formats.md)
- 审查原文：[docs/design/architecture-review.md](../docs/design/architecture-review.md)
