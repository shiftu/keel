# keel 用法

对应设计：[design.md](design/design.md)。字段与命令规格：[formats.md](design/formats.md)。

当前实现到 M5：`init` / `sync` / `decide` / `why` / `note` / `verify` / `promote` / `retire` /
`archive` / `task` / `template` / `check` / `brief` / `review` / `hook` / `version` 全部可用。
`init --from`、`keel template status|update` 与 `sync --codemap` 都已接上。

## 安装

macOS / Linux：

```sh
curl -fsSL https://raw.githubusercontent.com/shiftu/keel/main/install.sh | bash
```

Windows（PowerShell）：

```powershell
irm https://raw.githubusercontent.com/shiftu/keel/main/install.ps1 | iex
```

脚本把单个二进制放进 `~/.local/bin`（Windows 是 `%LOCALAPPDATA%\keel`，并自动加进用户 PATH）。
换地方用 `KEEL_INSTALL_DIR=/usr/local/bin`，装指定版本用 `KEEL_VERSION=v0.1.0`。
也可以到 [Releases](https://github.com/shiftu/keel/releases) 直接下载，没有任何运行时依赖。

有 Go 的话：`go install github.com/shiftu/keel/cmd/keel@latest`。

从源码构建（需要 Go 1.27+）：

```sh
git clone https://github.com/shiftu/keel && cd keel
make build            # 产出 ./keel
make install          # 装到 $KEEL_INSTALL_DIR，默认 ~/.local/bin
```

**装完确认 `command -v keel` 找得到它。** git hook 里用的是 `command -v keel`，
不在 PATH 里就自动放行 —— 这是为了不拦住没装 keel 的同事，代价是你自己也会被静默跳过。

发版流程见 [release.md](release.md)。

## 快速上手

```sh
cd <你的仓库>
keel init             # 建 .keel/、探测工具、装 git hooks、生成工具文件
```

`init` 之后仓库里多出：

| 路径 | 作用 |
|---|---|
| `.keel/` | 唯一事实来源：intent、决策、规则、记忆、技能、配置、产物清单 |
| `CLAUDE.md` / `AGENTS.md` | 稳定协议块（只在 `<!-- keel:begin -->` 到 `<!-- keel:end -->` 之间） |
| `.claude/` `.codex/` `.agents/` | 技能副本、hook 配置、MCP 配置 |
| `.git/hooks/pre-commit`、`commit-msg` | 提交前验证 |

`init` 结束会打印能力报告。**hook 写进文件不等于已启用**：Claude 侧要一次实际会话确认，
Codex 侧还要走一次原生项目信任流程，所以这两项报 `unknown` 是正常的。

## 一次会话里的顺序

```
接任务时     keel brief --task "给存储层加缓存" --path internal/store/
改代码前     keel why --path internal/store/db.go
做了决策     keel decide "…" --status accepted --body -
踩了坑       keel note "…" --tag storage
能验证就验    keel verify M-<短前缀> -- go test ./internal/store/...
要交接了     keel task set --goal "…" --done "…" --next "…"
提交前       keel check --target index      （pre-commit 会自动跑）
维护时       keel review                    到期、失效、学习候选
```

`brief` 放在最前面：它把接续摘要、工作流上限、适用先例和相关材料一次给全。

## 命令

列表选项（`--tag` / `--scope` / `--path` / `--condition` / `--rule-migration`）**既能重复给也能逗号分隔**：
`--tag a --tag b` 与 `--tag a,b` 等价。重复给不会静默只留最后一个。


### `keel init`

```
keel init [--tools claude,codex] [--from <git-url>[@<ref>]] [--no-hooks] [--adopt-hooks]
```

不给 `--tools` 就探测 PATH。已有 `.keel/` 时只补缺的文件，不覆盖。

`--from` 从模板仓库起步，见下面的 [`keel template`](#keel-template)。

仓库里已经有别人的 `pre-commit` 时，`init` 会**失败并说明怎么接**，不会静默跳过也不会覆盖：

- 是 POSIX shell 脚本 → 用 `--adopt-hooks`，原脚本移到 `pre-commit.keel-local` 并由新 hook 先调用，它失败就整体失败。
- 是 husky / lefthook / pre-commit 框架 → 在它们的配置里加一步 `keel check --target index`。
- 是非 shell 脚本 → keel 不碰，自己加调用。
- 确认不需要本地 hook → `--no-hooks`，合并门禁走 CI。

`core.hooksPath` 和 git worktree 都会被正确识别。

### `keel sync`

```
keel sync [--dry-run] [--codemap]
```

`.keel/` → 工具原生文件。先完整规划再写入：**有任何冲突就一个文件都不写**。

keel 只拥有自己写的那部分：标记块之间的内容、`generated.yaml` 里记过的整文件、
JSON 里它写过的那几条 hook 条目和 MCP 键。块外的内容、你自己加的 hook、
`.claude/settings.json` 里的其他字段，一律原样保留。

托管内容被手工改过时报冲突而不是覆盖。想让改动生效就搬回 `.keel/`。

`--codemap` 这一轮额外生成 `.keel/knowledge/CODEMAP.md`：一张按目录列的仓库地图，
每行是目录、文件数、文件类型。只列目录，没有符号也没有调用关系。
内容来自 `git ls-files`，没 `git add` 的文件不算——两台机器上生成的必须一模一样，
不然每次 sync 都是一个假 diff。

它和别的产物一样归 `generated.yaml` 管，所以**只给一次 `--codemap` 的话，
下一轮普通 `sync` 会把它删掉**（源没了就清理，和别的产物同一个规则）。
要长期留着，在 `keel.yaml` 里写 `knowledge.codemap: true`。

### `keel template`

跨仓库复用一套 keel 配置：模板仓库自己就是个 keel 仓库，你导入它 `.keel/` 的可复用部分。

```
keel init --from <git-url>[@<ref>]      # 首次导入
keel template status                    # 来源、钉住的 commit、本地改过哪些
keel template update [--to <ref>] [--dry-run]
```

导入的是 `keel.yaml`、`skills/**`、`rules/*.md`、`cases/**`。
**不导入** `decisions/` `memory/` `evidence/` `intent.md`——那些是模板仓库的项目事实，
证据的 digest 指着它那边的代码，在你这边永远对不上；`intent.md` 是你自己项目要回答的问题。

`tools` 不跟着模板走，它是本机探测结果。其余策略（工作流上限、gate、MCP、knowledge）照模板。

导入的规则**一律落成 `candidate`**，模板里写的是 `active` 也一样，同时清掉 `evidence` 和 `from`。
模板给的是建议，不是既成事实：要让它生效，你得在本地 `keel decide` 记下自己的理由，
把规则的 `from` 指过去，再 `keel promote` 跑一遍对照验证。装个模板不等于让别人的仓库
决定你这边执行什么代码。出处不会丢——`from_template` 把来源和 commit 钉死了。

**更新只快进安全的那些。** `update` 拿模板新版和「上次导入时那一版」「你现在这一版」三方比较：

| 你改过吗 | 模板改过吗 | 结果 |
|---|---|---|
| 没有 | 改了 | 快进，写入模板新版 |
| 改了 | 没改 | 保留你的，模板那版不会盖上来 |
| 改了 | 也改了 | **冲突，一个字节都不写**，退出码 1 |
| 没有这个文件 | 新增 | 写入 |
| 你删了 | — | 不写回 |
| — | 模板删了 | 不删你的 |

冲突不阻塞其他文件，能快进的照常快进。想要模板那一版，把本地改动挪走再 `update`；
想留本地的，不用管——冲突文件的基线不推进，下次还会提醒你。

`status` 不联网，只回答「本地相对导入基线动过什么」。要知道模板那边有没有新版，
用 `keel template update --dry-run`。

### `keel decide`

```
keel decide "<标题>" --tag <t>… --scope '<glob>'… [--status accepted] [--body -]
            [--supersedes D-xxx] [--rule-migration R-a:retired] [--by agent:claude]
```

默认 `proposed`。`--status accepted` 要求四段正文都非空——空 ADR 不能当门禁的覆盖。

正文从 stdin 读（正文里写清背景、决定、备选与理由、后果与验证方式四段）。
命令会打印 `Decision: D-<完整 uuid>`，把这行放进 commit message。

**替代旧决策**只有在新决策以 `accepted` 落地时才发生；`proposed` 只登记意图，旧决策继续生效。
被替代方有 `active` 派生规则时必须同时给 `--rule-migration`，否则拒绝。任一步失败整笔回滚。

### `keel why`

```
keel why [--path <路径>] [--query <关键词>] [--history]
```

`--path` 按 scope 匹配，也会查 git 历史里该路径的提交引用过哪些决策（`Decision:` trailer）。
`--history` 会带出被否决和被替代的结论——已经排除过的方案不该被重新提出来。

### `keel note`

```
keel note "<一句话>" --tag <t>… [--path f.go]… [--kind gotcha|fact|pointer|counterexample]
          [--condition environment=...] [--by agent:claude]
```

新记忆一律 `candidate`：可检索，但在有证据之前不会被当成项目规则注入。
正文超过 20 行会被拒绝——那说明它其实是一次决策。

### `keel verify`

```
keel verify <M-…|D-…> --rule R-<uuid> [--kind …] [--trust local|ci]
keel verify <M-…|D-…> -- <命令…>     [--id <验证器名>] [--timeout <秒>]
```

跑一次验证器，把结果写成 `.keel/evidence/E-<uuid>.md`，再按结果更新被验证的对象。

- 记忆：通过 → `verified` 并写 `verified_at`；失败 → `disputed`。
- 决策：只追加证据，**不改状态**。`proven` 是人的判断，不是跑通一条命令的自动结果。
- 验证器跑不起来或超时：照实记进证据，但**不做任何状态转换**——跑不起来既不是通过也不是失败。

`--` 之后的参数原样 exec，不经 shell。退出码约定和规则一样：0 通过、1 发现违规、其余是错误。
命令本身退出码 0 时 `keel verify` 退 0，否则退 1；无论如何证据都会写下来。

**没有「登记一条我认为它通过了」的入口。** 自述的通过既不能被别人重跑，也不能被撤回。

证据里记两个摘要：`subject_digest` 盯结论本身，`target.content_digest` 盯 scope 覆盖的代码。
前者对不上 → `memory_status_unsupported`（error，结论改了却还挂着旧证据）；
后者对不上 → `memory_evidence_stale`（提醒，代码变了，验证结果过期）。

### `keel task`

```
keel task set --goal "<目标>" [--done "…"]… [--next "…"]… [--failing "…"]… [--ref <ID>]… [--by agent:claude]
keel task show [--json]
keel task clear
```

同一个 worktree 上 Claude 与 Codex 之间的交接。存在 `.keel/cache/task/current.yaml`，**不进 git**。

- `--goal` / `--next` / `--failing` / `--ref` 给了就整体替换，没给保留原值；`--done` 追加。
- `--ref` 会校验对象存不存在：接不上的引用只会让下一个工具白找一遍。
- 记录写入时的 HEAD。HEAD 往前走之后，`show` 和 `brief` 会照实说「摘要可能已经落后」。
- 摘要出现在 `brief` 和 SessionStart 的最前面，并标明它来自本机缓存。

要跨 clone 留下的经验仍然走 `keel note` / `keel decide`。删掉缓存不会撤销任何已生效的知识。

### `keel promote` / `keel retire`

规则从候选到生效、以及生效之后撤回的唯一两个入口。

```
keel promote <R-…>
keel retire  <R-…> --reason "<原因>"
```

`promote` 跑规则的**对照验证**：把 `check.argv` 分别指着 `cases` 里的每个夹具目录跑，
比对 `expect`。前置条件任一不满足就直接拒绝，一条用例都不跑：

- 状态是 `candidate`；或者是 `active` 但证据已经对不上（那是重新验证，状态不变）
- 必须有 `check.argv`
- 必须有 `from` 指向一条**有效**决策——会拦住所有人提交的硬规则，得有一条说明「为什么」的决策背书
- `cases` 里 `pass` 和 `fail` 两个方向都要有

最后一条最重要：只证明「该过的过了」不算对照验证，一条永远 `exit 0` 的检查也能满足。

跑完写一条证据（subject = 规则 ID）。**失败也写**，规则留在 `candidate`——
失败历史留在仓库里，下次 `keel review` 还看得到。

`cases` 的夹具是 `.keel/cases/` 下的真目录，执行时 cwd 是夹具目录，
`argv[0]` 是仓库内相对路径时按仓库根解析。也就是「同一条规则，指着另一份仓库状态跑」。

`expect: fail` 的夹具里放的是故意违规的代码，**全仓库扫描型的检查脚本要把 `.keel/` 排除掉**，
否则规则会被自己的反例夹具绊倒。

改了 `check.argv`、`cases`、正文，或者 `argv[0]` 指向的检查脚本，
`check` 就会报 `rule_evidence_stale`（提醒）：现在生效的这条规则和当初通过验证的那条不是同一个东西。
重新 `keel promote` 一次即可。

`retire` 转 `retired`，把原因写进正文末尾。有决策的 `rule_migration` 曾把旧规则迁到这条时，
会提示旧规则可以考虑恢复——只提示，不自动改。不删除任何历史。

### `keel archive`

```
keel archive <M-…> --reason "<原因>"
```

记忆退场的显式入口，和规则的 `retire` 对称。转 `archived`，把原因写进正文末尾。

归档不是删除：文件留在 `.keel/memory/`，`keel why --history` 查得到，只是退出知识索引主表
和默认召回。有别的记忆用 `derived_from` 指向它时会提示转述可能也该处理——只提示，不自动改。

**keel 不会自己归档任何东西。** 「过期太久」不构成归档理由：时间不是证据。
要找该归档的，跑 `keel review` 看 `memory_archive_candidate`。

### `keel review`

```
keel review [--json]
```

**只读，且不执行任何规则。** 一次「看看有什么要维护的」不该顺带执行仓库里的代码。
规则当前过不过，跑 `keel check --target worktree`。

四段：到期复查、失效与冲突、学习候选、统计。学习候选是确定性聚类，不是自动写出来的修订：

| code | 含义 |
|---|---|
| `rule_candidate_ready` | 候选规则前置条件齐全，可以 `keel promote` |
| `rule_candidate_incomplete` | 缺 `check.argv` / `from` / 某个方向的用例 |
| `memory_cluster` | 同 scope 同 tag 的多条候选记忆，可能是同一条不变量 |
| `memory_disputed` | 有反例指向它 |
| `memory_archive_candidate` | 过了复查日期，而且从来没有过证据 |
| `index_pressure` | 现行对象超过 `knowledge.index_soft_limit` |
| `decision_reverted` | `git log` 里 revert 提交引用过这条决策 |

`memory_archive_candidate` 的条件比 `memory_review_due` 更紧：**过期且一条证据都没有**。
有证据但过期的那些该重新 `keel verify`，不该归档——两者出路不同。

`index_pressure` 是信号不是门禁，只出现在 `review` 里，不进 `check`，更不进 pre-commit。
现行对象太多不是错误，是体检结果：要么归档一批，要么把一簇记忆提炼成规则。
做成门禁会产生「为了过门禁而删记忆」的反向激励。

**条数只排审阅优先级。** 同一来源的重复转述在 `memory_cluster` 里算一条——
「同一件事换个说法记三遍」不会变成三份依据。

### `keel check`

```
keel check [--target worktree|index|commit-msg|range] [--commit-msg <文件>]
           [--base <rev> --head <rev>] [--json] [--quiet]
```

四个 target 看的是不同快照，**互相不能替代**：

| target | 看什么 | 谁在用 |
|---|---|---|
| `worktree` | 当前工作树，只读诊断 | 人、Stop hook |
| `index` | 暂存区快照（导到临时目录，不碰你的工作树） | pre-commit |
| `commit-msg` | 提交信息的 trailer + 暂存区 | commit-msg hook |
| `range` | `base...head` | CI |

工作树里修好但没暂存，`index` 照样失败。这是有意的。

检查内容：对象完整性（frontmatter、ID 重复、引用、状态一致性）、`active` 硬规则、
以及语义信号——新增直接依赖、新增顶层目录。这些信号需要一条**可用**的决策来解释：
状态有效、四段正文非空、scope 覆盖相关路径。不相关的 ID 和空 ADR 不算。

`--quiet` 通过时不输出任何内容，给 git hook 用。

### `keel brief`

```
keel brief [--task "<要做什么>"] [--path <路径>] [--action <动作类别>] [--budget <字节>] [--json]
```

按相关性挑上下文，每条都带被选中的理由。字节预算是硬约束，超出会显示截断说明而不是静默丢内容。

输出里的**工作流建议上限**由 `keel.yaml` 的 `workflow` 算出来：
`min(default_level, 命中的 actions, 命中的 paths)`。tag 只服务检索，不参与计算，
所以 `db` 下有几条成功先例不会给「删生产数据」放行。

### `keel hook`

内部入口，由 `.claude/settings.json` 与 `.codex/hooks.json` 调用，不用手动跑。
永远 `exit 0` + 合法 JSON；业务 CLI 的 0/1/2 不会泄漏成宿主的生命周期语义。

Stop 事件在第一次发现可修正问题时请求一次继续；同一批问题不会反复要求。
去重只抑制重复反馈——**不会把 fail 改成 pass**，提交检查照拦。

## 配置

`.keel/keel.yaml` 的核心字段严格校验，拼错的键直接报错。

```yaml
version: 1
tools: [claude, codex]

workflow:
  default_level: 1          # 0 先问人 | 1 有适用先例才自决 | 2 在上限内自决并记录
  auto_promote: false       # 必须为 false：keel 不按 proven 计数自动扩大自决范围
  actions:
    dependency.add: 1
    data.delete: 0
    public-api.change: 0
    security: 0
  paths:
    "infra/**": 0

gate:
  manifests: [go.mod, package.json, pyproject.toml, Cargo.toml, "requirements*.txt"]
  watch_top_level_dirs: true

mcp:
  servers:
    gitea:
      command: gitea-mcp
      args: ["-t", "stdio"]
      env_vars: [GITEA_TOKEN]   # 只声明转发哪些环境变量；禁止写明文密钥

knowledge:
  docs: [README.md, "docs/**/*.md"]
  codemap: false            # true 则每次 sync 都刷新 knowledge/CODEMAP.md
  index_history: false      # true 则索引把历史结论展开成清单，默认只给计数
  index_soft_limit: 200     # 现行对象超过它，review 报 index_pressure；0 关闭
```

`go.mod` 与 `package.json` 会被解析成依赖集合，能区分格式调整、版本升级和新增依赖。
其余清单只报「待判断信号」，不谎称能证明没有新增依赖。

## 在 CI 上用

```sh
keel check --target range --base "$BASE_SHA" --head "$HEAD_SHA" --json
```

本地 hook 可以被跳过，也不会随文件自动装到每个 clone。需要合并门禁就靠 CI。

## 谁会执行规则

| 入口 | 执行规则 |
|---|---|
| `keel check --target worktree` | ✔ 与工作树变更集求交 |
| `keel check --target index` / `commit-msg` | ✔ 在暂存区快照上，与暂存文件集求交 |
| `keel check --target range` | ✔ 与 base…head 文件集求交 |
| `keel review` / `keel why` / `keel brief` / SessionStart / **Stop** | ✘ |

两条原则：

1. **只有人显式发起的验证才执行仓库里的代码。** Stop 每轮自动触发，
   来自模板或自动提炼的规则不该因为「一轮结束」就被执行。
2. **只跑与本次变更相关的那些。** 规则的 `scope` 非空且变更集里没有匹配文件时跳过。
   改前端不该被 `internal/store/**` 的规则拦下。拿不到变更集（不是 git 仓库、工作树干净）时全跑，并在输出里说明。

## 知识索引

`keel sync` 生成 `.keel/knowledge/INDEX.md` 并进 git：决策、规则、记忆各一张表，
记忆那张还带验证时间和证据条数。clone 之后不装 keel 也能读。

索引**分两层**，这是它在对象上千之后仍然可读的原因：

- **现行**结论出全表。
- **历史**结论只出一张计数表：决策 `superseded` / `rejected`、规则 `retired`、记忆 `archived`。
  这张表最多四行，跟历史有多少条无关。

历史一条都没删，`keel why --history` 查得到。要在索引里看清单，设 `knowledge.index_history: true`。

`stale` 和 `disputed` 的记忆留在主表：它们是提醒（「这条过期了」「这条被反驳了」），
藏进历史区等于替人做了「不用管」的判断。`proposed` 的决策同理——它是待办，不是历史。

它是托管产物，手改会在下次 `sync` 报冲突而不是被默默覆盖。
内容只来自仓库里的对象，不含本机路径或生成时间——否则两台机器 sync 出来就不一样了。

## 边界

keel 内部**永远不调 LLM**，不管 key，不管网关。判断、写作、整理经验都交给宿主 agent。
它也不能授予宿主的沙箱、网络或发布权限——`workflow` 是建议上限，不是授权。
