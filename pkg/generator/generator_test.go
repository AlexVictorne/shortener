package generator_test

import (
	"shortener/pkg/generator"
	"testing"
)

func TestGenerator_GenerateID(t *testing.T) {
	tests := []struct {
		name    string
		length  int
		wantLen int
		wantErr bool
	}{
		{name: "length 8", length: 8, wantLen: 8, wantErr: false},
		{name: "length 16", length: 16, wantLen: 16, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := generator.NewGenerator(tt.length)
			got, gotErr := g.GenerateID()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("GenerateID failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("GenerateID succeeded unexpectedly")
			}
			if len(got) != tt.wantLen {
				t.Errorf("GenerateID = %v (len=%d), want len %d", got, len(got), tt.wantLen)
			}
		})
	}
}

func TestGenerator_Validate(t *testing.T) {
	tests := []struct {
		name   string
		length int
		id     string
		want   bool
	}{
		{name: "valid", length: 8, id: "abcdEF12", want: true},
		{name: "wrong", length: 8, id: "abcd", want: false},
		{name: "invalid", length: 8, id: "ab!@#$%^", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := generator.NewGenerator(tt.length)
			got := g.Validate(tt.id)
			if got != tt.want {
				t.Errorf("Validate = %v, want %v", got, tt.want)
			}
		})
	}
}
