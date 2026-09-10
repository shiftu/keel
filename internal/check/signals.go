package check

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/gitx"
	"github.com/shiftu/keel/internal/manifest"
	"github.com/shiftu/keel/internal/store"
)

// ChangeSet 描述本次要验证的那批变化，以及去哪里取两侧内容。
// worktree / index / range 取的是不同快照，绝不互相替代。
type ChangeSet struct {
	Repo      *gitx.Repo
	Files     []string
	BeforeRev string // HEAD 或 base；空表示没有上一版（首次提交）
	AfterRev  string // ":" 表示暂存区
}

// Coverage 是本次变化的解释来源：提交信息里的 trailer，
// 以及本次一起改动的决策文件。
type Coverage struct {
	Trailers []store.ID
	// ChangedDecisionPaths 是本次变化里新增或修改的决策文件（仓库相对路径）。
	ChangedDecisionPaths map[string]bool
}

// usableDecision 报告一条决策能否用来解释一次语义变化。
// 空正文的 proposed、已被替代或否决的都不算。
func usableDecision(d *store.Decision) (bool, string) {
	if !d.Status.IsActive() {
		return false, fmt.Sprintf("状态是 %s，不是有效结论", d.Status)
	}
	if missing := d.EmptySections(); len(missing) > 0 {
		return false, "正文缺少 " + strings.Join(missing, "、")
	}
	return true, ""
}

type covIndex struct {
	set   *store.Set
	cands []*store.Decision
	notes []string
}

func newCovIndex(set *store.Set, cov Coverage) *covIndex {
	ci := &covIndex{set: set}
	seen := map[store.ID]bool{}
	add := func(d *store.Decision, source string) {
		if seen[d.ID] {
			return
		}
		ok, why := usableDecision(d)
		if !ok {
			ci.notes = append(ci.notes, fmt.Sprintf("%s（%s）不能作为覆盖：%s", d.ID.Short(), source, why))
			return
		}
		seen[d.ID] = true
		ci.cands = append(ci.cands, d)
	}
	for _, id := range cov.Trailers {
		if d, ok := set.DecisionByID(id); ok {
			add(d, "trailer")
		}
	}
	for _, d := range set.Decisions {
		if cov.ChangedDecisionPaths[d.SourcePath()] {
			add(d, "本次一同改动")
		}
	}
	return ci
}

// covering 找一条 scope 覆盖该路径的可用决策。
// 无关 ID 和空文件不算覆盖——这正是「新增依赖 + 空 ADR」必须失败的原因。
func (ci *covIndex) covering(p string) (*store.Decision, bool) {
	for _, d := range ci.cands {
		if _, ok := store.MatchAny(d.Scope, p); ok {
			return d, true
		}
	}
	return nil, false
}

// Signals 检查语义变化是否有解释。只在 index / commit-msg / range 上跑：
// 工作树不是将要提交的内容，不能用它证明提交通过。
func Signals(st *store.Store, cfg store.Config, set *store.Set, res *Result, cs ChangeSet, cov Coverage) {
	ci := newCovIndex(set, cov)
	for _, n := range ci.notes {
		res.Notes = append(res.Notes, n)
	}
	dependencySignals(cfg, res, cs, ci)
	topLevelSignals(cfg, res, cs, ci)
}

func dependencySignals(cfg store.Config, res *Result, cs ChangeSet, ci *covIndex) {
	for _, f := range cs.Files {
		if !matchesManifest(cfg.Gate.Manifests, f) {
			continue
		}
		after, err := cs.Repo.FileAt(cs.AfterRev, f)
		if err != nil {
			res.AddError(Finding{Code: CodeManifestUnparsed, Path: f, Message: err.Error()})
			continue
		}
		if after == nil {
			continue // 清单被删除：不是新增依赖
		}
		var before []byte
		if cs.BeforeRev != "" {
			before, err = cs.Repo.FileAt(cs.BeforeRev, f)
			if err != nil {
				res.AddError(Finding{Code: CodeManifestUnparsed, Path: f, Message: err.Error()})
				continue
			}
		}
		if !manifest.Supported(f) {
			res.Add(Finding{
				Code: CodeManifestUnparsed, Severity: SeverityWarn, Path: f,
				Message: "配置为清单但 keel 还不会解析它，无法判断是否新增依赖",
				Fix:     "人工确认这次改动有没有引入新依赖；需要机械判断就先支持该格式",
			})
			continue
		}
		beforeSet, _ := manifest.Parse(f, before)
		afterSet, ok := manifest.Parse(f, after)
		if !ok {
			res.Add(Finding{
				Code: CodeManifestUnparsed, Severity: SeverityWarn, Path: f,
				Message: "清单内容解析失败，不能证明没有新增依赖",
				Fix:     "修好清单语法后重跑",
			})
			continue
		}
		diff := manifest.Compare(beforeSet, afterSet)
		if diff.Empty() {
			continue // 纯格式调整
		}
		for _, dep := range diff.DirectAdded() {
			if d, ok := ci.covering(f); ok {
				res.Notes = append(res.Notes,
					fmt.Sprintf("新增依赖 %s 由 %s 解释", dep.Name, d.ID.Short()))
				continue
			}
			res.Add(Finding{
				Code: CodeUnexplainedDependency, Path: f,
				Message: fmt.Sprintf("新增直接依赖 %s %s，本次没有可用的决策解释它", dep.Name, dep.Version),
				Fix: fmt.Sprintf("keel decide \"为什么引入 %s\" --tag dependency --scope %s --status accepted --body -"+
					"，或在提交信息里用完整 ID 引用已有决策", dep.Name, f),
			})
		}
		for _, dep := range diff.Changed {
			res.Add(Finding{
				Code: CodeDependencyVersion, Severity: SeverityWarn, Path: f,
				Message: fmt.Sprintf("依赖 %s 版本变为 %s", dep.Name, dep.Version),
				Fix:     "版本升级默认只提示；需要强制解释就在项目策略里调整",
			})
		}
	}
}

func matchesManifest(patterns []string, f string) bool {
	base := path.Base(f)
	for _, pat := range patterns {
		if store.MatchGlob(pat, f) || store.MatchGlob(pat, base) {
			return true
		}
	}
	return false
}

// topLevelSignals 报告新出现的顶层业务目录。
// keel 自己的目录和生成的工具目录不算——init 本身不该触发「新架构目录」。
func topLevelSignals(cfg store.Config, res *Result, cs ChangeSet, ci *covIndex) {
	if !cfg.Gate.WatchTopLevelDirs {
		return
	}
	after, err := cs.Repo.TopLevelDirs(cs.AfterRev)
	if err != nil {
		return
	}
	before := map[string]bool{}
	if cs.BeforeRev != "" {
		if b, err := cs.Repo.TopLevelDirs(cs.BeforeRev); err == nil {
			before = b
		}
	}
	var added []string
	for d := range after {
		if before[d] || isScaffoldDir(d) {
			continue
		}
		added = append(added, d)
	}
	sort.Strings(added)
	for _, d := range added {
		if dec, ok := ci.covering(d + "/"); ok {
			res.Notes = append(res.Notes, fmt.Sprintf("新顶层目录 %s/ 由 %s 解释", d, dec.ID.Short()))
			continue
		}
		res.Add(Finding{
			Code: CodeUnexplainedTopLevel, Path: d + "/",
			Message: fmt.Sprintf("新增顶层目录 %s/，本次没有可用的决策解释它", d),
			Fix: fmt.Sprintf("keel decide \"%s/ 承担什么\" --tag structure --scope '%s/**' --status accepted --body -"+
				"，或引用已有决策", d, d),
		})
	}
}

// isScaffoldDir 列出不算「新架构目录」的脚手架产物。
func isScaffoldDir(d string) bool {
	switch d {
	case store.DirName, ".claude", ".codex", ".agents", ".github", ".git":
		return true
	}
	return strings.HasPrefix(d, ".")
}

// TrailerDecisions 从提交信息里解析 Decision: 引用并校验可用性。
func TrailerDecisions(repo *gitx.Repo, set *store.Set, message string, res *Result) []store.ID {
	trailers, err := repo.Trailers(message)
	if err != nil {
		res.AddError(Finding{Code: CodeTrailerUnknownRef, Message: err.Error()})
		return nil
	}
	var ids []store.ID
	for _, raw := range trailers["Decision"] {
		id, err := store.ParseID(strings.TrimSpace(raw))
		if err != nil {
			res.Add(Finding{
				Code: CodeTrailerUnknownRef, Object: raw,
				Message: "提交信息里的 Decision: 不是完整 canonical ID：" + err.Error(),
				Fix:     "trailer 必须写完整 ID，短前缀只能在命令行输入时用",
			})
			continue
		}
		d, ok := set.DecisionByID(id)
		if !ok {
			res.Add(Finding{
				Code: CodeTrailerUnknownRef, Object: id.String(),
				Message: "提交信息引用的决策不存在",
				Fix:     "确认 ID，或先创建决策",
			})
			continue
		}
		if ok, why := usableDecision(d); !ok {
			res.Add(Finding{
				Code: CodeTrailerNotUsable, Object: id.String(), Path: d.SourcePath(),
				Message: "引用的决策不能作为覆盖：" + why,
				Fix:     "补齐正文并转 accepted，或引用别的决策",
			})
			continue
		}
		ids = append(ids, id)
	}
	return ids
}
