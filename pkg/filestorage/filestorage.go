package filestorage

import (
	"encoding/json"
	"os"
)

func SaveToFile(data any, filePath string) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, bytes, 0644)
}

func LoadFromFile(filePath string, out any) error {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, out)
}
