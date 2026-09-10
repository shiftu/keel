# keel 文件格式与命令规格

对应设计：[design.md](./design.md)。本文是实现时的对照表。审查建议见 [architecture-review.md](./architecture-review.md)；本文已按建议修订，冲突时以本文 + design.md 为准。

机器输出一律带 `schema` 版本。核心 frontmatter 字段严格校验；扩展放在 `extensions:` 命名空间。未知的核心字段名（例如策略拼写错误）报 error，不得静默保留后改变行为。

## 1. `.keel/` 目录

```
.keel/
├── intent.md
├── keel.yaml
├── generated.yaml              产物所有权、adapter schema 版本、生成摘要
├── decisions/D-<uuid>-<slug>.md
├── rules/R-<uuid>-<slug>.md
├── memory/M-<uuid>-<slug>.md
├── evidence/E-<uuid>.md
├── knowledge/INDEX.md          生成物，也进 git
├── knowledge/codemap.md        --codemap 时生成
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
- `why` 默认有效结论：`accepted | proven | revisit`。`--history` 含 `rejected | superseded | proposed`，并标注状态。
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
---

store 只认 SQLite。要换库先用 accepted 决策替代，并填写 rule_migration。
```

执行协议：

- 仅 `status: active` 且带 `check` 的规则在 `--target index|range` 时执行。`brief` / SessionStart / 检索不执行。
- cwd = 仓库根；超时默认 30s；杀掉进程组（Windows 无进程组信号语义，只杀规则进程本身）。结果：`pass | fail | error | timeout | skipped`。
- 目录不存在、工具不在 PATH → `error` 或 `skipped`（按「检查对象是否应存在」区分），**不得**变成 `pass`。
- `argv` 目标的退出码约定：0 = pass，1 = fail，其余 = error（与 `fixtures/shell-grep-negation/run.sh` 里的 `check()` 同构）。
- 禁止把 `! grep …` 当作推荐写法。若项目坚持 shell，executor 仍要区分 grep exit 1（无匹配）与 exit 2（用法/IO 错误）。夹具见 `fixtures/shell-grep-negation/`。
- `from` 决策 superseded/rejected：finding `rule_basis_invalid`（warn）。规则不自动 retired。
- 模板导入：保留 `from_template`；指向未映射项目决策的 `from` 不能使规则生效。

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

## 8. 命令规格

通用：`--json`；`-C <dir>`；`--quiet`。业务退出码 0 / 1 / 2。`keel hook` 的退出码见 §8.9，与业务 CLI 分离。

stdin/file 正文：`--body -` 读 stdin 至 EOF；`--body <path>` 读文件。与位置参数同时出现 → 退出码 2。

### 8.1 `keel init`

```
keel init [--tools claude,codex] [--from <git-url|dir>] [--no-hooks] [--yes]
```

1. 找 git 根，没有则退出码 2。
2. 已有 `.keel/` → 只补缺的文件，不覆盖。
3. `--from`：复制固定来源版本的模板 `.keel/`（skills、rules、keel.yaml 的 mcp 段、intent 骨架），不复制 decisions / memory / evidence。需要网络时仅这一步访问网络。
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

### 8.5 `keel why`

```
keel why [--path <path>] [--query <关键词>] [--history] [--json]
```

- `--path`：支持现有路径、尚未创建的路径、删除与重命名历史（结合 git）。不把「路径不存在」偷偷当成关键词。
- `--query`：title / tags / 正文子串。取代旧 `keel find`。
- 无参数 → 退出码 2。
- 默认有效结论；`--history` 含否决与替代，明确标记。
- 每条输出包含状态、路径、版本/ID、`why_selected`。

### 8.6 `keel check`

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
| Stop | worktree + 会话基线 | 纠正建议 + unresolved；不读猜测的消息草稿 |
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

去重：只抑制重复**反馈**（键见 §8.9），不得把 `fail` 改成 `pass`。Stop 的 unresolved 必须仍出现在随后的 index/range 检查中。

缓存：按代码目标 digest、规则集 digest、策略 digest、keel 版本绑定。缺失或不匹配显示 unknown，不能用旧绿色代替验证。

### 8.7 `keel brief`

```
keel brief [--task <text>] [--path <p>] [--changed] [--budget <bytes>] [--json]
```

SessionStart 无任务时只给基础材料（intent 入口、验证命令、查询方法、工作流上限）。agent 理解请求后带 `--task` / `--path` 再查。

选择顺序（确定性）：显式 ID/路径命中 → 适用条件 → 冲突与失败提醒 → 有效证据 → 主题匹配 → 时间；同分按稳定 ID。每条带 `why_selected`、状态、来源路径与版本。候选事实和外部材料作为引用，不提升成执行指令。

UTF-8 字节上限硬约束。超预算显示省略摘要和原文位置，不能静默丢掉。可额外显示 token **估算**，不宣称跨模型精确 token 数。

静态 CLAUDE.md / AGENTS.md **不**内嵌会过期的自主度表；动态状态只从 brief 来。

### 8.8 `keel review`

```
keel review [--json]
```

输出：到期决策及证据、学习候选（附依据）、工作流建议（**不是**自动新上限）、失效/冲突条目、统计。次数只排审阅优先级。证据来源：已提交 evidence、`git log` revert、硬规则结果。不把 cache-only 计数当跨 clone 事实。

### 8.9 `keel hook <adapter> <event>`

内部入口。读 stdin 事件，调共享内核，编码宿主输出。

- 正常决策路径：**exit 0 + 合法 JSON**（Claude 与 Codex 均如此编码）。
- 用法错误、内核崩溃：非 0，stderr 诊断；**不得**输出半截宿主 `decision` 对象。
- `check --json` 只返回 §8.6 的稳定结果，不返回 Claude/Codex 的 `decision`。
- Codex：按其文档，继续执行用 exit 0 JSON；需要阻断/继续的语义由 adapter 翻译。不得把业务退出码 2 泄漏成「Codex 继续」。
- Stop：不猜测 commit message。去重键 = repo + worktree + session + turn（若有）+ 信号摘要。第二次相同未解决问题：结束循环、保留 fail、不请求继续。

adapter 必须报告 hook 信任/就绪。未就绪时 hook 入口仍可 exit 0 并在 JSON 里声明 `adapter_status: not_ready`，同时 stderr 指向原生检查；显式 CLI 不受影响。

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

### 9.7 `.claude/rules/keel.md`

仅软规则 / 非执行摘要。Codex 合并进 AGENTS.md 块。

## 10. 内置技能要点

三个技能都短。步骤对齐修订后的生命周期。

**keel-decide**：① `keel why --path/--query`；② 是否够得上决策；③ 读 brief 里的工作流建议与路径/动作上限，**忽略**「三条 proven 就升级」；④ 级别 0 或无适用先例 → 输出选项/倾向/影响；否则 `keel decide … --body -`；⑤ trailer 用完整 ID。不得把自己写的 proven 当成独立验证。

**keel-learn**：① 查同类记忆；② 写 candidate，附 conditions；有验证则关联 evidence；③ 无值得保留的内容则结束。禁止靠重复转述凑条数。

**keel-review**：① `keel review --json`；② 生成有上限的 candidate，不直接改 active 硬规则；③ 对照验证（正/反/边界）由现行基线执行；④ 按 `workflow` 与项目审查生效或撤回；⑤ `keel check --target worktree` 与 `keel sync`。失效条目重定位或归档，不删除反例。

## 11. 工作流建议的计算

```
effective = min(workflow.default_level,
                命中的 workflow.actions[动作],
                命中的 workflow.paths[路径])
```

再与「适用范围内、无失效证据的先例」求交，得到是否允许记录而不先问人。无相关 evidence 的手写 proven **不**提高上限。多 tag 只影响检索，不参与 min()。省略 security tag 不能绕过 `workflow.actions.security` 或路径上限。

所有入口（brief、decide 技能、JSON）使用同一 `policy` 包结果。
