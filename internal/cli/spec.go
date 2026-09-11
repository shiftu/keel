package cli

// 命令的「说明书」：每个命令有哪些选项、选项能填什么。
//
// 它只服务补全。帮助文本仍然在各命令自己手里——那是给人读的散文，
// 不该被塞进表结构。这里要的是机器能筛能比的形状。
//
// 两处写着同一件事就会有一处先坏掉，所以 spec_test.go 拿 go/ast 读真正注册
// 选项的那几行，逐个命令比对：少写了、多写了、改了名，测试当场报出来。

// valueSpec 是一个候选取值：名字 + 一行说明。
type valueSpec struct {
	Name string
	Desc string
}

// vals 把一串名字包成没有说明的候选。
func vals(names ...string) []valueSpec {
	out := make([]valueSpec, 0, len(names))
	for _, n := range names {
		out = append(out, valueSpec{Name: n})
	}
	return out
}

// dyn 是「候选得现算」的来源。写死在 shell 脚本里的候选会随着仓库内容过期，
// 而且过期了不报错——补全不会失败，只会补错，用户还不知道。
type dyn string

const (
	dynNone    dyn = ""
	dynPath    dyn = "path"     // 文件系统路径
	dynRule    dyn = "rule"     // R-…
	dynMemory  dyn = "memory"   // M-…
	dynDecide  dyn = "decision" // D-…
	dynObject  dyn = "object"   // M-… / D-…
	dynShell   dyn = "shell"    // bash / zsh / fish / powershell
	dynNothing dyn = "nothing"  // 自由文本：明确表示「不补」，别退化成补命令名
)

// flagSpec 是一个命令行选项。
type flagSpec struct {
	Name   string // 不带前缀，如 tag
	Arg    string // 取值的占位名，如 ID；布尔选项留空
	Desc   string
	Values []valueSpec // 固定候选
	Dyn    dyn         // 动态候选
	List   bool        // 取值是逗号分隔的多项
}

// cmdSpec 是一个命令。Subs 非空时它只是个分发入口（task / template）。
type cmdSpec struct {
	Name    string
	Summary string
	Subs    []cmdSpec
	Flags   []flagSpec
	Arg     dyn  // 位置参数的候选来源
	Repeat  bool // 能给多个位置参数
	Hidden  bool // 不出现在候选里（内部入口）
}

// 通用选项：每个命令都接受，由 commonFlags 注册。
func commonSpec() []flagSpec {
	return []flagSpec{
		{Name: "C", Arg: "DIR", Desc: "仓库目录", Dyn: dynPath},
		{Name: "quiet", Desc: "只输出必要内容"},
		{Name: "json", Desc: "输出机器可读结果"},
	}
}

// byValues 是 --by 的常见写法。不穷举，只是省得每次手敲 agent: 前缀。
func byValues() []valueSpec {
	return []valueSpec{
		{"human", "人写的"},
		{"agent:claude", "Claude Code 写的"},
		{"agent:codex", "Codex 写的"},
	}
}

// specs 是全部命令。顺序跟 commands() 一致。
func specs() []cmdSpec {
	return []cmdSpec{
		{Name: "init", Summary: "建 .keel/，探测工具，安装 git hooks，然后 sync", Flags: []flagSpec{
			{Name: "tools", Arg: "LIST", Desc: "要适配的工具，逗号分隔（默认探测 PATH）",
				Values: vals("claude", "codex"), List: true},
			{Name: "from", Arg: "URL", Desc: "从模板仓库导入：<git-url>[@<ref>]"},
			{Name: "no-hooks", Desc: "不安装 git hooks"},
			{Name: "adopt-hooks", Desc: "已有 shell hook 时串联而不是放弃"},
		}},
		{Name: "sync", Summary: ".keel/ → 工具原生文件；先规划再写入", Flags: []flagSpec{
			{Name: "dry-run", Desc: "只打印计划与冲突，不写入"},
			{Name: "codemap", Desc: "这一轮同时生成目录概览"},
		}},
		{Name: "decide", Summary: "记录一次架构决策", Arg: dynNothing, Flags: []flagSpec{
			{Name: "tag", Arg: "LIST", Desc: "标签，可重复或逗号分隔", List: true},
			{Name: "scope", Arg: "GLOB", Desc: "管辖路径 glob，可重复或逗号分隔", Dyn: dynPath, List: true},
			{Name: "supersedes", Arg: "ID", Desc: "替代的决策 ID", Dyn: dynDecide},
			{Name: "rule-migration", Arg: "MAP", Desc: "规则迁移，形如 R-xxx:R-yyy 或 R-xxx:retired", List: true},
			{Name: "confidence", Arg: "N", Desc: "作者自评 0-1（不授予可信状态）"},
			{Name: "by", Arg: "WHO", Desc: "作者声明，如 agent:claude", Values: byValues()},
			{Name: "status", Arg: "S", Desc: "proposed 或 accepted", Values: vals("proposed", "accepted")},
			{Name: "body", Arg: "SRC", Desc: "正文来源：- 表示标准输入，否则为文件路径", Dyn: dynPath},
			{Name: "edit", Desc: "打开 $EDITOR 写正文"},
		}},
		{Name: "why", Summary: "查某路径或主题下的决策、规则、记忆", Flags: []flagSpec{
			{Name: "path", Arg: "PATH", Desc: "路径（可以是还不存在的新文件）", Dyn: dynPath},
			{Name: "query", Arg: "TEXT", Desc: "关键词：标题、标签、正文子串"},
			{Name: "history", Desc: "同时列出被否决、被替代的结论"},
		}},
		{Name: "note", Summary: "记一条候选记忆", Arg: dynNothing, Flags: []flagSpec{
			{Name: "tag", Arg: "LIST", Desc: "标签，可重复或逗号分隔", List: true},
			{Name: "path", Arg: "PATH", Desc: "相关路径，可重复或逗号分隔", Dyn: dynPath, List: true},
			{Name: "kind", Arg: "K", Desc: "记忆类型", Values: []valueSpec{
				{"gotcha", "踩过的坑"}, {"fact", "事实"},
				{"pointer", "指路"}, {"counterexample", "反例"},
			}},
			{Name: "by", Arg: "WHO", Desc: "作者声明，如 agent:codex", Values: byValues()},
			{Name: "body", Arg: "SRC", Desc: "正文来源：- 表示标准输入，否则为文件路径", Dyn: dynPath},
			{Name: "condition", Arg: "COND", Desc: "适用条件，形如 environment=仅本地盘", List: true},
		}},
		{Name: "check", Summary: "按 target 验证对象、规则与语义变化", Flags: []flagSpec{
			{Name: "target", Arg: "T", Desc: "检查对象", Values: []valueSpec{
				{"worktree", "工作区，随时诊断"}, {"index", "暂存区，提交前"},
				{"commit-msg", "提交信息"}, {"range", "一段提交"},
			}},
			{Name: "commit-msg", Arg: "FILE", Desc: "提交信息文件（--target commit-msg 时必填）", Dyn: dynPath},
			{Name: "base", Arg: "REV", Desc: "--target range 的起点"},
			{Name: "head", Arg: "REV", Desc: "--target range 的终点"},
		}},
		{Name: "verify", Summary: "跑一次验证器，把结果记成证据", Arg: dynObject, Flags: []flagSpec{
			{Name: "rule", Arg: "ID", Desc: "用这条规则的 check.argv 当验证器", Dyn: dynRule},
			{Name: "id", Arg: "NAME", Desc: "验证器标识，默认取命令摘要"},
			{Name: "kind", Arg: "K", Desc: "验证器类型", Values: []valueSpec{
				{"regression-test", "回归测试"}, {"check", "检查"},
				{"review", "评审"}, {"self-reported", "自称（不产生可信证据）"},
			}},
			{Name: "trust", Arg: "T", Desc: "证据可信级别", Values: vals("local", "ci")},
			{Name: "timeout", Arg: "SEC", Desc: "超时秒数，默认 30"},
		}},
		{Name: "promote", Summary: "规则对照验证：candidate → active", Arg: dynRule},
		{Name: "retire", Summary: "撤回一条规则，保留原因与历史", Arg: dynRule, Flags: []flagSpec{
			{Name: "reason", Arg: "TEXT", Desc: "为什么撤回；会写进规则正文"},
		}},
		{Name: "archive", Summary: "归档一条记忆，保留原因与历史", Arg: dynMemory, Flags: []flagSpec{
			{Name: "reason", Arg: "TEXT", Desc: "为什么归档；会写进记忆正文"},
		}},
		{Name: "task", Summary: "任务接续摘要：set / show / clear", Subs: []cmdSpec{
			{Name: "set", Summary: "写一条接续摘要", Flags: []flagSpec{
				{Name: "goal", Arg: "TEXT", Desc: "这次任务的目标"},
				{Name: "by", Arg: "WHO", Desc: "谁写的，如 agent:claude", Values: byValues()},
				{Name: "done", Arg: "TEXT", Desc: "已完成事项（可重复；追加）"},
				{Name: "next", Arg: "TEXT", Desc: "下一步（可重复；整体替换）"},
				{Name: "failing", Arg: "TEXT", Desc: "还在失败的验证（可重复；整体替换）"},
				{Name: "ref", Arg: "ID", Desc: "相关对象 ID（可重复；整体替换）", Dyn: dynObject},
			}},
			{Name: "show", Summary: "看当前接续摘要"},
			{Name: "clear", Summary: "清掉接续摘要"},
		}},
		{Name: "template", Summary: "模板来源：status / update", Subs: []cmdSpec{
			{Name: "status", Summary: "模板来源与本地改动"},
			{Name: "update", Summary: "拉一次模板更新", Flags: []flagSpec{
				{Name: "to", Arg: "REF", Desc: "改用另一个 ref（默认沿用导入时记的那个）"},
				{Name: "dry-run", Desc: "只报告，不写入"},
			}},
		}},
		{Name: "brief", Summary: "输出任务相关的上下文包", Flags: []flagSpec{
			{Name: "task", Arg: "TEXT", Desc: "这次要做什么"},
			{Name: "path", Arg: "PATH", Desc: "相关路径，可重复或逗号分隔", Dyn: dynPath, List: true},
			{Name: "action", Arg: "A", Desc: "动作类别", Values: vals(
				"dependency.add", "structure.change", "public-api.change",
				"data.delete", "security", "build.change")},
			{Name: "budget", Arg: "N", Desc: "正文 UTF-8 字节上限"},
		}},
		{Name: "review", Summary: "进化报告：到期、候选、工作流建议"},
		{Name: "completion", Summary: "装 shell 补全（不带参数会认一下当前 shell）", Arg: dynShell, Flags: []flagSpec{
			{Name: "install", Desc: "直接装到位，而不是把脚本打印出来"},
		}},
		{Name: "update", Summary: "把 keel 自己换成 GitHub 上的新版", Flags: []flagSpec{
			{Name: "check", Desc: "只看有没有新版，不下载"},
			{Name: "force", Desc: "版本一样也重装"},
			{Name: "version", Arg: "TAG", Desc: "指定版本，如 v0.5.0"},
		}},
		{Name: "hook", Summary: "内部：宿主 hook 事件编解码", Hidden: true},
		{Name: "__complete", Summary: "内部：给补全脚本算候选", Hidden: true},
		{Name: "version", Summary: "打印版本"},
	}
}

// lookupSpec 按名字找命令。
func lookupSpec(list []cmdSpec, name string) (cmdSpec, bool) {
	for _, c := range list {
		if c.Name == name {
			return c, true
		}
	}
	return cmdSpec{}, false
}
