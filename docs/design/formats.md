# keel 文件格式与命令规格

对应设计：[design.md](./design.md)。本文是实现时的对照表。审查建议见 [architecture-review.md](./architecture-review.md)；本文已按建议修订，冲突时以本文 + design.md 为准。

机器输出一律带 `schema` 版本。核心 frontmatter 字段严格校验；扩展放在 `extensions:` 命名空间。未知的核心字段名（例如策略拼写错误）报 error，不得静默保留后改变行为。

## 1. `.keel/` 目录

```
.keel/
├── intent.md
├── keel.yaml
├── generated.yaml              产物所有权、adapter schema 版本、生成摘要
├── template.yaml               模板来源、钉住的 commit、导入基线（只在从模板导入过时存在）
├── decisions/D-<uuid>-<slug>.md
├── rules/R-<uuid>-<slug>.md
├── memory/M-<uuid>-<slug>.md
├── evidence/E-<uuid>.md
├── knowledge/INDEX.md          生成物，也进 git
├── knowledge/CODEMAP.md        生成物，--codemap 或 knowledge.codemap 时才有
├── cases/<rule-id>/<expect>/   规则的对照夹具
├── skills/<name>/SKILL.md
├── cache/                      gitignore：运行缓存、原始日志、接续摘要
└── .gitignore                  内容：cache/
```

### 1.1 ID

- canonical ID：`<Type>-<uuid>`，Type ∈ `D|R|M|E`。UUID 使用 UUID v4 文本（小写、带连字符）。
- **永不重编号。** 文件名是 `ID-slug.md`；改标题只改 slug，不改 ID。
- 短前缀：取 UUID 前 8 位十六进制，仅当当前仓库无歧义时接受为输入；有歧义则要求完整 ID。trailer 必须写完整 canonical ID。
- 展示排序：`date` 降序，同分按 canonical ID 升序。
- 写入：独占创建（`O_EXCL` 或等价）、同目录临时文件、原子替换。多文件状态转换使用 `.keel/cache/locks/` 与恢复记录；崩溃后下次命令先恢复或报需要人工介入的半成品，不得留下一半 superseded。

slug：标题转小写、非字母数字换 `-`、截到 40 字符；中文直接保留。

frontmatter：`---` 包围的 YAML。`schema: 1` 必填。

### 1.2 `intent.md`

Markdown。建议小节：目标、非目标、约束、完成标准、引用。初始化时可全是「待补充」清单加指向 README 的链接。keel 不填写猜测内容。

## 2. `keel.yaml`

```yaml
version: 1
tools: [claude, codex]

workflow:
  # 工作流建议上限；不是宿主沙箱/网络/发布授权
  default_level: 1            # 0 先问人 | 1 适用先例才自决 | 2 在上限内自决并记录
  auto_promote: false         # MVP 必须为 false：不按计数自动扩大上限
  actions:                    # 动作类别 → 上限；未列出的保持 default_level
    dependency.add: 1
    data.delete: 0
    public-api.change: 0
    security: 0
  paths:                      # glob → 上限；多项命中取更严
    "internal/store/**": 1
    "infra/**": 0

brief:
  max_bytes: 16384            # UTF-8 字节硬上限
  max_decisions: 8
  max_notes: 8

gate:
  require_trailer: false
  manifests:
    - go.mod
    - package.json
    - pyproject.toml
    - Cargo.toml
    - requirements*.txt
  watch_top_level_dirs: true

sync:
  commit_outputs: true
  skills_mode: copy           # copy | link

mcp:
  servers:
    gitea:
      command: gitea-mcp
      args: ["-t", "stdio"]
      env_vars: [GITEA_TOKEN] # 只声明转发哪些环境变量；禁止明文密钥

knowledge:
  docs: ["README.md", "docs/**/*.md"]
  exclude: ["docs/design/formats.md"]
```

`autonomy.proven_threshold` **不再存在**。旧稿按 tag 计数升级的语义删除。

## 3. 决策

```markdown
---
schema: 1
id: D-550e8400-e29b-41d4-a716-446655440000
title: 用 SQLite 而不是 Postgres
status: accepted
date: 2026-09-10
by: agent:claude
tags: [db, storage]
scope: ["internal/store/**", "go.mod"]
supersedes: []                 # 替代意图；proposed 时不改对方状态
superseded_by: []              # 仅在 accepted 转换时由内核写入
rule_migration: []             # 替代时必填：[{from: R-…, to: R-…|retired}]
confidence: 0.7
review_after: 2026-12-10
evidence: []
extensions: {}
---

## 背景
单机部署，数据量 < 1GB，无并发写。

## 决定
用 SQLite（WAL 模式），不引入 Postgres。

## 备选与理由
- Postgres：多一个进程要运维，收益为零。
- 纯文件 JSON：没有事务，多设备同步会撕裂。

## 后果与验证方式
- 派生规则：store 层不得出现 `pgx` / `lib/pq` import。
- 若单表超 1000 万行或需要多写者，回到 revisit。
```

状态机：

```
proposed ──▶ accepted ──▶ proven
   │            │  ▲         │
   ▼            ▼  │         ▼
rejected     revisit ◀───────┘

accepted | proven | revisit ──▶ superseded     仅当另一决策以 accepted 完成替代，且 rule_migration 完整
```

- `proposed` + `supersedes: [D-old]`：**只记录意图**。`D-old` 保持原状态。
- 新决策 `proposed → accepted`：同一事务写入新文件、旧文件 `superseded` + `superseded_by`、规则迁移。任一步失败则整笔回滚。
- `why` 默认列出非历史结论：`accepted | proven | revisit | proposed`。`--history` 追加 `rejected | superseded`，并标注状态。
  判据是 `DecisionStatus.IsHistory()`，与知识索引的分层同一个函数。`proposed` 是待办不是历史——
  一个正在等人拍板的方案比大多数已生效结论更该被看见。
- 空正文模板允许保存为 `proposed`；`accepted` 要求四段均非空。无关/空 ADR 不能作为门禁覆盖。

## 4. 规则

```markdown
---
schema: 1
id: R-7c9e6679-7425-40de-944b-e07fc1f90ae7
title: store 层不得依赖 Postgres 驱动
status: active                 # candidate | active | retired
scope: ["internal/store/**"]
severity: error                # error | warn
check:
  argv: ["scripts/check/no-postgres-driver.sh"]  # 优先 argv 调脚本或现成工具，不写 shell 取反
  timeout_seconds: 30
from: D-550e8400-e29b-41d4-a716-446655440000
from_template: null            # 模板导入时填来源；未映射的项目决策引用不能生效
verifier_digest: sha256:…
evidence: []                   # 对照验证留下的证据（keel promote 写）
cases:                         # 对照用例；promote 的前置条件
  - dir: .keel/cases/R-7c9e6679/pass
    expect: pass
  - dir: .keel/cases/R-7c9e6679/fail
    expect: fail
  - dir: .keel/cases/R-7c9e6679/edge
    expect: pass
---

store 只认 SQLite。要换库先用 accepted 决策替代，并填写 rule_migration。
```

执行协议：

- 只有 `status: active` 且带 `check` 的规则会被执行，且只在 `--target worktree|index|range` 这三个**人显式发起的验证**里。
  `brief` / `why` / `review` / SessionStart / **Stop** 一律不执行规则——一次「看看有什么要维护的」不该变成执行仓库里的代码。
- **按 scope 与本次变更集求交后再执行。** `scope` 非空且本次变更集里没有匹配文件 → `skipped`，不计入结果。
  `scope` 为空 = 全仓库规则，总是执行。拿不到变更集（不是 git 仓库、worktree 干净）时全部执行并在 Notes 里说明。
  这条约束对应验收「无关任务不被新规则误伤」：改前端不该被 `internal/store/**` 的规则拦下。
- cwd = 仓库根；超时默认 30s；杀掉进程组（Windows 无进程组信号语义，只杀规则进程本身）。结果：`pass | fail | error | timeout | skipped`。
- 目录不存在、工具不在 PATH → `error` 或 `skipped`（按「检查对象是否应存在」区分），**不得**变成 `pass`。
- `argv` 目标的退出码约定：0 = pass，1 = fail，其余 = error（与 `fixtures/shell-grep-negation/run.sh` 里的 `check()` 同构）。
- 禁止把 `! grep …` 当作推荐写法。若项目坚持 shell，executor 仍要区分 grep exit 1（无匹配）与 exit 2（用法/IO 错误）。夹具见 `fixtures/shell-grep-negation/`。
- `from` 决策 superseded/rejected：finding `rule_basis_invalid`（warn）。规则不自动 retired。
- 模板导入：保留 `from_template`；指向未映射项目决策的 `from` 不能使规则生效。

`cases` 是对照用例：每个 `dir` 是仓库里一个真目录，`keel promote` 把 `check.argv` 在它上面跑一遍，
比对 `expect`。夹具是仓库里可 review、可 diff、跨 clone 可重跑的普通文件，不是隐藏的测试框架。

执行方式：**cwd = 夹具目录**；`argv[0]` 是仓库内相对路径且在仓库根存在时按仓库根解析，其余参数原样传。
也就是「同一条规则，指着另一份仓库状态跑」。

`expect: fail` 的夹具里放的是**故意违规的代码**。全仓库扫描型的检查脚本必须把 `.keel/` 排除掉，
否则规则会被自己的反例夹具绊倒——正常验证时扫到 `.keel/cases/*/fail/` 就报违规。

## 5. 记忆

```markdown
---
schema: 1
id: M-0dbdf12b-5e8c-4a0a-9c1a-2f3e4d5c6b7a
kind: gotcha                   # gotcha | fact | pointer | counterexample
status: candidate              # candidate | verified | disputed | stale | archived
summary: SQLite WAL 在 NFS 上会锁失败
tags: [storage]
scope: ["internal/store/**"]
conditions:
  environment: 测试用临时目录必须在本地盘；NFS 不适用
evidence: []
derived_from: []
supersedes: []
verified_at: null
review_after: 2026-12-10
by: agent:claude
date: 2026-09-10
---
SQLite WAL 在 NFS 上会锁失败；测试用临时目录必须在本地盘。
```

- 正文建议短；超过约 20 行 `check` 提示拆分，不因此拒绝已存在文件。
- `verified` 要求 `evidence` 非空且至少一条相关 `pass`（或项目审查记录）。
- 同一 `derived_from` / 同一错误的重复转述：review 计为一条证据簇，不按条数升级。

状态转换只有两个入口，除此之外 keel 不动文件里的 `status`：

| 入口 | 允许的转换 |
|---|---|
| `keel verify` 真的跑了一次验证器 | candidate/disputed → verified（`pass`）；candidate/verified → disputed（`fail`） |
| 人或 agent 手工编辑文件 | 任意合法转换，但要能过下面的一致性检查 |

`check` 只**推导并报告**，不改文件。推导规则（全部基于已提交的 evidence，不看 cache）：

| finding | 条件 | severity |
|---|---|---|
| `memory_status_unsupported` | `verified` 但没有一条 `result: pass` 且 `subject_digest` 等于该记忆当前内容摘要的证据 | error |
| `memory_evidence_stale` | `verified` 且证据存在，但证据的 `target.content_digest` 不等于当前 scope 内容摘要 | warn |
| `memory_review_due` | `review_after` 早于今天，且状态是 candidate / verified | warn |
| `memory_conflict` | 某条 `counterexample` 的 `derived_from` 指向一条仍是 `verified` 的记忆 | warn |
| `object_stale` | scope 在工作树里一个都匹配不上（与决策、规则同一条规则） | warn |

规则那侧对应的一条：`rule_evidence_stale`（warn）——`active` 规则有过 promote 证据，
但它现在的定义摘要对不上了。它正在拦所有人，却没有对得上的验证。

`memory_status_unsupported` 是 error 而不是 warn：文件自称 verified、证据却支持不了它，等于把未验证的经验当成项目规则用。
改了记忆正文就会让旧证据的 `subject_digest` 对不上——这是有意的，结论变了就要重新验证。

`brief` 与 `why` 展示时必须区分「verified」和「verified（证据已过期）」；后者不作为现行结论呈现。

## 6. 证据

```yaml
schema: 1
id: E-11111111-2222-4333-8444-555555555555
subject: M-0dbdf12b-5e8c-4a0a-9c1a-2f3e4d5c6b7a
subject_digest: sha256:<digest>
kind: regression-test          # regression-test | check | review | self-reported
trust: local                   # local | ci | self-reported
target:
  commit: null                 # 仅当输入确为该提交内容
  content_digest: sha256:<tested-content-digest>
  dirty_inputs: []             # dirty 时列出纳入 digest 的路径
verifier:
  id: store-regression
  definition_digest: sha256:<digest>
  command: [go, test, ./internal/store/..., -run, TestRegression]
result: pass
observed_at: 2026-09-10T12:00:00Z
producer: keel-check
```

digest 输入集合排除 `evidence/` 自身与 `cache/`。密钥与不必要的个人信息不得写入摘要。

两个 digest 回答两个不同的问题，都不能省：

- `subject_digest`：**被验证的结论**在验证时的内容摘要。对不上 = 证据说的不是现在这条记忆。
  subject 可以是记忆、决策或规则；规则的证据由 `keel promote` 的对照验证产生。
- `target.content_digest`：**被验证的代码**在验证时的内容摘要（按 subject 的 `scope` 收集，路径排序后逐个摘要）。
  对不上 = 结论没变，但代码变了，验证结果过期。

`target.commit` 只在工作树干净时填 HEAD；脏工作树填 null 并把纳入摘要的路径列进 `dirty_inputs`。
不能拿一个脏工作树的结果冒充某个提交的验证。

**证据只能由 keel 真的跑完一次验证器产生**（`keel verify`）。没有「登记一条我认为它通过了」的入口：
那样的记录既不能被别人重跑，也不能被撤回，等于把自我确认写进仓库。

## 7. `generated.yaml`

```yaml
schema: 1
adapter_schema:
  claude: "2026-09-10"
  codex: "2026-09-10"
files:
  - path: CLAUDE.md
    owned: ["keel:begin..keel:end"]
    digest: sha256:…
  - path: .claude/skills/keel-decide/SKILL.md
    owned: ["*"]
    source: .keel/skills/keel-decide/SKILL.md
    digest: sha256:…
```

`sync`：先完整规划 diff 和冲突，再写入。同名非托管内容 → 冲突。上次生成后被人改过的托管内容 → 冲突。删除源对象仅清理仍与上次生成 digest 相同的产物。机器探测与渲染分离，避免同一源在不同机器得到无意差异。

### 7.1 `template.yaml`

只在从模板导入过的仓库里存在。

```yaml
schema: 1
source: https://github.com/org/keel-template.git
ref: main                       # 用户要的分支/标签，update 默认沿用
commit: 3f2a…                   # 解析出来的固定版本
files:
  - path: skills/team-style/SKILL.md
    digest: sha256:…            # 导入当时模板那一份的摘要 = 三方比较的基线
```

`path` 相对 `.keel/`。**基线摘要才是三方比较的真相，`commit` 只是出处**：
冲突文件的基线不推进，所以「两边都改过」会一直报到人处理为止，不会被下一次 `update` 悄悄抹掉。

`keel.yaml` 比的是配置的**含义**不是字节：两边都解码成配置结构、清掉 `tools`、再用同一个编码器写回。
`tools` 是本机探测结果，跨仓库不该一致；不清掉的话配置文件会永远停在「本地已改」，再也收不到模板的策略更新。

导入的规则被强制改写成候选形态：`status: candidate`、`evidence: []`、`verifier_digest: ""`、`from: null`、
`from_template: <source>@<commit>`。理由见 design.md 的 M4 切片——`promote` 不能自我批准，
装个模板不等于让别人的仓库决定这边执行什么代码。

## 8. 命令规格

通用：`--json`；`-C <dir>`；`--quiet`。业务退出码 0 / 1 / 2。`keel hook` 的退出码见 §8.12，与业务 CLI 分离。

stdin/file 正文：`--body -` 读 stdin 至 EOF；`--body <path>` 读文件。与位置参数同时出现 → 退出码 2。

列表选项（`--tag` / `--scope` / `--path` / `--condition` / `--rule-migration`）既接受重复给也接受逗号分隔，
两种写法等价且可混用。**重复给必须全部收下**：只留最后一个是静默丢弃，和拒绝未知选项是同一条原则。
`keel task` 的 `--done` / `--next` / `--failing` 只接受重复给、不拆逗号——那里每一项都是自然语言。

### 8.1 `keel init`

```
keel init [--tools claude,codex] [--from <git-url|dir>] [--no-hooks] [--yes]
```

1. 找 git 根，没有则退出码 2。
2. 已有 `.keel/` → 只补缺的文件，不覆盖。
3. `--from <url>[@<ref>]`：导入固定来源版本的模板，详见 §7.1。导入面是 `keel.yaml`、`skills/**`、`rules/*.md`、`cases/**`；**不导入** `decisions/` `memory/` `evidence/` `intent.md`。规则一律落成 `candidate`。这一步和 `keel template update` 是仅有的两个访问网络的入口。已经导入过模板的仓库再给 `--from` 报用法错误。
4. 探测工具二进制；找不到也允许 `--tools` 强制，并在能力报告里标 unknown/unsupported。
5. 写三个内置 skill。
6. 安装 git hooks：**禁止**在任意既有 hook 末尾无条件追加 shell。必须识别 `core.hooksPath`、worktree、现有 hook 管理器（Husky、lefthook、pre-commit 框架等）。支持的组合使用明确调用链；不支持的组合报告具体集成缺口，以非零退出码失败（可 `--no-hooks` 跳过）。hook 脚本见 §9.5 与 `fixtures/git-hook-exit/`。
7. 调 `sync`。
8. Codex：提示原生信任流程；文件写完 ≠ hook 已启用。

### 8.2 `keel sync`

```
keel sync [--codemap] [--dry-run] [--link]
```

纯规划 + 写入。`--dry-run` 只打印 diff/冲突。中途失败不留下半份产物（先写临时再替换，或记录恢复）。

`--codemap` 这一轮额外生成 `.keel/knowledge/CODEMAP.md`（§9.9）。它和别的产物一样归 `generated.yaml` 管：
只给 `--codemap` 的话，下一轮普通 `sync` 会按「源没了就清理」把它删掉。要长期留着，
在 `keel.yaml` 里写 `knowledge.codemap: true`。

### 8.3 `keel decide`

```
keel decide "<标题>" --tag <t>[,t2] --scope '<glob>'[,'<glob>'] \
            [--supersedes D-<uuid>] [--rule-migration R-a:R-b|R-a:retired] \
            [--confidence 0.7] [--by agent:claude] \
            [--status proposed|accepted] [--body - | --body <file>] [--edit]
```

- 无 `--body` 且无 `--edit` → 四段空标题模板，默认 `proposed`。
- `--status accepted` 拒绝空四段。
- `--supersedes` 在 `proposed` 只写字段；仅 `--status accepted` 触发原子迁移。
- 被替代的决策若有 `active` 派生规则，`--status accepted` 必须同时给 `--rule-migration`，否则退出码 2。
  迁移每项形如 `R-<from>:R-<to>` 或 `R-<from>:retired`。
- 原子迁移失败时删除刚建的新决策文件，旧决策保持原状（testscript 夹具 `supersede-accepted-migrates` 锁定）。
- 成功打印：文件路径 + `Decision: D-<uuid>`。

### 8.4 `keel note`

```
keel note "<正文>" --tag <t> [--path f.go[,g.go]] [--kind gotcha|fact|pointer|counterexample] \
          [--condition key=value[,k2=v2]] [--by agent:codex] [--body - | --body <file>]
```

默认 `status: candidate`。正文超过 20 行拒收（退出码 2），提示走 `decide` 或拆分。`--condition` 写入 `conditions` 适用条件。

### 8.5 `keel verify`

```
keel verify <subject-ref> --rule R-<uuid> [--kind check] [--trust local|ci]
keel verify <subject-ref> -- <argv…>      [--id <verifier-id>] [--kind …] [--trust …] [--timeout <秒>]
```

subject 是 `M-` 或 `D-` 的完整 ID 或短前缀。二选一给验证器，给了两个报用法错误：

- `--rule`：用某条规则的 `check.argv` 与它的 `timeout_seconds`；verifier id = 规则 ID，
  `definition_digest` = 规则 argv 与正文的摘要（规则改了，旧证据就认得出来）。
- `--` 之后的位置参数：显式 argv，原样 exec，不经 shell。verifier id 默认取 argv 摘要前 12 位。

流程：算 subject 摘要 → 收集 subject `scope` 覆盖的内容并算 `target.content_digest` →
在仓库根跑 argv（沿用 §4 的退出码约定与超时/进程组语义）→ 写 `evidence/E-<uuid>.md` →
按 §5 的状态转换表更新 subject。

- subject 是记忆：`pass` → verified 并写 `verified_at`；`fail` → disputed。两种情况都把证据 ID 追加进 `evidence`。
- subject 是决策：只追加证据 ID，**不改状态**。`proven` 是人的判断，不是跑通一条命令的自动结果。
- 验证器 `error` / `timeout`：照实写进证据，**不做任何状态转换**——跑不起来不是通过，也不是失败。
- 写证据与改 subject 是一笔：改 subject 失败就删掉刚写的证据文件。
- 退出码：验证器 pass → 0；fail → 1；error/timeout → 1。证据无论如何都写下来。

### 8.6 `keel task`

任务接续摘要。存在 `.keel/cache/task/current.yaml`，**在 gitignore 里**：
它是同一个 worktree 上 Claude 与 Codex 之间的交接，不是长期记忆。跨 clone 要留的经验用 `keel note`。

```
keel task set --goal "<目标>" [--done "<已完成>"]… [--next "<下一步>"]…
              [--failing "<失败的验证>"]… [--ref <ID>]… [--by agent:claude]
keel task show [--json]
keel task clear
```

- `--goal` / `--next` / `--failing` / `--ref`：给了就整体替换该字段，没给就保留原值。
- `--done`：追加（它是已完成事项的流水，不是当前状态）。
- `set` 记录写入时的 HEAD。`show` 与 `brief` 在 HEAD 变了之后照实说明「摘要记录于 <sha>」，不假装还准确。
- 摘要出现在 `brief` 最前面，并标明它来自本机缓存、跨 clone 不保证。

### 8.7 `keel promote` / `keel retire`

规则从候选到生效、以及生效之后撤回的唯一两个入口。

```
keel promote <R-…> [--json]
keel retire  <R-…> --reason "<原因>" [--json]
```

`promote` 的前置条件，任一不满足直接拒绝（退出码 1 或 2）：

| 条件 | 为什么 |
|---|---|
| 状态是 `candidate`，或是 `active` 但证据已对不上 | 后者是**重新验证**：改了定义之后再跑一次，状态不变 |
| 有 `check.argv` | 没有可执行检查的是软规则，靠人遵守，不走这条路 |
| `from` 指向一条**有效**决策 | 会拦住所有人提交的硬规则，必须有一条说明「为什么」的决策背书 |
| `cases` 里 `pass` 和 `fail` 两个方向都有 | 只证明「该过的过了」不算对照验证——一条永远返回 0 的检查也能满足 |
| 每条 case 的实际结果等于 `expect` | 验证器由现行基线提供，候选不能放宽自己的检查后宣告通过 |

跑完写一条证据（subject = 规则 ID，kind = `check`），正文逐条列出用例结果。
**失败也写证据**，`result: fail`，规则留在 `candidate`——失败历史留在仓库里，下次 review 看得到。

规则的 `definition_digest` 覆盖四样：`check.argv`、`cases`、正文，
以及 `argv[0]` 指向的仓库内脚本的内容。改检查脚本和改 argv 是同一件事——
现在生效的这条规则，和当初通过对照验证的那条不再是同一个东西。
对不上时 `check` 报 `rule_evidence_stale`（warn），重新 `promote` 一次即可。

`retire` 把 `active` / `candidate` 转 `retired`，在正文末尾追加一段带日期的撤回原因。
如果有决策的 `rule_migration` 曾把某条旧规则迁到这条，**提示**旧规则可以考虑恢复——只提示，不自动改。
不删除任何历史。

`archive <M-…> --reason` 是记忆一侧的对称命令：转 `archived`，在正文末尾追加一段带日期的归档原因。
有别的记忆 `derived_from` 指向它时**提示**转述可能也该处理——只提示，不自动改。
`archived` 是终态，再次 archive 报用法错误，不静默成功。

这是记忆状态的显式入口。`check` 与 `review` 都不改写 `status`：状态转换只有两个来源，
`keel verify` 真的跑完一次验证器，或者人显式改。时间不构成第三个来源。

### 8.8 `keel why`

```
keel why [--path <path>] [--query <关键词>] [--history] [--json]
```

- `--path`：支持现有路径、尚未创建的路径、删除与重命名历史（结合 git）。不把「路径不存在」偷偷当成关键词。
- `--query`：title / tags / 正文子串。取代旧 `keel find`。
- 无参数 → 退出码 2。
- 默认有效结论；`--history` 含否决与替代，明确标记。
- 每条输出包含状态、路径、版本/ID、`why_selected`。

### 8.9 `keel check`

```
keel check [--target worktree|index|commit-msg|range] \
           [--base <rev>] [--head <rev>] \
           [--commit-msg <file>] \
           [--json]
```

未指定 `--target` 时：交互式默认 `worktree`（只读诊断）；由 git hook 调用时必须显式 `index` 或 `commit-msg`。

| 调用场景 | target | 行为 |
|---|---|---|
| SessionStart / brief | （不跑 check） | 只读上下文 |
| Stop | 只做对象层检查 | 纠正建议 + unresolved；**不执行规则**（自动触发的入口不执行仓库代码）；不读猜测的消息草稿 |
| pre-commit | index | 验证将提交的文件及同一快照中的规则、决策 |
| commit-msg | commit-msg：`$1` + index | `git interpret-trailers --parse`；语义变更覆盖 |
| CI | range：`--base` + `--head` | 重算验证、决策覆盖、生成产物漂移 |

index 检查在临时快照中执行，不 stash/reset 用户工作树。不能可靠快照时：finding `index_snapshot_unavailable`，检查失败——不能用工作树结果证明 index 通过。

基础检查：

| 项 | 结果 |
|---|---|
| frontmatter 解析失败、ID 重复、核心字段非法 | error |
| `supersedes` / `from` / `evidence` 引用不存在 | error |
| active 硬规则失败 | error/warn 按 severity |
| 规则依据失效 | warn `rule_basis_invalid` |
| 两条 accepted/proven 决策 scope 重叠且解释冲突 | warn |
| scope 路径全部消失 | warn stale；不删除 |

语义信号（index/range）：

| 信号 | 条件 | 覆盖要求 |
|---|---|---|
| 新依赖 | 已解析 manifest 的依赖集合新增 | 决策完整、可用、覆盖相关路径、与该变化有解释关系 |
| 依赖版本变更 | 已解析到版本变化 | 按项目策略；默认提示，可配要求 |
| manifest 格式/无关脚本 | 非依赖集合变化 | 不报新依赖 |
| 未知 manifest | 配了路径但解析不了 | 待判断信号，不谎称已覆盖 |
| 顶层业务目录 | `watch_top_level_dirs`；排除 `.keel/` 与生成工具目录 | 同新依赖的覆盖规则 |

无关 ADR、空 `proposed`、错误 ID：不算覆盖。

JSON（业务 CLI，**不是**宿主 hook 外形）：

```json
{
  "schema": 1,
  "status": "fail",
  "target": "index",
  "findings": [
    {
      "code": "unexplained_dependency",
      "severity": "error",
      "path": "go.mod",
      "object": null,
      "fix": "keel decide \"…\" --scope go.mod 或引用已有决策完整 ID"
    }
  ]
}
```

去重：只抑制重复**反馈**（键见 §8.12），不得把 `fail` 改成 `pass`。Stop 的 unresolved 必须仍出现在随后的 index/range 检查中。

缓存：按代码目标 digest、规则集 digest、策略 digest、keel 版本绑定。缺失或不匹配显示 unknown，不能用旧绿色代替验证。

### 8.10 `keel brief`

```
keel brief [--task <text>] [--path <p>] [--changed] [--budget <bytes>] [--json]
```

SessionStart 无任务时只给基础材料（intent 入口、验证命令、查询方法、工作流上限）。agent 理解请求后带 `--task` / `--path` 再查。

选择顺序（确定性，同分按日期降序再按稳定 ID 升序）：

| 分档 | 命中条件 |
|---|---|
| 6 显式 ID | 任务文本里出现了这条的完整 ID 或短前缀 |
| 5 路径 | `--path` 命中它的 scope |
| 4 冲突与失败 | 决策 revisit；记忆 disputed 或 verified 但证据已过期；规则依据失效 |
| 3 有效证据 | 有 `result: pass` 且两个 digest 都对得上的证据 |
| 2 主题 | 标题词或 tag 出现在 `--task` 文本里 |
| 1 兜底 | 本仓库的有效对象 |

**「适用条件」不参与打分，而是逐条展示**：`conditions` 是自由文本，判断它适不适用是宿主 agent 的活，
不是 keel 的。keel 保证有条件的条目一定把条件原样带出来，让 agent 看得见。

每条带 `why_selected`、状态、来源路径。候选事实和外部材料作为引用，不提升成执行指令。
状态是 `verified` 但证据过期的记忆显示成 `verified（证据已过期）`，不作为现行结论。

工作流上限为 1（有适用先例才自决）时，brief 额外列出适用先例及其**是否有有效证据**。
没有带证据的先例就明说缺依据——这一条不会因为有几条手写的 `proven` 而改变。

第一段是任务接续摘要（§8.6），有才显示，并标明它来自本机缓存。

UTF-8 字节上限硬约束。超预算显示省略摘要和原文位置，不能静默丢掉。可额外显示 token **估算**，不宣称跨模型精确 token 数。

静态 CLAUDE.md / AGENTS.md **不**内嵌会过期的自主度表；动态状态只从 brief 来。

### 8.11 `keel review`

```
keel review [--json]
```

**只读，且不执行任何规则。** 一次「看看有什么要维护的」不该顺带执行仓库里的代码。
规则当前过不过，跑 `keel check --target worktree`。

四段输出：

| 段 | 内容 | 数据来源 |
|---|---|---|
| 到期 | `review_after` 已过的决策与记忆，各自带证据状态 | 已提交的对象与 evidence |
| 失效与冲突 | 知识层的 finding：`rule_basis_invalid`、`memory_status_unsupported`、`memory_evidence_stale`、`memory_conflict`、`decision_scope_overlap`、`object_stale` | `check` 的对象层检查（不含规则执行） |
| 学习候选 | 见下表 | 已提交对象 + `git log` |
| 统计 | 各类对象条数、有证据的比例 | 已提交对象 |

学习候选是**确定性聚类**，不是自动写出来的修订。keel 给簇和依据，判断它们是不是同一条不变量是宿主 agent 的活：

| 候选 | 条件 |
|---|---|
| `rule_candidate_ready` | `candidate` 规则，`cases` 齐全 → 可以 `keel promote` |
| `rule_candidate_incomplete` | `candidate` 规则，缺 `check.argv` / `from` / 某个方向的 case → 先补齐 |
| `memory_cluster` | 同 scope 同 tag 的 ≥2 条 candidate 记忆 → 可能是同一条不变量 |
| `memory_disputed` | 有 `counterexample` 指向它 → 先处理冲突，再谈提炼 |
| `decision_reverted` | `git log` 里 `Revert` 提交引用过的决策 → 结论可能不成立 |

**次数只排审阅优先级，不决定可信或生效。** 同一 `derived_from` 的重复转述在 `memory_cluster` 里计为一条，
不按条数升级。不把 `cache/` 里的计数当跨 clone 事实——`review` 只读已提交的内容。

### 8.12 `keel hook <adapter> <event>`

内部入口。读 stdin 事件，调共享内核，编码宿主输出。

- 正常决策路径：**exit 0 + 合法 JSON**（Claude 与 Codex 均如此编码）。
- 用法错误、内核崩溃：非 0，stderr 诊断；**不得**输出半截宿主 `decision` 对象。
- `check --json` 只返回 §8.9 的稳定结果，不返回 Claude/Codex 的 `decision`。
- Codex：按其文档，继续执行用 exit 0 JSON；需要阻断/继续的语义由 adapter 翻译。不得把业务退出码 2 泄漏成「Codex 继续」。
- Stop：不猜测 commit message。去重键 = repo + worktree + session + turn（若有）+ 信号摘要。第二次相同未解决问题：结束循环、保留 fail、不请求继续。

adapter 必须报告 hook 信任/就绪。未就绪时 hook 入口仍可 exit 0 并在 JSON 里声明 `adapter_status: not_ready`，同时 stderr 指向原生检查；显式 CLI 不受影响。

### 8.13 `keel template`

```
keel template status [--json]
keel template update [--to <ref>] [--dry-run] [--json]
```

`status` **不联网**：它回答的是「本地相对导入基线动过什么」，不是「模板那边有没有新版」。后者归 `update`。

`update` 取模板新版做三方比较，每个文件的处置只由 base / theirs / ours 三者决定：

| ours vs base | theirs vs base | 动作 | 写盘 |
|---|---|---|---|
| 同 | 同 | `unchanged` | 否 |
| 同 | 变 | `fast-forward` | 是 |
| 变 | 同 | `keep-local` | 否 |
| 变 | 变 | `conflict` | **否** |
| 本地已删 | 任意 | `local-deleted` | 否 |
| 任意 | 模板已删 | `template-deleted` | 否 |
| 本地没有 | 新增 | `add` | 是 |

冲突不阻塞其他文件：能快进的照常快进。有冲突时退出码 1，其余情况 0。
`--dry-run` 一个字节都不写。`fast-forward` 写 `keel.yaml` 时保留本地 `tools`。

`local-deleted` 与 `template-deleted` 每次都会如实报告且退出码仍是 0：
两个方向 keel 都不替人做主——不把删掉的写回去，也不替人删本地的。

## 9. 产物

### 9.1 `CLAUDE.md` / `AGENTS.md` 标记块

只放稳定协议：项目目标入口、验证命令、知识查询方法、工作流上限。不放会过期的 proven 计数表。

```markdown
<!-- keel:begin -->
## keel 协议（由 keel sync 生成，勿手改；改 .keel/）

本仓库用 keel 保存项目目标、约定、任务经验和验证证据。

1. 目标与约束见 `.keel/intent.md`。
2. 改相关路径前：`keel why --path <path>`；任务上下文：`keel brief --task "…" --path <path>`。
3. 加/换依赖、跨模块结构、公开接口或数据格式、构建/部署方式：记录决策或引用已有完整 ID。空 ADR 不算覆盖。
4. 工作流建议上限见 `keel.yaml` 的 `workflow`；tag 只用于检索。扩大上限走配置审查，不按记录条数自动升级。
5. 提交用完整 trailer：`Decision: D-<uuid>`。
6. 有价值的经验用 `keel note`（candidate）；不要把未验证记忆写成命令。

查询：`keel why` / `keel brief` / `keel check --target index`
<!-- keel:end -->
```

块外原样保留。Codex AGENTS.md 有层级覆盖与体积限制；适配不保证相同文件文本产生完全相同的有效上下文。

### 9.2 `.claude/settings.json`

```json
{
  "hooks": {
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "keel hook claude session-start" }] }],
    "Stop":         [{ "hooks": [{ "type": "command", "command": "keel hook claude stop" }] }]
  }
}
```

所有权以 `generated.yaml` 为准，不以 `command` 前缀猜测用户手写调用。

### 9.3 `.codex/hooks.json`

调用 `keel hook codex session-start` / `keel hook codex stop`。具体 schema 以探测到的 Codex 版本为准。

### 9.4 MCP

Claude `.mcp.json` 可使用 `${GITEA_TOKEN}` 展开（官方支持）。

Codex `.codex/config.toml`：

```toml
[mcp_servers.gitea]
command = "gitea-mcp"
args = ["-t", "stdio"]
env_vars = ["GITEA_TOKEN"]
```

禁止写成 `env = { GITEA_TOKEN = "${GITEA_TOKEN}" }` 并假设与 Claude 一致。需要变量重命名时显式报告是否支持。

### 9.5 git hooks

正确传播失败（夹具 `fixtures/git-hook-exit/` 必须证明旧 `|| true` 会吞失败）：

```sh
# keel:begin
if command -v keel >/dev/null 2>&1; then
  keel check --quiet --target index || exit $?
fi
# keel:end
```

```sh
# keel:begin
if command -v keel >/dev/null 2>&1; then
  keel check --quiet --target commit-msg --commit-msg "$1" || exit $?
fi
# keel:end
```

验收（init 安装）：检查失败、未安装 keel、已有 hook 失败、非 shell hook、自定义 `hooksPath`、worktree 六种情况都有明确结果。没装 keel 放行是有意的：本地龙骨不是每个 clone 的门卫；合并门禁走 CI。

### 9.6 技能分发

源 `.keel/skills/<name>/` → 工具目录。`generated.yaml` 记录 digest。用户改过 → 冲突。源删除且目标仍等于上次生成 → 删除；否则保留并报冲突。`.keel-managed` 空文件不再作为唯一所有权证明。

### 9.7 `.keel/knowledge/INDEX.md`

`sync` 生成，整文件托管（`owned: '*'`），**进 git**。它是 clone 之后不装 keel 也能读的入口。

内容只来自仓库里的对象，不含本机路径、时间戳或探测结果——否则两台机器 sync 出来会不一样。

**分两层。** 主表只列现行对象：决策、规则、记忆各一张表，记忆那张带验证时间与证据条数。
历史对象退出主表，只进一张计数表：

| 类别 | 历史状态 |
|---|---|
| 决策 | `superseded`、`rejected` |
| 规则 | `retired` |
| 记忆 | `archived` |

判据是 `DecisionStatus.IsHistory()` / `RuleStatus.IsHistory()` / `MemoryStatus.IsHistory()`，
`why --history` 用的是同一组函数——两边口径必须一致。

`proposed` 的决策、`stale` 与 `disputed` 的记忆**不算历史**，留在主表：前者是待办，
后两者是提醒，把它们藏进历史区等于替人做了「不用管」的判断。

计数表最多四行，与历史对象条数无关——这是索引不随对象数无限变长的地方。
`knowledge.index_history: true` 追加逐条清单（ID、状态、标题，决策再带 `superseded_by` 去向）。

### 9.8 `.claude/rules/keel.md`

仅软规则 / 非执行摘要。Codex 合并进 AGENTS.md 块。

### 9.9 `.keel/knowledge/CODEMAP.md`

`sync --codemap` 或 `knowledge.codemap: true` 时生成，整文件托管，进 git。

只列目录：每个目录一行，文件数加按条数降序的文件类型。**没有符号、没有调用关系**——
§10 明确不做 AST 和代码图谱。

输入是 `git ls-files`，不是工作树遍历：没登记的文件不进概览，
两台机器上生成的必须逐字节一致，否则每次 sync 都是一个假 diff（和 INDEX.md 同一个约束）。

## 10. 内置技能要点

三个技能都短。步骤对齐修订后的生命周期。

**keel-decide**：① `keel why --path/--query`；② 是否够得上决策；③ 读 brief 里的工作流建议与路径/动作上限，**忽略**「三条 proven 就升级」；④ 级别 0 或无适用先例 → 输出选项/倾向/影响；否则 `keel decide … --body -`；⑤ trailer 用完整 ID。不得把自己写的 proven 当成独立验证。

**keel-learn**：① 查同类记忆；② 写 candidate，附 conditions；有验证则关联 evidence；③ 无值得保留的内容则结束。禁止靠重复转述凑条数。

**keel-review**：① `keel review --json`；② 生成有上限的 candidate 规则，**不直接写 `status: active`**；
③ 给它 `from`（依据决策）和 `cases`（至少 pass / fail 两个方向的真目录）；④ `keel promote R-…` 跑对照验证，
过不了就留在 candidate，失败证据也留在仓库里；⑤ 发现反例或回归用 `keel retire R-… --reason "…"`；
⑥ `keel check --target worktree` 与 `keel sync` 收尾。失效条目重定位或归档，不删除反例。

## 11. 工作流建议的计算

```
effective = min(workflow.default_level,
                命中的 workflow.actions[动作],
                命中的 workflow.paths[路径])
```

再与「适用范围内、无失效证据的先例」求交，得到是否允许记录而不先问人。无相关 evidence 的手写 proven **不**提高上限。多 tag 只影响检索，不参与 min()。省略 security tag 不能绕过 `workflow.actions.security` 或路径上限。

所有入口（brief、decide 技能、JSON）使用同一 `policy` 包结果。
