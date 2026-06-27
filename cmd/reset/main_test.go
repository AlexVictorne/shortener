package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseStruct разбирает фрагмент Go-кода и возвращает первую найденную структуру.
func parseStruct(t *testing.T, src string) structInfo {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", "package p\n"+src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			if st, ok := ts.Type.(*ast.StructType); ok {
				return structInfo{name: ts.Name.Name, fields: st.Fields.List}
			}
		}
	}
	t.Fatal("no struct found in snippet")
	return structInfo{}
}

// ---- receiverName ----

func TestReceiverName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Config", "c"},
		{"ShortURL", "surl"},
		{"ResetableStruct", "rs"},
		{"HTTPSClient", "httpsc"},
		{"simple", "s"},
		{"A", "a"},
	}
	for _, tc := range cases {
		got := receiverName(tc.input)
		if got != tc.want {
			t.Errorf("receiverName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---- hasGenerateReset ----

func TestHasGenerateReset(t *testing.T) {
	parse := func(src string) *ast.CommentGroup {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "t.go", "package p\n"+src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if gd, ok := d.(*ast.GenDecl); ok && gd.Doc != nil {
				return gd.Doc
			}
		}
		return nil
	}

	t.Run("present", func(t *testing.T) {
		doc := parse("// generate:reset\ntype T struct{}")
		if !hasGenerateReset(doc) {
			t.Error("expected true")
		}
	})
	t.Run("absent", func(t *testing.T) {
		doc := parse("// just a comment\ntype T struct{}")
		if hasGenerateReset(doc) {
			t.Error("expected false")
		}
	})
	t.Run("nil", func(t *testing.T) {
		if hasGenerateReset(nil) {
			t.Error("expected false for nil doc")
		}
	})
	t.Run("among_multiple_comments", func(t *testing.T) {
		doc := parse("// Package docs.\n// generate:reset\n// More docs.\ntype T struct{}")
		if !hasGenerateReset(doc) {
			t.Error("expected true when generate:reset is among multiple comments")
		}
	})
}

// ---- fieldResetLine ----

func TestFieldResetLine_Primitives(t *testing.T) {
	cases := []struct {
		field string
		want  string
	}{
		{"i int", "r.i = 0"},
		{"s string", `r.s = ""`},
		{"b bool", "r.b = false"},
		{"f float64", "r.f = 0"},
		{"by byte", "r.by = 0"},
	}
	for _, tc := range cases {
		si := parseStruct(t, "type T struct { "+tc.field+" }")
		field := si.fields[0]
		got := fieldResetLine("r", field.Names[0].Name, field.Type)
		if got != tc.want {
			t.Errorf("field %q: got %q, want %q", tc.field, got, tc.want)
		}
	}
}

func TestFieldResetLine_Slice(t *testing.T) {
	si := parseStruct(t, "type T struct { items []string }")
	field := si.fields[0]
	got := fieldResetLine("r", "items", field.Type)
	if got != "r.items = r.items[:0]" {
		t.Errorf("slice reset: got %q", got)
	}
}

func TestFieldResetLine_Map(t *testing.T) {
	si := parseStruct(t, "type T struct { m map[string]int }")
	field := si.fields[0]
	got := fieldResetLine("r", "m", field.Type)
	if got != "clear(r.m)" {
		t.Errorf("map reset: got %q", got)
	}
}

func TestFieldResetLine_PointerToPrimitive(t *testing.T) {
	si := parseStruct(t, "type T struct { sp *string }")
	field := si.fields[0]
	got := fieldResetLine("r", "sp", field.Type)
	want := "if r.sp != nil {\n\t\t*r.sp = \"\"\n\t}"
	if got != want {
		t.Errorf("*string reset:\ngot  %q\nwant %q", got, want)
	}
}

func TestFieldResetLine_PointerToNamedType(t *testing.T) {
	si := parseStruct(t, "type T struct { child *Child }")
	field := si.fields[0]
	got := fieldResetLine("r", "child", field.Type)
	if !strings.Contains(got, "interface{ Reset() }") {
		t.Errorf("*NamedType should use interface assertion, got: %q", got)
	}
	if !strings.Contains(got, "r.child != nil") {
		t.Errorf("*NamedType should check nil, got: %q", got)
	}
}

func TestFieldResetLine_NamedType(t *testing.T) {
	si := parseStruct(t, "type T struct { nested Other }")
	field := si.fields[0]
	got := fieldResetLine("r", "nested", field.Type)
	if !strings.Contains(got, "interface{ Reset() }") {
		t.Errorf("named type should use interface assertion, got: %q", got)
	}
}

func TestFieldResetLine_SelectorExpr(t *testing.T) {
	si := parseStruct(t, "type T struct { t time.Time }")
	field := si.fields[0]
	got := fieldResetLine("r", "t", field.Type)
	if !strings.Contains(got, "interface{ Reset() }") {
		t.Errorf("pkg.Type should use interface assertion, got: %q", got)
	}
}

// ---- generateResetMethod ----

func TestGenerateResetMethod_NilGuard(t *testing.T) {
	si := parseStruct(t, "type Empty struct{}")
	out := generateResetMethod(si)
	if !strings.Contains(out, "if e == nil") {
		t.Errorf("missing nil guard, got:\n%s", out)
	}
}

func TestGenerateResetMethod_FullStruct(t *testing.T) {
	src := `type S struct {
		i     int
		str   string
		strP  *string
		items []int
		m     map[string]string
		child *S
	}`
	si := parseStruct(t, src)
	out := generateResetMethod(si)

	checks := []string{
		"s.i = 0",
		`s.str = ""`,
		"s.strP != nil",
		"s.items = s.items[:0]",
		"clear(s.m)",
		"interface{ Reset() }",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("generated method missing %q:\n%s", want, out)
		}
	}
}

// ---- end-to-end ----

// TestEndToEnd проверяет полный цикл: исходный файл → walkPackages → generateFile → содержимое reset.gen.go.
func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()

	src := `package mypkg

// generate:reset
type Item struct {
	ID    int
	Name  string
	Tags  []string
	Meta  map[string]string
}
`
	if err := os.WriteFile(filepath.Join(dir, "item.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	packages, err := walkPackages(dir)
	if err != nil {
		t.Fatalf("walkPackages: %v", err)
	}
	if len(packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(packages))
	}

	var pkg *pkgInfo
	for _, p := range packages {
		pkg = p
	}
	if pkg.name != "mypkg" {
		t.Errorf("pkg name: got %q, want %q", pkg.name, "mypkg")
	}
	if len(pkg.structs) != 1 || pkg.structs[0].name != "Item" {
		t.Errorf("unexpected structs: %+v", pkg.structs)
	}

	if err := generateFile(dir, pkg); err != nil {
		t.Fatalf("generateFile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "reset.gen.go"))
	if err != nil {
		t.Fatalf("reading reset.gen.go: %v", err)
	}
	content := string(data)

	checks := []string{
		"package mypkg",
		"DO NOT EDIT",
		"func (i *Item) Reset()",
		"i.ID = 0",
		`i.Name = ""`,
		"i.Tags = i.Tags[:0]",
		"clear(i.Meta)",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("reset.gen.go missing %q:\n%s", want, content)
		}
	}
}

// TestEndToEnd_NoStructs проверяет, что структуры без комментария // generate:reset не попадают в генерацию.
func TestEndToEnd_NoStructs(t *testing.T) {
	dir := t.TempDir()
	src := "package empty\n\ntype Plain struct{ X int }\n"
	if err := os.WriteFile(filepath.Join(dir, "plain.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	packages, err := walkPackages(dir)
	if err != nil {
		t.Fatalf("walkPackages: %v", err)
	}
	if len(packages) != 0 {
		t.Errorf("expected 0 packages with generate:reset structs, got %d", len(packages))
	}
}

// TestEndToEnd_MultipleStructsSamePackage проверяет, что несколько структур одного пакета попадают в один reset.gen.go.
func TestEndToEnd_MultipleStructsSamePackage(t *testing.T) {
	dir := t.TempDir()
	src := `package multi

// generate:reset
type A struct{ X int }

// generate:reset
type B struct{ Y string }
`
	if err := os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	packages, err := walkPackages(dir)
	if err != nil {
		t.Fatalf("walkPackages: %v", err)
	}
	pkg := packages[dir]
	if pkg == nil || len(pkg.structs) != 2 {
		t.Fatalf("expected 2 structs, got %+v", pkg)
	}

	if err := generateFile(dir, pkg); err != nil {
		t.Fatalf("generateFile: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "reset.gen.go"))
	content := string(data)
	if !strings.Contains(content, "func (a *A) Reset()") || !strings.Contains(content, "func (b *B) Reset()") {
		t.Errorf("reset.gen.go missing one of the Reset methods:\n%s", content)
	}
}
