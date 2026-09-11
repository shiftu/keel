package cli

import "strings"

// 四家 shell 的胶水脚本。
//
// 它们都只做同一件事：把「正在敲的词 + 已经敲完的词」递给 keel __complete，
// 拿回一行一个候选（值 \t 说明），再按各家的规矩喂回去。候选是什么、怎么筛，
// 全在 complete() 里——脚本一次装好，之后 keel 升级、加命令、仓库里多了几条规则
// 都不用重装。
//
// 正在敲的那个词走 --cur= 而不是当最后一个参数，是因为它经常是空串：
// fish 的空命令替换会整个消失，Windows PowerShell 5.1 传空串给外部程序也会丢。
// 一旦丢了，keel 就会把上一个词当成正在敲的那个，补出来的东西全是错的——
// 而且不报错。写成 --cur= 就永远是个非空参数，谁也丢不掉。

// shells 是支持的 shell，按 `keel completion` 的提示顺序。
var shells = []string{"bash", "zsh", "fish", "powershell"}

func shellValues() []valueSpec {
	return []valueSpec{
		{"bash", "输出 bash 补全脚本"},
		{"zsh", "输出 zsh 补全脚本"},
		{"fish", "输出 fish 补全脚本"},
		{"powershell", "输出 PowerShell 补全脚本"},
	}
}

const bashScript = `# keel 补全（bash）。候选由 keel 本身现算，装一次即可。
_keel_complete() {
    local out line
    COMPREPLY=()
    # 这里刻意不用 <(...) 进程替换：macOS 自带的 bash 3.2 在函数里配上改过的 IFS
    # 会让它悄悄读不到东西（不报错，就是没候选）。先收进变量再喂 here-string，哪都能跑。
    out=$(keel __complete "--cur=${COMP_WORDS[COMP_CWORD]}" "${COMP_WORDS[@]:1:COMP_CWORD-1}" 2>/dev/null)
    [ -n "$out" ] || return 0
    while IFS= read -r line; do
        [ -n "$line" ] || continue
        COMPREPLY+=("${line%%$'\t'*}")   # bash 显示不了说明，切掉
    done <<< "$out"
}
complete -o default -F _keel_complete keel
`

const zshScript = `#compdef keel
# keel 补全（zsh）。候选由 keel 本身现算，装一次即可。
_keel() {
    local -a lines
    # \t 换成 : 是 _describe 的格式；只换第一个，说明里的冒号（https://…）才不会被当分隔符。
    lines=(${(f)"$(keel __complete "--cur=${words[CURRENT]}" "${(@)words[2,CURRENT-1]}" 2>/dev/null)"})
    lines=(${lines[@]/$'\t'/:})
    _describe -t keel 'keel' lines
}

# 这个文件有两种用法，得都照顾到：
#   放进 fpath（推荐）—— compinit 会把整个文件当成 _keel 的函数体自动加载，
#     这时函数刚定义好还没跑，得当场调一次，否则第一次按 Tab 什么都不出。
#   直接 source —— 那就只登记一下，交给补全系统。
if [ "${funcstack[1]}" = "_keel" ]; then
    _keel "$@"
else
    compdef _keel keel
fi
`

const fishScript = `# keel 补全（fish）。候选由 keel 本身现算，装一次即可。
function __keel_complete
    set -l tokens (commandline -opc)
    set -l args
    # 只有一个词（就是 keel 本身）时不能切片 —— 老版本 fish 会报下标越界。
    if test (count $tokens) -gt 1
        set args $tokens[2..-1]
    end
    set -l cur (commandline -ct)
    # fish 认 值\t说明 这个格式，直接透传。
    keel __complete "--cur=$cur" $args 2>/dev/null
end
# -f：不要在候选里混进文件名，路径由 keel 自己补。
complete -c keel -f -a '(__keel_complete)'
`

const powershellScript = `# keel 补全（PowerShell）。候选由 keel 本身现算，装一次即可。
Register-ArgumentCompleter -Native -CommandName keel -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $words = @($commandAst.CommandElements | Select-Object -Skip 1 | ForEach-Object { $_.ToString() })
    # 正在敲的词也在 CommandElements 里，摘掉它 —— 它要单独走 --cur=。
    if ($wordToComplete -ne '' -and $words.Count -gt 0 -and $words[-1] -eq $wordToComplete) {
        if ($words.Count -eq 1) { $words = @() } else { $words = $words[0..($words.Count - 2)] }
    }
    (keel __complete "--cur=$wordToComplete" @words 2>$null) | ForEach-Object {
        $value, $desc = $_ -split "` + "`" + `t", 2
        if (-not $desc) { $desc = $value }
        [System.Management.Automation.CompletionResult]::new($value, $value, 'ParameterValue', $desc)
    }
}
`

// shellScript 返回某个 shell 的补全脚本。
func shellScript(shell string) (string, bool) {
	switch strings.ToLower(shell) {
	case "bash":
		return bashScript, true
	case "zsh":
		return zshScript, true
	case "fish":
		return fishScript, true
	case "powershell", "pwsh":
		return powershellScript, true
	}
	return "", false
}

// installHint 是「装到哪、怎么装」的两行人话。第一行是一劳永逸的写法，
// 第二行是当场生效的写法（不想改 rc 文件时用）。
func installHint(shell string) []string {
	switch strings.ToLower(shell) {
	case "bash":
		return []string{
			`mkdir -p ~/.local/share/bash-completion/completions && keel completion bash > ~/.local/share/bash-completion/completions/keel`,
			`# 或临时生效：source <(keel completion bash)`,
		}
	case "zsh":
		return []string{
			`mkdir -p ~/.zsh/completions && keel completion zsh > ~/.zsh/completions/_keel`,
			`# 再确保 ~/.zshrc 里有：fpath=(~/.zsh/completions $fpath) && autoload -U compinit && compinit`,
		}
	case "fish":
		return []string{
			`keel completion fish > ~/.config/fish/completions/keel.fish`,
			`# fish 会自动加载，不用重启`,
		}
	case "powershell", "pwsh":
		return []string{
			`keel completion powershell | Out-String | Invoke-Expression`,
			`# 想永久生效就把上面这行加到 $PROFILE`,
		}
	}
	return nil
}
