package store

// MemoryTrust 是一条记忆现在实际站得住多少。check 和 brief 必须用同一份判断，
// 否则会出现「提交时报未验证、brief 里却当现行结论」这种自相矛盾。
type MemoryTrust struct {
	// Supported 表示有一条 pass 证据，且它的 subject_digest 等于记忆当前内容摘要。
	Supported bool
	// ContentStale 表示证据验证过的那份代码之后变过了。
	ContentStale bool
	// Evidence 是支持它的那条证据，没有则为 nil。
	Evidence *Evidence
}

// Claimed 报告记忆自称已验证。
func (t MemoryTrust) Claimed(m *Memory) bool { return m.Status == MemVerified }

// Trustworthy 报告它现在可以作为现行结论呈现。
func (t MemoryTrust) Trustworthy() bool { return t.Supported && !t.ContentStale }

// TrustOf 推导一条记忆的可信度。dg 可以为 nil，此时不做代码内容比对。
//
// 一条结论可能被验证过多次。只要其中**任意一次**验的就是当前这份代码，它就不算过期；
// 都对不上时取最新那次来报告，这样提示里的证据 ID 是有意义的那一条。
func TrustOf(set *Set, m *Memory, dg *Digester) MemoryTrust {
	all := set.SupportingEvidences(m)
	if len(all) == 0 {
		return MemoryTrust{}
	}
	t := MemoryTrust{Supported: true, Evidence: all[0]}
	if dg == nil {
		return t
	}
	now, err := dg.Digest(m.Scope)
	if err != nil || now == "" {
		return t
	}
	for _, e := range all {
		if e.Target.ContentDigest == "" || e.Target.ContentDigest == now {
			t.Evidence = e
			return t
		}
	}
	t.ContentStale = true
	return t
}

// MemoryScopes 收集全部记忆的 scope，供 NewDigester 一次遍历。
func MemoryScopes(set *Set) [][]string {
	var out [][]string
	for _, m := range set.Memories {
		if len(m.Scope) > 0 {
			out = append(out, m.Scope)
		}
	}
	return out
}

// RuleEvidenceCurrent 报告规则现在这个样子有没有对得上的 promote 证据：
// 结果是 pass、subject_digest 对得上、且验证器定义摘要也对得上。
//
// check 和 promote 必须用同一份判断，否则会出现「check 说证据过期、
// promote 说不用重跑」这种自相矛盾。
func RuleEvidenceCurrent(root string, set *Set, r *Rule) bool {
	subject := ObjectDigest(r)
	definition := RuleDefinitionDigest(root, r)
	for _, e := range set.EvidenceFor(r.ID) {
		if e.Result == "pass" && e.SubjectDigest == subject &&
			e.Verifier.DefinitionDigest == definition {
			return true
		}
	}
	return false
}
