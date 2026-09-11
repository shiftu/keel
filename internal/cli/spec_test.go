package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 补全的说明书（spec.go）和真正注册选项的那几行是两处代码。两处写着同一件事，
// 就会有一处先坏掉——而补全坏了不报错，只是补错，用户还不知道。
//
// 所以这里不靠人去记：直接读源码里 newFlagSet + fs.Xxx(...) 的那些调用，
// 逐个命令跟 spec 对。少写了、多写了、改了名，测试当场报出来。

func TestSpecCommandsMatchDispatch(t *testing.T) {
	inSpec := map[string]bool{}
	for _, c := range specs() {
		inSpec[c.Name] = true
	}
	inTable := map[string]bool{}
	for _, c := range commands() {
		inTable[c.name] = true
		if !inSpec[c.name] {
			t.Errorf("命令 %q 在 commands() 里有，spec 里没有", c.name)
		}
	}
	for _, c := range specs() {
		if !inTable[c.Name] {
			t.Errorf("命令 %q 在 spec 里有，commands() 里没有", c.Name)
		}
	}
}

func TestSpecHiddenMatchesDispatch(t *testing.T) {
	hidden := map[string]bool{}
	for _, c := range commands() {
		hidden[c.name] = c.hidden
	}
	for _, c := range specs() {
		if c.Hidden != hidden[c.Name] {
			t.Errorf("命令 %q：spec hidden=%v，commands() hidden=%v", c.Name, c.Hidden, hidden[c.Name])
		}
	}
}

func TestSpecFlagsMatchSource(t *testing.T) {
	real := flagSetsFromSource(t)
	for _, c := range specs() {
		if c.Hidden {
			continue // 内部入口不参与补全，也就没有说明书
		}
		if len(c.Subs) > 0 {
			for _, sub := range c.Subs {
				checkFlags(t, real, c.Name+" "+sub.Name, sub)
			}
			continue
		}
		checkFlags(t, real, c.Name, c)
	}
}

func checkFlags(t *testing.T, real map[string]flagSetFacts, name string, c cmdSpec) {
	t.Helper()
	got, ok := real[name]
	if !ok {
		t.Errorf("%s：源码里找不到 newFlagSet(%q)；spec 和实现对不上", name, name)
		return
	}
	if !got.common {
		t.Errorf("%s：没有调 commonFlags —— 每个命令都该接受 -C / --quiet / --json", name)
	}
	// commonSpec 那三个由 commonFlags 自己注册，调用点上看不见，
	// 所以只对命令自己的选项，通用的那几个靠上面 got.common 这一条盯着。
	want := map[string]bool{}
	for _, f := range c.Flags {
		want[f.Name] = true
	}
	for n := range got.flags {
		if !want[n] {
			t.Errorf("%s：源码注册了 --%s，spec 里没写", name, n)
		}
	}
	for n := range want {
		if !got.flags[n] {
			t.Errorf("%s：spec 写了 --%s，源码没注册", name, n)
		}
	}
}

type flagSetFacts struct {
	flags  map[string]bool
	common bool
}

// flagSetsFromSource 读本包的源码，找出每个 newFlagSet("<name>") 上注册了哪些选项。
func flagSetsFromSource(t *testing.T) map[string]flagSetFacts {
	t.Helper()
	out := map[string]flagSetFacts{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s：%v", name, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			collectFlagSet(fn.Body, out)
		}
	}
	if len(out) == 0 {
		t.Fatal("一个 newFlagSet 都没找到，这个测试自己坏了")
	}
	return out
}

// collectFlagSet 在一个函数体里找 newFlagSet 和随后挂在它身上的选项注册。
func collectFlagSet(body *ast.BlockStmt, out map[string]flagSetFacts) {
	varName, cmdName := "", ""
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		lit, ok := callStringArg(as.Rhs[0], "newFlagSet", 0)
		if !ok {
			return true
		}
		if id, ok := as.Lhs[0].(*ast.Ident); ok {
			varName, cmdName = id.Name, lit
		}
		return true
	})
	if varName == "" {
		return
	}
	facts := flagSetFacts{flags: map[string]bool{}}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			// fs.Bool("name", …) / fs.Var(&v, "name", …) / fs.StringVar(&v, "name", …)
			id, ok := fn.X.(*ast.Ident)
			if !ok || id.Name != varName {
				return true
			}
			idx := 0
			if strings.HasSuffix(fn.Sel.Name, "Var") {
				idx = 1
			}
			if s, ok := stringArg(call, idx); ok {
				facts.flags[s] = true
			}
		case *ast.Ident:
			// listFlag(fs, "name", …) / commonFlags(fs, e)
			if len(call.Args) == 0 {
				return true
			}
			first, ok := call.Args[0].(*ast.Ident)
			if !ok || first.Name != varName {
				return true
			}
			switch fn.Name {
			case "commonFlags":
				facts.common = true
			case "listFlag":
				if s, ok := stringArg(call, 1); ok {
					facts.flags[s] = true
				}
			}
		}
		return true
	})
	out[cmdName] = facts
}

func callStringArg(e ast.Expr, fnName string, idx int) (string, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != fnName {
		return "", false
	}
	return stringArg(call, idx)
}

func stringArg(call *ast.CallExpr, idx int) (string, bool) {
	if idx >= len(call.Args) {
		return "", false
	}
	lit, ok := call.Args[idx].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// 选项的取值候选只要写了，就得是这个选项真认的那几个——
// 补一个命令当场会拒绝的值，比不补更糟。
func TestSpecValuesAreSane(t *testing.T) {
	var walk func(list []cmdSpec, path string)
	walk = func(list []cmdSpec, path string) {
		for _, c := range list {
			name := strings.TrimSpace(path + " " + c.Name)
			for _, f := range c.Flags {
				if f.Arg == "" && (len(f.Values) > 0 || f.Dyn != dynNone) {
					t.Errorf("%s --%s：布尔选项不该有取值候选", name, f.Name)
				}
				if len(f.Values) > 0 && f.Dyn != dynNone {
					t.Errorf("%s --%s：固定候选和动态候选只能有一个", name, f.Name)
				}
				seen := map[string]bool{}
				for _, v := range f.Values {
					if seen[v.Name] {
						t.Errorf("%s --%s：候选 %q 重复", name, f.Name, v.Name)
					}
					seen[v.Name] = true
				}
			}
			walk(c.Subs, name)
		}
	}
	walk(specs(), "")
}

// 帮助里的命令顺序和 spec 一致，人和补全看到的才是同一张表。
func TestSpecOrderMatchesDispatch(t *testing.T) {
	var a, b []string
	for _, c := range commands() {
		a = append(a, c.name)
	}
	for _, c := range specs() {
		b = append(b, c.Name)
	}
	if strings.Join(a, ",") != strings.Join(b, ",") {
		sort.Strings(a)
		sort.Strings(b)
		t.Errorf("命令顺序对不上：\n commands(): %v\n specs():    %v", a, b)
	}
}
