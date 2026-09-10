package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/adapter"
	"github.com/shiftu/keel/internal/brief"
	"github.com/shiftu/keel/internal/check"
	"github.com/shiftu/keel/internal/store"
)

// hookEvent 是宿主发到 stdin 的事件。Claude 与 Codex 的公共字段形状一致。
type hookEvent struct {
	SessionID      string `json:"session_id"`
	HookEventName  string `json:"hook_event_name"`
	Cwd            string `json:"cwd"`
	TurnID         string `json:"turn_id"`
	StopHookActive bool   `json:"stop_hook_active"`
}

// cmdHook 是内部入口：读 stdin 事件，调共享内核，编码成宿主输出。
//
// 正常决策路径一律 exit 0 + 合法 JSON。业务 CLI 的 0/1/2 不会泄漏到这里——
// 把「检查失败」当成「宿主该继续」是两套语义的混用。
func cmdHook(e *env, args []string) error {
	fs := newFlagSet("hook")
	_ = commonFlags(fs, e)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := exactlyArgs("hook", rest, 2); err != nil {
		return usagef("用法：keel hook <claude|codex> <session-start|stop>")
	}
	adapterName, event := rest[0], rest[1]
	if _, ok := adapter.ByName(adapterName); !ok {
		return usagef("未知适配器 %q（支持 claude、codex）", adapterName)
	}

	var ev hookEvent
	if data, err := io.ReadAll(e.stdin); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &ev) // 事件解析失败不影响我们能做的事
	}
	if ev.Cwd != "" {
		e.dir = ev.Cwd
	}

	switch event {
	case "session-start":
		return hookSessionStart(e, adapterName)
	case "stop":
		return hookStop(e, adapterName, ev)
	}
	return usagef("未知事件 %q（支持 session-start、stop）", event)
}

// hookSessionStart 只读：给稳定协议入口和基础材料，不跑项目检查。
func hookSessionStart(e *env, adapterName string) error {
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "SessionStart",
			"additionalContext": "",
		},
	}
	b, cfg, err := buildBrief(e, brief.Query{})
	if err != nil {
		// 仓库没初始化不是错误，安静退出即可。
		return emitHookJSON(e, out, adapterStatus(e, adapterName))
	}
	inner := out["hookSpecificOutput"].(map[string]any)
	inner["additionalContext"] = b.Text(cfg.Brief.MaxBytes)
	return emitHookJSON(e, out, adapterStatus(e, adapterName))
}

// hookStop 提供有界纠正：第一次出现可修正的问题时请求一次继续；
// 已在继续循环里、或同一批问题已经反馈过，就结束循环并保留失败状态。
//
// 去重只抑制重复反馈，不会把 fail 改成 pass，也不会让提交检查豁免。
func hookStop(e *env, adapterName string, ev hookEvent) error {
	st, err := e.discover()
	if err != nil {
		return emitHookJSON(e, map[string]any{}, adapterStatus(e, adapterName))
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		return emitHookJSON(e, map[string]any{}, adapterStatus(e, adapterName))
	}
	set, err := st.Load()
	if err != nil {
		return emitHookJSON(e, map[string]any{}, adapterStatus(e, adapterName))
	}

	res := check.NewResult(check.TargetWorktree)
	check.Objects(st, cfg, set, res)
	check.Rules(st, set, res)
	res.Sort()

	var problems []string
	for _, f := range res.Findings {
		if f.Severity != check.SeverityError {
			continue
		}
		problems = append(problems, fmt.Sprintf("[%s] %s", f.Code, f.Message))
	}
	if len(problems) == 0 {
		return emitHookJSON(e, map[string]any{}, adapterStatus(e, adapterName))
	}

	sig := signalDigest(problems)
	seen, err := stopSeen(st, ev, sig)
	if err != nil {
		return err
	}
	if ev.StopHookActive || seen {
		e.io.Errf("keel：这些问题仍未解决，本次不再重复要求继续。提交时 keel check --target index 依然会拦：\n  %s\n",
			strings.Join(problems, "\n  "))
		return emitHookJSON(e, map[string]any{}, adapterStatus(e, adapterName))
	}
	if err := markStopSeen(st, ev, sig); err != nil {
		return err
	}
	reason := "keel 在工作树里发现这些问题，处理完再收尾：\n  " + strings.Join(problems, "\n  ") +
		"\n（这不是成功证据；提交前仍要跑 keel check --target index）"
	return emitHookJSON(e, map[string]any{"decision": "block", "reason": reason},
		adapterStatus(e, adapterName))
}

// adapterStatus 报告 hook 就绪情况。未就绪也 exit 0，只在 JSON 与 stderr 里说明，
// 显式 CLI 不受影响。
func adapterStatus(e *env, name string) string {
	st, err := e.discover()
	if err != nil {
		return "not_ready"
	}
	a, ok := adapter.ByName(name)
	if !ok {
		return "unknown"
	}
	for _, c := range a.Capabilities(st.Root) {
		if c.Capability != adapter.CapStopContinue {
			continue
		}
		switch c.Support {
		case adapter.Supported:
			return "ready"
		case adapter.Unsupported:
			return "not_ready"
		}
		return "unknown"
	}
	return "unknown"
}

func emitHookJSON(e *env, payload map[string]any, status string) error {
	payload["adapter_status"] = status
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e.io.Println(string(data))
	return nil
}

func signalDigest(problems []string) string {
	sorted := append([]string{}, problems...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:8])
}

// stopKey 用 仓库 + 工作树 + 会话 + turn + 信号摘要 组键。
// 只用 session id 会把后续任务永久放行。
func stopKey(st *store.Store, ev hookEvent, sig string) string {
	parts := []string{st.Root, ev.SessionID, ev.TurnID, sig}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func stopSeen(st *store.Store, ev hookEvent, sig string) (bool, error) {
	p := filepath.Join(st.CacheDir(), "stop", stopKey(st, ev, sig))
	_, err := os.Stat(p)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func markStopSeen(st *store.Store, ev hookEvent, sig string) error {
	dir := filepath.Join(st.CacheDir(), "stop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, stopKey(st, ev, sig)), nil, 0o644)
}
