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
keel verify M-xxxx -- go test ./...          真跑一次验证器，把结果记成证据
keel promote R-xxxx                          规则过了对照验证才生效
keel retire R-xxxx --reason "…"              撤回一条规则，保留原因与历史
keel archive M-xxxx --reason "…"             归档一条记忆，保留原因与历史
keel task set --goal "…" --next "…"          跨会话、跨工具的任务交接
keel brief --task "…" --path <路径>           任务相关的上下文包
keel review                                  到期、失效、学习候选
keel check --target index                    验证真正要提交的内容
```

## 安装

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/shiftu/keel/main/install.sh | bash
```

**Windows（PowerShell）**

```powershell
irm https://raw.githubusercontent.com/shiftu/keel/main/install.ps1 | iex
```

或者到 [Releases](https://github.com/shiftu/keel/releases) 下载对应系统的单个可执行文件，放到 PATH 里。没有任何运行时依赖。

有 Go 的话：`go install github.com/shiftu/keel/cmd/keel@latest`

装完确认一下 `command -v keel` 能找到它 —— keel 装的 git hook 就是靠这个找二进制的，
不在 PATH 里，hook 会静默放行而不是报错。

## 上手

```bash
cd <你的仓库>
keel init                    # 建 .keel/、探测工具、装 git hooks、生成工具文件
```

每个命令的用法、配置项和 CI 用法见 [docs/usage.md](docs/usage.md)。

## 从源码构建

```bash
git clone https://github.com/shiftu/keel && cd keel
make build                   # 产出 ./keel
make install                 # 装到 ~/.local/bin（改 KEEL_INSTALL_DIR 换地方）
make check                   # gofmt + go vet + go test ./...
```

需要 Go 1.27+。发版流程见 [docs/release.md](docs/release.md)。

## 状态

M0（协议）、M1（统一底座）、M2（可信记忆）、M3（受控进化）与 M4（复用与扩展）已实现，`go test ./...` 全绿。

- M2：证据只能由 `keel verify` 真的跑完一次验证器产生；记忆的 verified/disputed/过期由证据推导，
  `check` 只报告不改写；worktree 级任务接续摘要；`knowledge/INDEX.md`。
- M3：规则要过 `keel promote` 的对照验证（pass / fail 两个方向都得有）才能生效，且必须有决策依据；
  `keel retire` 撤回时保留原因与失败历史；规则按 scope 与变更集求交后才执行，
  Stop 与 review 一律不执行规则；`keel review` 给确定性的学习候选簇。

- M4：`keel init --from <git-url>[@<ref>]` 把模板仓库的 `.keel/` 可复用部分导入并把来源钉在一个 commit 上；
  `keel template update` 用三方比较更新，两边都改过的文件一个字节都不写；`sync --codemap` 出目录概览。

- M5：`knowledge/INDEX.md` 分现行/历史两层，历史只出计数，索引不随对象数无限变长；
  `keel archive` 是记忆退场的显式入口；`keel review` 给归档候选和索引压力信号。
  淘汰不是删除，keel 也不替人决定淘汰谁——时间不是证据。
  导入的规则一律落成 candidate——模板给的是建议，要生效仍得在本地 `keel decide` 记依据、`keel promote` 跑对照验证。

更多适配器（claude / codex 之外）未做。技能的对照评估继续后置：它要求宿主在固定任务集上执行任务，
在那套基础设施存在之前做，只会给出没有验证支撑的可信度；M4 只把技能的**版本**定义成模板钉住的那个 commit。
设计见 [docs/design/design.md](docs/design/design.md)，
格式与命令规格见 [docs/design/formats.md](docs/design/formats.md)，
审查原文见 [docs/design/architecture-review.md](docs/design/architecture-review.md)。

## License

MIT，见 [LICENSE](LICENSE)。
