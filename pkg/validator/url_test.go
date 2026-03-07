package validator

import "testing"

func TestValidateURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid http", "http://127.0.0.1:8080", "http://127.0.0.1:8080", false},
		{"valid https", "https://127.0.0.1:8080", "https://127.0.0.1:8080", false},
		{"no scheme by ip", "127.0.0.1:8080", "", true},
		{"without hostname", "http://:8081", "", true},
		{"no scheme by localhost", "localhost:8888", "", true},
		{"valid by localhost", "http://localhost:8000", "http://localhost:8000", false},
		{"invalid url", "http://%41:8080/", "", true},
		{"empty input", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ValidateURL(c.input)
			if c.wantErr {
				if err == nil {
					t.Errorf("ValidateURL(%q) expected error, got none", c.input)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateURL(%q) unexpected error: %v", c.input, err)
				}
				if got != c.want {
					t.Errorf("ValidateURL(%q) = %q, want %q", c.input, got, c.want)
				}
			}
		})
	}
}
