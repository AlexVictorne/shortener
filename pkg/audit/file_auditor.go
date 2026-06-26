package audit

import (
	"context"
	"encoding/json"
	"os"
	"sync"
)

// FileAuditor пишет события аудита в файл (по одному JSON на строку).
type FileAuditor struct {
	path string
	mu   sync.Mutex
}

func NewFileAuditor(path string) (*FileAuditor, error) {
	// Проверяем, что файл доступен для записи (создаём если нет).
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	return &FileAuditor{path: path}, nil
}

func (fa *FileAuditor) Emit(_ context.Context, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	fa.mu.Lock()
	defer fa.mu.Unlock()

	f, err := os.OpenFile(fa.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}
