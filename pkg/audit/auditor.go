// Package audit предоставляет интерфейс Auditor и несколько реализаций:
// NoopAuditor (заглушка), FileAuditor (запись в файл), RemoteAuditor (HTTP POST)
// и MultiAuditor (рассылка по нескольким приёмникам).
package audit

import (
	"context"
	"errors"
)

// Auditor — интерфейс наблюдателя аудита.
// Emit отправляет событие в приёмник; Close освобождает ресурсы при завершении работы.
type Auditor interface {
	// Emit асинхронно или синхронно отправляет событие аудита в приёмник.
	Emit(ctx context.Context, e Event) error
	// Close освобождает ресурсы, связанные с приёмником (закрывает файл, соединение и т.п.).
	Close() error
}

// NoopAuditor — заглушка, используемая по умолчанию, когда приёмники не настроены.
type NoopAuditor struct{}

func (NoopAuditor) Emit(_ context.Context, _ Event) error { return nil }
func (NoopAuditor) Close() error                          { return nil }

// MultiAuditor рассылает каждое событие всем зарегистрированным приёмникам.
// При ошибках нескольких приёмников все ошибки объединяются через errors.Join.
type MultiAuditor struct {
	sinks []Auditor
}

// NewMultiAuditor создаёт MultiAuditor из переданных приёмников.
func NewMultiAuditor(sinks ...Auditor) *MultiAuditor {
	return &MultiAuditor{sinks: sinks}
}

func (m *MultiAuditor) Emit(ctx context.Context, e Event) error {
	var errs []error
	for _, s := range m.sinks {
		if err := s.Emit(ctx, e); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *MultiAuditor) Close() error {
	var errs []error
	for _, s := range m.sinks {
		if err := s.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
