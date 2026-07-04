// cmd/reset генерирует методы Reset() для структур с комментарием // generate:reset.
// Запускать из корня проекта: go run ./cmd/reset/
//
// Для определения, реализует ли поле структуры интерфейс { Reset() },
// используется типовая информация go/types (через golang.org/x/tools/go/packages),
// а не рантайм-приведение типов. Это позволяет генератору решать на этапе
// генерации, нужна ли проверка вообще, и не добавлять в сгенерированный код
// лишние "if resetter, ok := ...(interface{ Reset() })" для полей, которые
// заведомо не могут его реализовывать.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

// resetterIface — интерфейс { Reset() }, с которым сверяются типы полей
// через types.Implements.
var resetterIface = types.NewInterfaceType([]*types.Func{
	types.NewFunc(token.NoPos, nil, "Reset", types.NewSignatureType(nil, nil, nil, nil, nil, false)),
}, nil).Complete()

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax |
	packages.NeedTypesInfo

// fieldInfo описывает одно поле структуры вместе с его типом,
// разрешённым через go/types.
type fieldInfo struct {
	name string
	typ  types.Type
}

// structInfo описывает структуру, помеченную // generate:reset.
// named — тип структуры из go/types; используется, чтобы отследить
// самоссылки и ссылки между структурами, генерируемыми в одном запуске
// (см. resetterChecker).
type structInfo struct {
	name   string
	named  *types.Named
	fields []fieldInfo
}

// pkgInfo описывает пакет с одной или несколькими структурами для генерации.
type pkgInfo struct {
	name    string
	dir     string
	structs []structInfo
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	pkgs, err := loadPackages(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading packages: %v\n", err)
		os.Exit(1)
	}

	generated := 0
	for _, pkg := range pkgs {
		if len(pkg.structs) == 0 {
			continue
		}
		if err := generateFile(pkg); err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s/reset.gen.go: %v\n", pkg.dir, err)
			os.Exit(1)
		}
		fmt.Printf("generated %s/reset.gen.go (%d structs)\n", pkg.dir, len(pkg.structs))
		generated++
	}
	if generated == 0 {
		fmt.Println("no // generate:reset structs found")
	}
}

// loadPackages загружает все пакеты, начиная с root и ниже, вместе с типовой
// информацией, и извлекает структуры, помеченные // generate:reset.
func loadPackages(root string) ([]*pkgInfo, error) {
	cfg := &packages.Config{Mode: loadMode, Dir: root}
	rawPkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(rawPkgs) > 0 {
		return nil, fmt.Errorf("one or more packages under %s failed to type-check", root)
	}

	var result []*pkgInfo
	for _, p := range rawPkgs {
		if len(p.GoFiles) == 0 {
			continue
		}
		structs := collectStructs(p)
		if len(structs) == 0 {
			continue
		}
		result = append(result, &pkgInfo{
			name:    p.Name,
			dir:     filepath.Dir(p.GoFiles[0]),
			structs: structs,
		})
	}
	return result, nil
}

// collectStructs обходит AST-файлы пакета p и извлекает структуры,
// помеченные // generate:reset, вместе с типовой информацией об их полях.
func collectStructs(p *packages.Package) []structInfo {
	var structs []structInfo
	for _, file := range p.Syntax {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE || !hasGenerateReset(genDecl.Doc) {
				continue
			}
			for _, spec := range genDecl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				structs = append(structs, structInfo{
					name:   ts.Name.Name,
					named:  namedType(p, ts.Name.Name),
					fields: collectFields(p, st),
				})
			}
		}
	}
	return structs
}

// namedType возвращает *types.Named для типа с именем name, объявленного
// в пакете p, если такой тип найден в области видимости пакета.
func namedType(p *packages.Package, name string) *types.Named {
	obj := p.Types.Scope().Lookup(name)
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return nil
	}
	named, _ := tn.Type().(*types.Named)
	return named
}

// collectFields извлекает описания полей структуры st, разрешая тип
// каждого поля через типовую информацию пакета p.
func collectFields(p *packages.Package, st *ast.StructType) []fieldInfo {
	var fields []fieldInfo
	for _, field := range st.Fields.List {
		t := p.TypesInfo.TypeOf(field.Type)
		if t == nil {
			continue
		}
		if len(field.Names) == 0 {
			// анонимное (встроенное) поле — имя равно имени типа
			if name := anonFieldName(field.Type); name != "" {
				fields = append(fields, fieldInfo{name: name, typ: t})
			}
			continue
		}
		for _, n := range field.Names {
			if n.Name == "_" {
				continue
			}
			fields = append(fields, fieldInfo{name: n.Name, typ: t})
		}
	}
	return fields
}

func hasGenerateReset(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		if strings.TrimSpace(c.Text) == "// generate:reset" {
			return true
		}
	}
	return false
}

// resetterChecker решает, реализует ли тип интерфейс { Reset() }.
//
// Помимо обычной проверки через types.Implements, учитывает структуры,
// которые генерируются в текущем запуске в том же пакете (generated):
// на первом запуске метод Reset() для них ещё физически не существует
// в исходном коде, поэтому types.Implements всегда вернул бы false для
// самоссылок (см. пример child *ResetableStruct в задании) и ссылок
// между несколькими генерируемыми структурами одного пакета.
//
// Ограничение: ссылки на generate:reset структуры из ДРУГИХ пакетов,
// ещё ни разу не сгенерированные, на первом запуске распознаны не будут —
// для них потребуется повторный запуск генератора после того, как
// зависимый пакет получит свой reset.gen.go.
type resetterChecker struct {
	generated map[*types.Named]bool
}

func newResetterChecker(structs []structInfo) resetterChecker {
	generated := make(map[*types.Named]bool, len(structs))
	for _, s := range structs {
		if s.named != nil {
			generated[s.named] = true
		}
	}
	return resetterChecker{generated: generated}
}

func (rc resetterChecker) implementsResetter(t types.Type) bool {
	if types.Implements(t, resetterIface) {
		return true
	}
	if named := namedOf(t); named != nil {
		return rc.generated[named]
	}
	return false
}

// namedOf разворачивает указатель и возвращает именованный тип, если он есть.
func namedOf(t types.Type) *types.Named {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, _ := t.(*types.Named)
	return named
}

func generateFile(pkg *pkgInfo) error {
	var b strings.Builder
	b.WriteString("// Code generated by cmd/reset; DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s\n", pkg.name)

	rc := newResetterChecker(pkg.structs)
	for _, s := range pkg.structs {
		b.WriteString(generateResetMethod(s, rc))
	}

	src, err := format.Source([]byte(b.String()))
	if err != nil {
		return fmt.Errorf("format error: %w\n--- source ---\n%s", err, b.String())
	}

	return os.WriteFile(filepath.Join(pkg.dir, "reset.gen.go"), src, 0644)
}

func generateResetMethod(s structInfo, rc resetterChecker) string {
	recv := receiverName(s.name)
	var b strings.Builder

	fmt.Fprintf(&b, "\nfunc (%s *%s) Reset() {\n", recv, s.name)
	fmt.Fprintf(&b, "\tif %s == nil {\n\t\treturn\n\t}\n", recv)

	for _, f := range s.fields {
		if line := fieldResetLine(recv, f.name, f.typ, rc); line != "" {
			b.WriteString("\t" + line + "\n")
		}
	}

	b.WriteString("}\n")
	return b.String()
}

// fieldResetLine строит строку сброса для поля recv.name типа t.
// Возвращает "", если для типа t сброс не требуется или невозможен
// (функции, каналы, интерфейсы, фиксированные массивы, типы без Reset()).
func fieldResetLine(recv, name string, t types.Type, rc resetterChecker) string {
	switch tt := t.(type) {
	case *types.Basic:
		if zero, ok := zeroForBasic(tt); ok {
			return fmt.Sprintf("%s.%s = %s", recv, name, zero)
		}
		return ""

	case *types.Pointer:
		return pointerResetLine(recv, name, tt, rc)

	case *types.Slice:
		return fmt.Sprintf("%s.%s = %s.%s[:0]", recv, name, recv, name)

	case *types.Map:
		return fmt.Sprintf("clear(%s.%s)", recv, name)

	case *types.Array, *types.Interface, *types.Signature, *types.Chan:
		return ""

	default:
		// именованный тип (в т.ч. обёртка над примитивом), анонимная
		// структура или тип из другого пакета — сбрасываем только если
		// он (через адресуемое поле) реализует Reset().
		if rc.implementsResetter(types.NewPointer(t)) {
			return fmt.Sprintf("%s.%s.Reset()", recv, name)
		}
		return ""
	}
}

func pointerResetLine(recv, name string, ptr *types.Pointer, rc resetterChecker) string {
	switch tt := ptr.Elem().(type) {
	case *types.Basic:
		if zero, ok := zeroForBasic(tt); ok {
			return fmt.Sprintf("if %s.%s != nil {\n\t\t*%s.%s = %s\n\t}", recv, name, recv, name, zero)
		}
		return ""

	case *types.Slice:
		return fmt.Sprintf("if %s.%s != nil {\n\t\t*%s.%s = (*%s.%s)[:0]\n\t}", recv, name, recv, name, recv, name)

	case *types.Map:
		return fmt.Sprintf("if %s.%s != nil {\n\t\tclear(*%s.%s)\n\t}", recv, name, recv, name)

	case *types.Array:
		return ""

	default:
		if rc.implementsResetter(ptr) {
			return fmt.Sprintf("if %s.%s != nil {\n\t\t%s.%s.Reset()\n\t}", recv, name, recv, name)
		}
		return ""
	}
}

// zeroForBasic возвращает литерал нулевого значения для базового типа b.
// Второй результат — false для типов без осмысленного нулевого литерала
// (например, UnsafePointer или Invalid).
func zeroForBasic(b *types.Basic) (string, bool) {
	info := b.Info()
	switch {
	case info&types.IsBoolean != 0:
		return "false", true
	case info&types.IsString != 0:
		return `""`, true
	case info&(types.IsInteger|types.IsFloat|types.IsComplex) != 0:
		return "0", true
	default:
		return "", false
	}
}

// receiverName строит короткое имя ресивера из CamelCase имени структуры.
func receiverName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsUpper(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	if b.Len() == 0 {
		return strings.ToLower(name[:1])
	}
	return b.String()
}

func anonFieldName(typ ast.Expr) string {
	switch t := typ.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.StarExpr:
		return anonFieldName(t.X)
	}
	return ""
}
