// Package buildinfo предоставляет утилиты для форматирования информации о сборке.
package buildinfo

import "fmt"

// orNA возвращает "N/A" если строка пустая, иначе — саму строку.
func orNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}

// Format возвращает многострочную информацию о сборке для вывода при старте приложения.
// Пустые значения заменяются на "N/A".
func Format(version, date, commit string) string {
	return fmt.Sprintf(
		"Build version: %s\nBuild date: %s\nBuild commit: %s",
		orNA(version), orNA(date), orNA(commit),
	)
}
