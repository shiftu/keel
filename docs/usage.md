# keel 用法

对应设计：[design.md](design/design.md)。字段与命令规格：[formats.md](design/formats.md)。

当前实现到 M1：`init` / `sync` / `decide` / `why` / `note` / `check` / `brief` / `hook` / `version` 可用；
`review` 属于 M3，会明确报「尚未实现」。

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
改代码前     keel why --path internal/store/db.go
接任务时     keel brief --task "给存储层加缓存" --path internal/store/
做了决策     keel decide "…" --status accepted --body -
踩了坑       keel note "…" --tag storage
提交前       keel check --target index      （pre-commit 会自动跑）
```

## 命令

### `keel init`

```
keel init [--tools claude,codex] [--no-hooks] [--adopt-hooks]
```

不给 `--tools` 就探测 PATH。已有 `.keel/` 时只补缺的文件，不覆盖。

仓库里已经有别人的 `pre-commit` 时，`init` 会**失败并说明怎么接**，不会静默跳过也不会覆盖：

- 是 POSIX shell 脚本 → 用 `--adopt-hooks`，原脚本移到 `pre-commit.keel-local` 并由新 hook 先调用，它失败就整体失败。
- 是 husky / lefthook / pre-commit 框架 → 在它们的配置里加一步 `keel check --target index`。
- 是非 shell 脚本 → keel 不碰，自己加调用。
- 确认不需要本地 hook → `--no-hooks`，合并门禁走 CI。

`core.hooksPath` 和 git worktree 都会被正确识别。

### `keel sync`

```
keel sync [--dry-run]
```

`.keel/` → 工具原生文件。先完整规划再写入：**有任何冲突就一个文件都不写**。

keel 只拥有自己写的那部分：标记块之间的内容、`generated.yaml` 里记过的整文件、
JSON 里它写过的那几条 hook 条目和 MCP 键。块外的内容、你自己加的 hook、
`.claude/settings.json` 里的其他字段，一律原样保留。

托管内容被手工改过时报冲突而不是覆盖。想让改动生效就搬回 `.keel/`。

### `keel decide`

```
keel decide "<标题>" --tag <t> --scope '<glob>' [--status accepted] [--body -]
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
keel note "<一句话>" --tag <t> [--path f.go] [--kind gotcha|fact|pointer|counterexample]
          [--condition environment=...] [--by agent:claude]
```

新记忆一律 `candidate`：可检索，但在有证据之前不会被当成项目规则注入。
正文超过 20 行会被拒绝——那说明它其实是一次决策。

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
```

`go.mod` 与 `package.json` 会被解析成依赖集合，能区分格式调整、版本升级和新增依赖。
其余清单只报「待判断信号」，不谎称能证明没有新增依赖。

## 在 CI 上用

```sh
keel check --target range --base "$BASE_SHA" --head "$HEAD_SHA" --json
```

本地 hook 可以被跳过，也不会随文件自动装到每个 clone。需要合并门禁就靠 CI。

## 边界

keel 内部**永远不调 LLM**，不管 key，不管网关。判断、写作、整理经验都交给宿主 agent。
它也不能授予宿主的沙箱、网络或发布权限——`workflow` 是建议上限，不是授权。
