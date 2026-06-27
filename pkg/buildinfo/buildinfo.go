// Package buildinfo содержит переменные и утилиты для отображения информации о сборке.
// Переменные Version, Date и Commit устанавливаются на этапе компиляции через -ldflags.
package buildinfo

import "fmt"

// Version — версия сборки, устанавливается через -ldflags.
var Version string

// Date — дата сборки, устанавливается через -ldflags.
var Date string

// Commit — хеш коммита сборки, устанавливается через -ldflags.
var Commit string

// orNA возвращает "N/A" если строка пустая, иначе - саму строку.
func orNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}

// String возвращает многострочную информацию о сборке для вывода при старте приложения.
func String() string {
	return fmt.Sprintf(
		"Build version: %s\nBuild date: %s\nBuild commit: %s",
		orNA(Version), orNA(Date), orNA(Commit),
	)
}
