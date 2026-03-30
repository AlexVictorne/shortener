package filestorage

import (
	"os"
	"testing"
)

type testStruct struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestSaveAndLoadToFile(t *testing.T) {
	filePath := "testdata.json"
	defer os.Remove(filePath)

	original := []testStruct{
		{ID: 1, Name: "ya"},
		{ID: 2, Name: "ru"},
	}

	err := SaveToFile(original, filePath)
	if err != nil {
		t.Fatalf("SaveToFile error: %v", err)
	}

	var loaded []testStruct
	err = LoadFromFile(filePath, &loaded)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	if len(loaded) != len(original) {
		t.Fatalf("Loaded length %d != original %d", len(loaded), len(original))
	}
	for i := range original {
		if original[i] != loaded[i] {
			t.Errorf("Mismatch at %d: got %+v, want %+v", i, loaded[i], original[i])
		}
	}
}
