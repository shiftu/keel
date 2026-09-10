package manifest

import "testing"

func TestGoModDirectVsIndirect(t *testing.T) {
	before := []byte(`module x

go 1.27.0

require gopkg.in/yaml.v3 v3.0.1

require golang.org/x/sys v0.29.0 // indirect
`)
	after := []byte(`module x

go 1.27.0

require (
	github.com/new/dep v1.2.3
	gopkg.in/yaml.v3 v3.0.2
)

require golang.org/x/sys v0.29.0 // indirect
`)
	b, _ := Parse("go.mod", before)
	a, _ := Parse("go.mod", after)
	d := Compare(b, a)
	if len(d.DirectAdded()) != 1 || d.DirectAdded()[0].Name != "github.com/new/dep" {
		t.Fatalf("DirectAdded = %+v", d.DirectAdded())
	}
	if len(d.Changed) != 1 || d.Changed[0].Name != "gopkg.in/yaml.v3" {
		t.Fatalf("Changed = %+v", d.Changed)
	}
}

// 只重排格式、不动依赖集合，不能被报成新增依赖。
func TestGoModFormattingOnly(t *testing.T) {
	before := []byte("module x\n\nrequire a/b v1.0.0\nrequire c/d v2.0.0\n")
	after := []byte("module x\n\nrequire (\n\ta/b v1.0.0\n\tc/d v2.0.0\n)\n")
	b, _ := Parse("go.mod", before)
	a, _ := Parse("go.mod", after)
	if d := Compare(b, a); !d.Empty() {
		t.Errorf("纯格式调整不该产生差异，得到 %+v", d)
	}
}

func TestPackageJSONGroups(t *testing.T) {
	before := []byte(`{"name":"x","dependencies":{"left-pad":"1.0.0"}}`)
	after := []byte(`{"name":"x","dependencies":{"left-pad":"1.0.0"},"devDependencies":{"vitest":"2.0.0"}}`)
	b, ok := Parse("package.json", before)
	if !ok {
		t.Fatal("应支持 package.json")
	}
	a, _ := Parse("package.json", after)
	d := Compare(b, a)
	if len(d.DirectAdded()) != 1 || d.DirectAdded()[0].Name != "vitest" {
		t.Fatalf("DirectAdded = %+v", d.DirectAdded())
	}
}

func TestBrokenJSONIsNotParsed(t *testing.T) {
	if _, ok := Parse("package.json", []byte("{not json")); ok {
		t.Error("语法坏了不能报告解析成功")
	}
}

func TestUnsupportedManifest(t *testing.T) {
	if Supported("Cargo.toml") {
		t.Error("M1 还不解析 Cargo.toml，应报待判断信号")
	}
}
