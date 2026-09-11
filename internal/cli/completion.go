package cli

import (
	"os"
	"strings"
)

// cmdCompletion 装 shell 补全。
//
// 不带 shell 名时先认一下当前 shell，再把安装命令直接给出来——
// 「你得先知道自己在用哪个 shell、再知道文件该放哪」是这类功能最劝退的一步，
// 而这两件事 keel 都能替用户答上。
func cmdCompletion(e *env, args []string) error {
	fs := newFlagSet("completion")
	_ = commonFlags(fs, e)
	install := fs.Bool("install", false, "直接装到位，而不是把脚本打印出来")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("completion", rest, 1); err != nil {
		return err
	}
	want := ""
	if len(rest) == 1 {
		want = strings.ToLower(rest[0])
	}

	if *install {
		return installForShell(e, want)
	}
	if want != "" {
		script, ok := shellScript(want)
		if !ok {
			return usagef("不认识的 shell %q；可选：%s", want, strings.Join(shells, " "))
		}
		e.io.Printf("%s", script)
		return nil
	}

	sh := detectShell()
	if sh == "" {
		e.io.Println("认不出你在用哪个 shell（$SHELL 是空的）。挑一个：")
		for _, s := range shells {
			e.io.Println("    keel completion --install " + s)
		}
		return nil
	}
	e.io.Println("看起来你在用 " + sh + "。最省事的是让 keel 自己装：")
	e.io.Println()
	e.io.Println("    keel completion --install")
	e.io.Println()
	e.io.Println("想自己控制装到哪，就照抄下面这行：")
	e.io.Println()
	for _, line := range installHint(sh) {
		e.io.Println("    " + line)
	}
	e.io.Println()
	e.io.Println(completionNote)
	return nil
}

// completionNote 是装完之后最该知道的一句：不用再装第二次。
const completionNote = "装一次就够了：命令、选项、规则与记忆的 ID，都是每次按 Tab 现问 keel 要的。"

// installForShell 是 --install 的活：把补全脚本写到该在的地方，再往 rc 里补上加载它的那几行。
//
// install.sh / install.ps1 装完 keel 会顺手调它，所以认不出 shell 时不当成失败——
// 补全没装上不该让整个安装看起来是坏的。但用户自己点名了一个不认识的 shell，那是用法错误。
func installForShell(e *env, want string) error {
	sh := want
	if sh == "" {
		sh = detectShell()
	}
	if sh == "" {
		e.io.Errln("认不出你在用哪个 shell（$SHELL 是空的），补全先跳过。挑一个自己装：")
		for _, s := range shells {
			e.io.Errln("    keel completion --install " + s)
		}
		return nil
	}
	if _, ok := shellScript(sh); !ok {
		return usagef("不认识的 shell %q；可选：%s", sh, strings.Join(shells, " "))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	var psProfile string
	if sh == "powershell" || sh == "pwsh" {
		// $PROFILE 的真实路径只能问 PowerShell 本人要：OneDrive 会把「文档」整个重定向走。
		if psProfile = powerShellProfile(); psProfile == "" {
			e.io.Errln("问不出 PowerShell 的 $PROFILE 在哪，补全先跳过。手动装：keel completion powershell")
			return nil
		}
	}
	res, err := installCompletion(sh, home, psProfile)
	if err != nil {
		return err
	}
	switch {
	case res.ScriptWritten || res.RCWritten:
		e.io.Println("补全已装好（" + sh + "）：" + tildePath(res.ScriptPath, home))
		if res.RCWritten {
			e.io.Println("已在 " + tildePath(res.RCPath, home) + " 里加上加载它的几行；新开一个终端就生效。")
		}
	default:
		e.io.Println("补全已经是最新的（" + sh + "）。")
	}
	for _, n := range res.Notes {
		e.io.Errln(n)
	}
	if !e.quiet {
		e.io.Println(completionNote)
	}
	return nil
}

// tildePath 把家目录换成 ~，路径短一点好读。
func tildePath(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// detectShell 从 $SHELL 认 shell；认不出就返回空串，由调用方给出选项。
func detectShell() string {
	base := os.Getenv("SHELL")
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	for _, s := range shells {
		if base == s {
			return s
		}
	}
	if base == "pwsh" {
		return "powershell"
	}
	return ""
}

// cmdComplete 是给补全脚本调的暗门（不出现在帮助里）：一行一个候选，
// 格式是 值 \t 说明。显示不了说明的 shell 自己切掉后半段。
//
// 约定：正在敲的那个词走 --cur=（空串也是它，写成 --cur= 就丢不掉），
// 其余参数是已经敲完的词。complete() 要的是「最后一个是当前词」，在这儿拼好。
//
// 它不走 parseArgs：递进来的是用户敲了一半的东西，不是给 keel 的参数，
// 拿解析用法的那套去卡它只会在用户敲错时把补全整个弄哑。
func cmdComplete(e *env, args []string) error {
	const curFlag = "--cur="
	cur := ""
	var done []string
	for _, a := range args {
		if strings.HasPrefix(a, curFlag) {
			cur = strings.TrimPrefix(a, curFlag)
			continue
		}
		done = append(done, a)
	}
	// 用户自己写的 -C <dir> 要认：他问的是那个仓库里有哪些规则和记忆。
	dir := e.dir
	for len(done) > 0 {
		switch {
		case done[0] == "-C" && len(done) > 1:
			dir, done = done[1], done[2:]
			continue
		case strings.HasPrefix(done[0], "-C="):
			dir, done = strings.TrimPrefix(done[0], "-C="), done[1:]
			continue
		}
		break
	}
	for _, v := range complete(append(done, cur), dir) {
		e.io.Println(v.Name + "\t" + oneLine(v.Desc))
	}
	return nil
}

// oneLine 把说明压成一行：候选是按行读的，说明里混进换行会被当成另一个候选。
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}
