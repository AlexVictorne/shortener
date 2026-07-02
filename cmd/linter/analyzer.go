package main

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Analyzer — статический анализатор exitcheck.
//
// Анализатор проверяет каждый файл пакета и сообщает о двух категориях
// нарушений:
//
//   - вызов встроенной функции panic (в любом месте программы);
//   - вызов log.Fatal, log.Fatalf, log.Fatalln или os.Exit вне функции
//     main пакета main.
//
// Оба правила направлены на то, чтобы завершение процесса всегда было
// явным, контролируемым и происходило только в точке входа программы.
var Analyzer = &analysis.Analyzer{
	Name: "exitcheck",
	Doc:  "сообщает о вызовах panic, log.Fatal* и os.Exit вне функции main пакета main",
	Run:  run,
}

// logFatalNames — множество имён функций пакета log, приводящих к завершению
// процесса. Проверяется только имя селектора; принадлежность пакету log
// устанавливается по имени квалификатора в AST.
var logFatalNames = map[string]bool{
	"Fatal":   true,
	"Fatalf":  true,
	"Fatalln": true,
}

// run — точка входа анализатора. Обходит все файлы пакета и для каждого
// вызывает checkFile.
func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		checkFile(pass, file)
	}
	return nil, nil
}

// checkFile анализирует один файл. Для каждого объявления верхнего уровня
// определяет, является ли оно функцией main пакета main, и передаёт тело
// объявления в соответствующую функцию обхода.
func checkFile(pass *analysis.Pass, file *ast.File) {
	isMainPkg := file.Name.Name == "main"

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// Разрешены log.Fatal*/os.Exit только в func main() пакета main.
			isMain := isMainPkg && d.Name != nil && d.Name.Name == "main"
			if d.Body != nil {
				walk(pass, d.Body, isMain)
			}
		case *ast.GenDecl:
			// Инициализаторы переменных/констант уровня пакета находятся вне
			// любой функции — запрещённые вызовы здесь недопустимы.
			walk(pass, d, false)
		}
	}
}

// walk обходит произвольный узел AST node. Параметр inMain сигнализирует,
// что node находится в теле функции main пакета main и вызовы
// log.Fatal*/os.Exit в нём разрешены.
//
// Вложенные функциональные литералы (func() { ... }) обрабатываются
// рекурсивно с inMain = false: замыкание не является функцией main, даже
// если определено внутри неё.
func walk(pass *analysis.Pass, node ast.Node, inMain bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		// Функциональный литерал образует собственную область видимости.
		// Спускаемся в него с inMain = false.
		if fl, ok := n.(*ast.FuncLit); ok {
			walk(pass, fl.Body, false)
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			reportIfForbidden(pass, call, inMain)
		}
		return true
	})
}

// reportIfForbidden проверяет одно выражение вызова и при обнаружении
// нарушения выдаёт диагностическое сообщение через pass.Reportf.
//
// Проверяются три случая:
//   - вызов идентификатора "panic" — запрещён всегда;
//   - вызов os.Exit — запрещён вне main.main;
//   - вызов log.Fatal, log.Fatalf или log.Fatalln — запрещён вне main.main.
//
// Принадлежность пакету определяется по имени квалифицирующего идентификатора
// в AST (os.Exit → квалификатор "os", log.Fatal → квалификатор "log").
// Это соответствует типичному использованию стандартных пакетов и намеренно
// не проверяет полный путь импорта во избежание лишней сложности.
func reportIfForbidden(pass *analysis.Pass, call *ast.CallExpr, inMain bool) {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		if fn.Name == "panic" {
			pass.Reportf(call.Pos(), "use of panic is forbidden")
		}

	case *ast.SelectorExpr:
		pkgIdent, ok := fn.X.(*ast.Ident)
		if !ok {
			return
		}
		sel := fn.Sel.Name

		if pkgIdent.Name == "os" && sel == "Exit" && !inMain {
			pass.Reportf(call.Pos(), "os.Exit called outside of main.main")
		}

		if pkgIdent.Name == "log" && logFatalNames[sel] && !inMain {
			pass.Reportf(call.Pos(), "log.%s called outside of main.main", sel)
		}
	}
}
