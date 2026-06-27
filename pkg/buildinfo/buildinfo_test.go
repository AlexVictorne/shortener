package buildinfo

import (
	"strings"
	"testing"
)

// TestOrNA проверяет, что orNA возвращает "N/A" для пустой строки и саму строку иначе.
func TestOrNA(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "N/A"},
		{"v1.0.0", "v1.0.0"},
		{"   ", "   "},
	}
	for _, tc := range tests {
		if got := orNA(tc.input); got != tc.want {
			t.Errorf("orNA(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestString_DefaultsToNA проверяет, что при пустых переменных все поля содержат "N/A".
func TestString_DefaultsToNA(t *testing.T) {
	Version = ""
	Date = ""
	Commit = ""

	out := String()

	for _, line := range []string{
		"Build version: N/A",
		"Build date: N/A",
		"Build commit: N/A",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("String() does not contain %q, got:\n%s", line, out)
		}
	}
}

// TestString_WithValues проверяет, что установленные значения отображаются корректно.
func TestString_WithValues(t *testing.T) {
	Version = "1.2.3"
	Date = "2026-06-27"
	Commit = "abc1234"
	defer func() {
		Version = ""
		Date = ""
		Commit = ""
	}()

	out := String()

	for _, line := range []string{
		"Build version: 1.2.3",
		"Build date: 2026-06-27",
		"Build commit: abc1234",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("String() does not contain %q, got:\n%s", line, out)
		}
	}
}
