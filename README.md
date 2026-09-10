# keel

**给 vibe coding 装一根龙骨。**

keel 把项目目标、开发约定、任务经验和验证证据保存在 git 仓库里，为 Claude Code / Codex
提供一致、可追溯的工作上下文，并用可验证、可撤回的规则与技能更新减少重复犯错。

- 仓库即大脑：长期状态是 git 里的 Markdown + YAML。无服务、无账号、无数据库。核心命令离线。
- CLI 是笨的，agent 是聪明的：keel 内部不调 LLM；判断交给宿主 agent。
- 进化必须可验证、可撤回：任务经验 → 学习候选 → 对照验证 → 按既有策略生效 → 回归时撤回。
  扩大自决范围是有证据之后的结果，不是记录条数的副作用。

```
keel init        keel decide "…" --tag db --scope 'internal/store/**'
keel sync        keel why --path internal/store/
keel check       keel note "…" --tag db
keel brief       keel review
```

状态：设计阶段（v2，已吸收架构审查）。设计见 [docs/design/design.md](docs/design/design.md)，
格式与命令规格见 [docs/design/formats.md](docs/design/formats.md)，
审查原文见 [docs/design/architecture-review.md](docs/design/architecture-review.md)。
