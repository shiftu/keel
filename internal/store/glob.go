package store

import "strings"

// MatchGlob 用 / 分段匹配路径。支持 ** （任意多段，含零段）、* 和 ? （段内）。
// 约定：`dir/**` 同时匹配 `dir` 本身和 dir 下的一切，这样 scope 写目录即可覆盖该目录。
func MatchGlob(pattern, name string) bool {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	name = strings.TrimPrefix(strings.TrimSpace(name), "./")
	pattern = strings.TrimSuffix(pattern, "/")
	name = strings.TrimSuffix(name, "/")
	if pattern == "" || name == "" {
		return pattern == name
	}
	return matchSegs(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

// MatchAny 返回第一个命中的 pattern；没有命中返回空串和 false。
func MatchAny(patterns []string, name string) (string, bool) {
	for _, p := range patterns {
		if MatchGlob(p, name) {
			return p, true
		}
	}
	return "", false
}

func matchSegs(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchSegs(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if !matchSeg(pat[0], name[0]) {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

func matchSeg(pat, s string) bool {
	pi, si := 0, 0
	star, match := -1, 0
	for si < len(s) {
		switch {
		case pi < len(pat) && (pat[pi] == '?' || pat[pi] == s[si]):
			pi++
			si++
		case pi < len(pat) && pat[pi] == '*':
			star = pi
			match = si
			pi++
		case star >= 0:
			pi = star + 1
			match++
			si = match
		default:
			return false
		}
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}
