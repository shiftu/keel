# keel

**给 vibe coding 装一根龙骨。**

keel 把项目目标、开发约定、任务经验和验证证据保存在 git 仓库里，为 Claude Code / Codex
提供一致、可追溯的工作上下文，并用可验证、可撤回的规则与技能更新减少重复犯错。

- 仓库即大脑：长期状态是 git 里的 Markdown + YAML。无服务、无账号、无数据库。核心命令离线。
- CLI 是笨的，agent 是聪明的：keel 内部不调 LLM；判断交给宿主 agent。
- 进化必须可验证、可撤回：任务经验 → 学习候选 → 对照验证 → 按既有策略生效 → 回归时撤回。
  扩大自决范围是有证据之后的结果，不是记录条数的副作用。

```
keel init                                    建 .keel/、装 git hooks、生成工具文件
keel sync                                    .keel/ → CLAUDE.md / AGENTS.md / skills / MCP / hooks
keel decide "…" --tag db --scope 'internal/store/**' --status accepted
keel why --path internal/store/db.go         改这里之前先看什么
keel note "…" --tag db                       记一条候选经验
keel brief --task "…" --path <路径>           任务相关的上下文包
keel check --target index                    验证真正要提交的内容
```

安装、上手和每个命令的用法见 [docs/usage.md](docs/usage.md)。

状态：M0（协议）与 M1（统一底座）已实现，`go test ./...` 全绿；M2 起的证据、任务接续与受控进化未做。设计见 [docs/design/design.md](docs/design/design.md)，
格式与命令规格见 [docs/design/formats.md](docs/design/formats.md)，
审查原文见 [docs/design/architecture-review.md](docs/design/architecture-review.md)。
