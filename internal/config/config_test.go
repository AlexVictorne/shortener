package config_test

import (
	"shortener/internal/config"
	"testing"
)

func TestValidateURL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		def   string
		want  string
	}{
		{"valid http", "http://127.0.0.1:8080", "http://localhost:8080", "http://127.0.0.1:8080"},
		{"valid https", "https://127.0.0.1:8080", "http://localhost:8080", "https://127.0.0.1:8080"},
		{"valid without scheme by ip", "127.0.0.1:8080", "http://localhost:8080", "http://127.0.0.1:8080"},
		{"valid without hostname", "http://:8081", "http://localhost:8080", "http://localhost:8081"},
		{"valid without scheme by localhost", "localhost:8888", "http://localhost:8080", "http://localhost:8888"},
		{"valid by localhost", "http://localhost:8000", "http://localhost:8080", "http://localhost:8000"},
		{"invalid url", "http://%41:8080/", "http://localhost:8080", "http://localhost:8080"},
		{"empty input", "", "http://localhost:8080", "http://localhost:8080"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := config.ValidateURL(c.input, c.def, "URL")
			if got != c.want {
				t.Errorf("ValidateURL(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}
