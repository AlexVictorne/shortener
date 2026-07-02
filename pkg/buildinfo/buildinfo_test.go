package buildinfo

import (
	"strings"
	"testing"
)

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

func TestFormat_DefaultsToNA(t *testing.T) {
	out := Format("", "", "")

	for _, line := range []string{
		"Build version: N/A",
		"Build date: N/A",
		"Build commit: N/A",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("Format() does not contain %q, got:\n%s", line, out)
		}
	}
}

func TestFormat_WithValues(t *testing.T) {
	out := Format("1.2.3", "2026-06-27", "abc1234")

	for _, line := range []string{
		"Build version: 1.2.3",
		"Build date: 2026-06-27",
		"Build commit: abc1234",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("Format() does not contain %q, got:\n%s", line, out)
		}
	}
}
