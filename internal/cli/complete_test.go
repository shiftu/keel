package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shiftu/keel/internal/ui"
)

func names(vs []valueSpec) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Name)
	}
	return out
}

func has(vs []valueSpec, want string) bool {
	for _, v := range vs {
		if v.Name == want {
			return true
		}
	}
	return false
}

func TestCompleteCommands(t *testing.T) {
	got := complete([]string{""}, ".")
	if !has(got, "decide") || !has(got, "why") {
		t.Errorf("空前缀该给出全部命令，实际 %v", names(got))
	}
	for _, v := range got {
		if strings.HasPrefix(v.Name, "__") || v.Name == "hook" {
			t.Errorf("内部命令 %q 不该出现在候选里", v.Name)
		}
		if v.Desc == "" {
			t.Errorf("命令 %q 没有说明", v.Name)
		}
	}
}

func TestCompletePrefixFilters(t *testing.T) {
	got := names(complete([]string{"ver"}, "."))
	if len(got) != 2 || got[0] != "verify" || got[1] != "version" {
		t.Errorf("ver 该补出 verify / version，实际 %v", got)
	}
}

func TestCompleteUnknownCommandGivesNothing(t *testing.T) {
	if got := complete([]string{"nosuchcmd", ""}, "."); len(got) != 0 {
		t.Errorf("不认识的命令后面不该有候选，实际 %v", names(got))
	}
}

func TestCompleteFlags(t *testing.T) {
	got := complete([]string{"why", "--"}, ".")
	for _, want := range []string{"--path", "--query", "--history", "--json", "--quiet"} {
		if !has(got, want) {
			t.Errorf("keel why -- 该补出 %s，实际 %v", want, names(got))
		}
	}
	// -C 是单横线，补 -- 的时候不该冒出来。
	if has(got, "--C") {
		t.Errorf("-C 不该写成 --C：%v", names(got))
	}
}

func TestCompleteFlagValues(t *testing.T) {
	got := names(complete([]string{"check", "--target", ""}, "."))
	want := []string{"worktree", "index", "commit-msg", "range"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("--target 候选 = %v，想要 %v", got, want)
	}
	if got := names(complete([]string{"check", "--target", "co"}, ".")); len(got) != 1 || got[0] != "commit-msg" {
		t.Errorf("--target co 该补出 commit-msg，实际 %v", got)
	}
}

// bash 按 COMP_WORDBREAKS 断词，--kind=g 会被拆成三个词。补出来的必须是整个词。
func TestCompleteEqualsFormFromBash(t *testing.T) {
	got := names(complete([]string{"note", "--kind", "=", "g"}, "."))
	if len(got) != 1 || got[0] != "--kind=gotcha" {
		t.Errorf("--kind=g 该补出 --kind=gotcha，实际 %v", got)
	}
}

// 逗号列表只补最后一段，但要带上前面敲好的部分，否则 shell 会把它吃掉。
func TestCompleteCommaList(t *testing.T) {
	got := names(complete([]string{"init", "--tools", "claude,co"}, "."))
	if len(got) != 1 || got[0] != "claude,codex" {
		t.Errorf("--tools claude,co 该补出 claude,codex，实际 %v", got)
	}
	// 已经写过的不再重复给。
	if got := names(complete([]string{"init", "--tools", "claude,codex,"}, ".")); len(got) != 0 {
		t.Errorf("两个都写完了就不该再有候选，实际 %v", got)
	}
}

func TestCompleteSubcommands(t *testing.T) {
	got := names(complete([]string{"task", ""}, "."))
	want := []string{"set", "show", "clear"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("task 子命令 = %v，想要 %v", got, want)
	}
	if !has(complete([]string{"task", "set", "--"}, "."), "--goal") {
		t.Error("task set -- 该补出 --goal")
	}
	if got := complete([]string{"task", "nosuch", ""}, "."); len(got) != 0 {
		t.Errorf("不认识的子命令后面不该有候选，实际 %v", names(got))
	}
}

// `--` 之后是交给别人跑的命令，不是 keel 的词。
func TestCompleteStopsAtDoubleDash(t *testing.T) {
	if got := complete([]string{"verify", "M-1234abcd", "--", "go", ""}, "."); len(got) != 0 {
		t.Errorf("-- 之后不该有候选，实际 %v", names(got))
	}
}

// 只收一个位置参数的命令，已经给过就别再提示。
func TestCompleteSinglePositional(t *testing.T) {
	dir := repoWithObjects(t)
	if got := complete([]string{"promote", ""}, dir); len(got) == 0 {
		t.Error("promote 该补出规则 ID")
	}
	if got := complete([]string{"promote", "R-11111111", ""}, dir); len(got) != 0 {
		t.Errorf("promote 已经给过 ID 了，不该再补，实际 %v", names(got))
	}
}

func TestCompleteObjectIDs(t *testing.T) {
	dir := repoWithObjects(t)

	rules := complete([]string{"promote", ""}, dir)
	if len(rules) != 1 || rules[0].Name != "R-11111111" || rules[0].Desc != "两个-横线的-slug" {
		t.Errorf("规则候选 = %+v", rules)
	}
	if got := names(complete([]string{"archive", ""}, dir)); len(got) != 1 || got[0] != "M-22222222" {
		t.Errorf("archive 该只补记忆，实际 %v", got)
	}
	// verify 认记忆也认决策。
	got := names(complete([]string{"verify", ""}, dir))
	if len(got) != 2 || got[0] != "D-33333333" || got[1] != "M-22222222" {
		t.Errorf("verify 候选 = %v，想要 D-… 和 M-…（按字典序）", got)
	}
	// 敲得比短前缀长，说明在写完整 ID，那就给完整的。
	full := names(complete([]string{"promote", "R-11111111-"}, dir))
	if len(full) != 1 || full[0] != "R-11111111-1111-4111-8111-111111111111" {
		t.Errorf("长前缀该补完整 ID，实际 %v", full)
	}
}

func TestCompleteObjectIDsOutsideRepo(t *testing.T) {
	if got := complete([]string{"promote", ""}, t.TempDir()); got != nil {
		t.Errorf("不在 keel 仓库里就没有 ID 可补，实际 %v", names(got))
	}
}

func TestCompletePaths(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "alpha.md"), "x")
	mustWrite(t, filepath.Join(dir, ".hidden"), "x")
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := names(complete([]string{"why", "--path", ""}, dir))
	if len(got) != 2 || got[0] != "alpha.md" || got[1] != "sub/" {
		t.Errorf("路径候选 = %v，想要 alpha.md 和 sub/（隐藏文件不出现）", got)
	}
	if got := names(complete([]string{"why", "--path", "."}, dir)); len(got) != 1 || got[0] != ".hidden" {
		t.Errorf("明确敲了点号才给隐藏文件，实际 %v", got)
	}
	if got := names(complete([]string{"why", "--path", "sub/"}, dir)); len(got) != 1 || got[0] != "sub/deep/" {
		t.Errorf("子目录候选 = %v", got)
	}
}

// 用户自己写的 -C 要认：他问的是那个仓库里有什么。
func TestCompleteHonorsChdirFlag(t *testing.T) {
	dir := repoWithObjects(t)
	c, io := newCapture()
	if err := cmdComplete(&env{io: io, dir: t.TempDir()}, []string{"--cur=", "-C", dir, "promote"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.out.String(), "R-11111111") {
		t.Errorf("-C 指过去的仓库里的规则该被补出来，实际 %q", c.out.String())
	}
}

// 说明里混进换行会被当成另一个候选。
func TestCompleteOutputIsOneLinePerCandidate(t *testing.T) {
	c, io := newCapture()
	if err := cmdComplete(&env{io: io, dir: "."}, []string{"--cur="}); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimRight(c.out.String(), "\n"), "\n") {
		if strings.Count(line, "\t") != 1 {
			t.Errorf("每行该正好一个制表符：%q", line)
		}
	}
}

// 测试里要看命令往 stdout 写了什么，就得自己拿个缓冲区当标准流。
type capture struct {
	out strings.Builder
	err strings.Builder
}

func newCapture() (*capture, ui.IO) {
	c := &capture{}
	return c, ui.IO{Out: &c.out, Err: &c.err}
}

func repoWithObjects(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"rules/R-11111111-1111-4111-8111-111111111111-两个-横线的-slug.md": "",
		"memory/M-22222222-2222-4222-8222-222222222222-记忆.md":         "",
		"decisions/D-33333333-3333-4333-8333-333333333333-决策.md":      "",
		"rules/README.md":   "", // 不是对象文件，不该被当成候选
		"evidence/.keep":    "",
		"knowledge/keep.md": "",
	}
	for rel, body := range files {
		mustWrite(t, filepath.Join(dir, ".keel", rel), body)
	}
	return dir
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
