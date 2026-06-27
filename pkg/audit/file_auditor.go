package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sync"
)

// FileAuditor пишет события аудита в файл (по одному JSON на строку).
// Файл открывается один раз при создании и закрывается через Close.
type FileAuditor struct {
	mu sync.Mutex
	f  *os.File
	bw *bufio.Writer
}

func NewFileAuditor(path string) (*FileAuditor, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &FileAuditor{f: f, bw: bufio.NewWriterSize(f, 64*1024)}, nil
}

func (fa *FileAuditor) Emit(_ context.Context, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	fa.mu.Lock()
	defer fa.mu.Unlock()

	fa.bw.Write(data)
	fa.bw.WriteByte('\n')
	return fa.bw.Flush()
}

// Close сбрасывает буфер и закрывает файл.
func (fa *FileAuditor) Close() error {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	if err := fa.bw.Flush(); err != nil {
		return err
	}
	return fa.f.Close()
}
