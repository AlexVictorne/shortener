package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestModule создает минимальный go.mod во временной директории dir,
// чтобы golang.org/x/tools/go/packages мог загрузить пакет как отдельный модуль.
func writeTestModule(t *testing.T, dir string) {
	t.Helper()
	mod := "module resettest\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0644); err != nil {
		t.Fatal(err)
	}
}

// loadTestStructs пишет src как единственный файл временного пакета,
// прогоняет его через loadPackages и возвращает структуры первого пакета.
func loadTestStructs(t *testing.T, src string) []structInfo {
	t.Helper()
	dir := t.TempDir()
	writeTestModule(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := loadPackages(dir)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package with generate:reset structs, got %d", len(pkgs))
	}
	return pkgs[0].structs
}

// fieldByName возвращает тип поля name среди fields, либо проваливает тест.
func fieldByName(t *testing.T, fields []fieldInfo, name string) types.Type {
	t.Helper()
	for _, f := range fields {
		if f.name == name {
			return f.typ
		}
	}
	t.Fatalf("field %q not found among %+v", name, fields)
	return nil
}

// structByName возвращает структуру name среди structs, либо проваливает тест.
func structByName(t *testing.T, structs []structInfo, name string) structInfo {
	t.Helper()
	for _, s := range structs {
		if s.name == name {
			return s
		}
	}
	t.Fatalf("struct %q not found", name)
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

// ---- zeroForBasic ----

func TestZeroForBasic(t *testing.T) {
	src := `package p

// generate:reset
type T struct {
	i  int
	s  string
	b  bool
	f  float64
	by byte
	r  rune
	c  complex128
}
`
	fields := loadTestStructs(t, src)[0].fields
	cases := map[string]string{
		"i":  "0",
		"s":  `""`,
		"b":  "false",
		"f":  "0",
		"by": "0",
		"r":  "0",
		"c":  "0",
	}
	for name, want := range cases {
		typ := fieldByName(t, fields, name)
		basic, ok := typ.(*types.Basic)
		if !ok {
			t.Fatalf("field %q: expected *types.Basic, got %T", name, typ)
		}
		got, ok := zeroForBasic(basic)
		if !ok || got != want {
			t.Errorf("zeroForBasic(%q) = %q, %v; want %q, true", name, got, ok, want)
		}
	}
}

// ---- fieldResetLine ----

func TestFieldResetLine_Primitives(t *testing.T) {
	src := `package p

// generate:reset
type T struct {
	i  int
	s  string
	b  bool
	f  float64
	by byte
}
`
	fields := loadTestStructs(t, src)[0].fields
	rc := resetterChecker{}
	cases := []struct {
		field string
		want  string
	}{
		{"i", "r.i = 0"},
		{"s", `r.s = ""`},
		{"b", "r.b = false"},
		{"f", "r.f = 0"},
		{"by", "r.by = 0"},
	}
	for _, tc := range cases {
		typ := fieldByName(t, fields, tc.field)
		got := fieldResetLine("r", tc.field, typ, rc)
		if got != tc.want {
			t.Errorf("field %q: got %q, want %q", tc.field, got, tc.want)
		}
	}
}

func TestFieldResetLine_Slice(t *testing.T) {
	src := `package p

// generate:reset
type T struct {
	items []string
}
`
	fields := loadTestStructs(t, src)[0].fields
	got := fieldResetLine("r", "items", fieldByName(t, fields, "items"), resetterChecker{})
	if got != "r.items = r.items[:0]" {
		t.Errorf("slice reset: got %q", got)
	}
}

func TestFieldResetLine_Map(t *testing.T) {
	src := `package p

// generate:reset
type T struct {
	m map[string]int
}
`
	fields := loadTestStructs(t, src)[0].fields
	got := fieldResetLine("r", "m", fieldByName(t, fields, "m"), resetterChecker{})
	if got != "clear(r.m)" {
		t.Errorf("map reset: got %q", got)
	}
}

func TestFieldResetLine_PointerToPrimitive(t *testing.T) {
	src := `package p

// generate:reset
type T struct {
	sp *string
}
`
	fields := loadTestStructs(t, src)[0].fields
	got := fieldResetLine("r", "sp", fieldByName(t, fields, "sp"), resetterChecker{})
	want := "if r.sp != nil {\n\t\t*r.sp = \"\"\n\t}"
	if got != want {
		t.Errorf("*string reset:\ngot  %q\nwant %q", got, want)
	}
}

func TestFieldResetLine_PointerToNamedType_WithReset(t *testing.T) {
	src := `package p

type Child struct{ X int }

func (c *Child) Reset() { c.X = 0 }

// generate:reset
type T struct {
	child *Child
}
`
	fields := loadTestStructs(t, src)[0].fields
	got := fieldResetLine("r", "child", fieldByName(t, fields, "child"), resetterChecker{})
	if !strings.Contains(got, "r.child.Reset()") {
		t.Errorf("*Child with Reset() should call it directly, got: %q", got)
	}
	if !strings.Contains(got, "r.child != nil") {
		t.Errorf("*Child should check nil, got: %q", got)
	}
	if strings.Contains(got, "interface{") {
		t.Errorf("static go/types check must not emit a runtime interface assertion, got: %q", got)
	}
}

func TestFieldResetLine_NamedType_ValueField_WithPointerReset(t *testing.T) {
	src := `package p

type Other struct{ Y string }

func (o *Other) Reset() { o.Y = "" }

// generate:reset
type T struct {
	nested Other
}
`
	fields := loadTestStructs(t, src)[0].fields
	got := fieldResetLine("r", "nested", fieldByName(t, fields, "nested"), resetterChecker{})
	if got != "r.nested.Reset()" {
		t.Errorf("addressable value field should call Reset() directly without nil check, got: %q", got)
	}
}

func TestFieldResetLine_ExternalTypeWithoutReset(t *testing.T) {
	src := `package p

import "time"

// generate:reset
type T struct {
	t time.Time
}
`
	fields := loadTestStructs(t, src)[0].fields
	got := fieldResetLine("r", "t", fieldByName(t, fields, "t"), resetterChecker{})
	if got != "" {
		t.Errorf("time.Time does not implement Reset(); expected no generated line, got: %q", got)
	}
}

func TestFieldResetLine_NamedTypeWithoutReset(t *testing.T) {
	src := `package p

type Plain struct{ Z int }

// generate:reset
type T struct {
	plain Plain
}
`
	fields := loadTestStructs(t, src)[0].fields
	got := fieldResetLine("r", "plain", fieldByName(t, fields, "plain"), resetterChecker{})
	if got != "" {
		t.Errorf("Plain has no Reset(); expected no generated line, got: %q", got)
	}
}

// ---- generateResetMethod ----

func TestGenerateResetMethod_NilGuard(t *testing.T) {
	out := generateResetMethod(structInfo{name: "Empty"}, resetterChecker{})
	if !strings.Contains(out, "if e == nil") {
		t.Errorf("missing nil guard, got:\n%s", out)
	}
}

// TestGenerateResetMethod_SelfReference — ключевой сценарий из задания:
// структура ссылается сама на себя через указатель (child *S). На момент
// генерации Reset() для S ещё не существует в исходном коде, поэтому
// resetterChecker должен опираться на набор структур, генерируемых в этом
// же запуске, а не только на types.Implements.
func TestGenerateResetMethod_SelfReference(t *testing.T) {
	src := `package p

// generate:reset
type S struct {
	i     int
	str   string
	strP  *string
	items []int
	m     map[string]string
	child *S
}
`
	structs := loadTestStructs(t, src)
	s := structByName(t, structs, "S")
	rc := newResetterChecker(structs)
	out := generateResetMethod(s, rc)

	checks := []string{
		"s.i = 0",
		`s.str = ""`,
		"s.strP != nil",
		"s.items = s.items[:0]",
		"clear(s.m)",
		"s.child != nil",
		"s.child.Reset()",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("generated method missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "interface{") {
		t.Errorf("self-reference must be resolved statically, no runtime assertion expected:\n%s", out)
	}
}

// ---- end-to-end ----

// TestEndToEnd проверяет полный цикл: исходный файл → loadPackages → generateFile → содержимое reset.gen.go.
func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

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

	pkgs, err := loadPackages(dir)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	pkg := pkgs[0]
	if pkg.name != "mypkg" {
		t.Errorf("pkg name: got %q, want %q", pkg.name, "mypkg")
	}
	if len(pkg.structs) != 1 || pkg.structs[0].name != "Item" {
		t.Errorf("unexpected structs: %+v", pkg.structs)
	}

	if err := generateFile(pkg); err != nil {
		t.Fatalf("generateFile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(pkg.dir, "reset.gen.go"))
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
	writeTestModule(t, dir)
	src := "package empty\n\ntype Plain struct{ X int }\n"
	if err := os.WriteFile(filepath.Join(dir, "plain.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := loadPackages(dir)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages with generate:reset structs, got %d", len(pkgs))
	}
}

// TestEndToEnd_MultipleStructsSamePackage проверяет, что несколько структур одного пакета попадают в один reset.gen.go.
func TestEndToEnd_MultipleStructsSamePackage(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)
	src := `package multi

// generate:reset
type A struct{ X int }

// generate:reset
type B struct{ Y string }
`
	if err := os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := loadPackages(dir)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgs) != 1 || len(pkgs[0].structs) != 2 {
		t.Fatalf("expected 1 package with 2 structs, got %+v", pkgs)
	}

	if err := generateFile(pkgs[0]); err != nil {
		t.Fatalf("generateFile: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(pkgs[0].dir, "reset.gen.go"))
	content := string(data)
	if !strings.Contains(content, "func (a *A) Reset()") || !strings.Contains(content, "func (b *B) Reset()") {
		t.Errorf("reset.gen.go missing one of the Reset methods:\n%s", content)
	}
}

// TestEndToEnd_CrossStructReference проверяет ссылку между двумя генерируемыми
// структурами одного пакета (не самоссылку): Parent содержит поле типа *Child,
// и обе структуры помечены // generate:reset в одном запуске.
func TestEndToEnd_CrossStructReference(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)
	src := `package tree

// generate:reset
type Child struct {
	Value int
}

// generate:reset
type Parent struct {
	child *Child
}
`
	if err := os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := loadPackages(dir)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if err := generateFile(pkgs[0]); err != nil {
		t.Fatalf("generateFile: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(pkgs[0].dir, "reset.gen.go"))
	content := string(data)

	if !strings.Contains(content, "func (c *Child) Reset()") {
		t.Errorf("missing Child.Reset():\n%s", content)
	}
	if !strings.Contains(content, "p.child.Reset()") {
		t.Errorf("Parent.Reset() should call child.Reset() even though Child.Reset() didn't exist before this run:\n%s", content)
	}
}

// TestCollectFields_SkipsBlankIdentifier — регрессионный тест на баг старой
// AST-based реализации: поле с именем "_" (blank identifier) не может быть
// адресовано как r._ в Go ("cannot refer to blank field or method"), поэтому
// генератор обязан пропускать такие поля, а не превращать их в невалидный код.
func TestCollectFields_SkipsBlankIdentifier(t *testing.T) {
	src := `package p

// generate:reset
type T struct {
	i int
	_ int
	s string
}
`
	fields := loadTestStructs(t, src)[0].fields
	if len(fields) != 2 {
		t.Fatalf("expected blank field to be skipped, got %d fields: %+v", len(fields), fields)
	}
	for _, f := range fields {
		if f.name == "_" {
			t.Errorf("blank identifier field must not appear in collected fields: %+v", fields)
		}
	}
}

// TestGenerateResetMethod_BlankIdentifier_CompilesCleanly проверяет end-to-end,
// что структура с полем "_" генерирует валидный, компилируемый Reset() —
// то есть сборка сгенерированного пакета проходит без ошибок формата/компиляции.
func TestGenerateResetMethod_BlankIdentifier_CompilesCleanly(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)
	src := `package blankpkg

// generate:reset
type T struct {
	i int
	_ int
}
`
	if err := os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := loadPackages(dir)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if err := generateFile(pkgs[0]); err != nil {
		t.Fatalf("generateFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(pkgs[0].dir, "reset.gen.go"))
	if err != nil {
		t.Fatalf("reading reset.gen.go: %v", err)
	}
	if strings.Contains(string(content), "._") {
		t.Errorf("generated code must never reference the blank field via selector:\n%s", content)
	}

	// Повторная загрузка пакета вместе со сгенерированным файлом должна
	// пройти type-check без ошибок — иначе сгенерированный код невалиден.
	if _, err := loadPackages(dir); err != nil {
		t.Fatalf("generated reset.gen.go does not compile: %v", err)
	}
}

// ---- loadPackages: ошибки компиляции ----

// TestLoadPackages_TypeCheckError фиксирует новое поведение по сравнению со
// старой чисто AST-based реализацией: раньше go/parser разбирал только
// синтаксис и мог работать даже при наличии ошибок компиляции в модуле.
// Теперь loadPackages использует go/packages с типовой проверкой и обязан
// вернуть ошибку, если хотя бы один загруженный пакет не проходит type-check —
// иначе типовая информация для остальных пакетов может быть неполной/некорректной.
func TestLoadPackages_TypeCheckError(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)
	src := `package broken

// generate:reset
type T struct {
	i undefinedType
}
`
	if err := os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadPackages(dir); err == nil {
		t.Error("expected loadPackages to fail on a package that does not type-check")
	}
}
